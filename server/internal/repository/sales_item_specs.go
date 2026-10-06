package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"customer-support/internal/model"
)

func (r *SalesItemRepo) ListSpecs(itemID string, includeInactive bool) ([]model.SalesItemSpec, error) {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return nil, nil
	}
	q := `SELECT spec_id, item_id, COALESCE(spec,''), COALESCE(unit,'EA'), COALESCE(price,0),
		COALESCE(sort_order,0), COALESCE(is_active,1)
		FROM sales_item_specs WHERE item_id=?`
	if !includeInactive {
		q += ` AND COALESCE(is_active,1)=1`
	}
	q += ` ORDER BY sort_order, spec_id`
	rows, err := r.db.Query(q, itemID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.SalesItemSpec
	for rows.Next() {
		var sp model.SalesItemSpec
		var active int
		if err := rows.Scan(&sp.SpecID, &sp.ItemID, &sp.Spec, &sp.Unit, &sp.Price, &sp.SortOrder, &active); err != nil {
			return nil, err
		}
		sp.IsActive = active != 0
		out = append(out, sp)
	}
	return out, rows.Err()
}

func (r *SalesItemRepo) ListSupplierPrices(itemID string) ([]model.SalesItemSupplierPrice, error) {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return nil, nil
	}
	rows, err := r.db.Query(`
		SELECT sp_id, item_id, COALESCE(supplier_id,''), COALESCE(spec_id,''),
			COALESCE(unit,''), COALESCE(price,0), COALESCE(sort_order,0)
		FROM sales_item_supplier_prices WHERE item_id=?
		ORDER BY supplier_id, sort_order, sp_id`, itemID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.SalesItemSupplierPrice
	for rows.Next() {
		var p model.SalesItemSupplierPrice
		if err := rows.Scan(&p.SPID, &p.ItemID, &p.SupplierID, &p.SpecID, &p.Unit, &p.Price, &p.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *SalesItemRepo) LoadCatalog(p *model.SalesItem) error {
	if p == nil || strings.TrimSpace(p.ItemID) == "" {
		return nil
	}
	specs, err := r.ListSpecs(p.ItemID, true)
	if err != nil {
		return err
	}
	prices, err := r.ListSupplierPrices(p.ItemID)
	if err != nil {
		return err
	}
	p.Specs = specs
	p.Suppliers = groupSupplierPrices(r.db, prices)
	active := 0
	minP, maxP := 0, 0
	for _, sp := range specs {
		if !sp.IsActive {
			continue
		}
		active++
		if sp.Price <= 0 {
			continue
		}
		if minP == 0 || sp.Price < minP {
			minP = sp.Price
		}
		if sp.Price > maxP {
			maxP = sp.Price
		}
	}
	p.SpecCount = active
	p.PriceMin, p.PriceMax = minP, maxP
	return nil
}

func groupSupplierPrices(db *sql.DB, prices []model.SalesItemSupplierPrice) []model.SalesItemSupplierBlock {
	var blocks []model.SalesItemSupplierBlock
	idx := map[string]int{}
	for _, p := range prices {
		sid := strings.TrimSpace(p.SupplierID)
		if sid == "" {
			continue
		}
		i, ok := idx[sid]
		if !ok {
			name := ""
			if db != nil {
				_ = db.QueryRow(`SELECT COALESCE(org_name,'') FROM customers WHERE customer_id=?`, sid).Scan(&name)
			}
			idx[sid] = len(blocks)
			blocks = append(blocks, model.SalesItemSupplierBlock{SupplierID: sid, SupplierName: name})
			i = len(blocks) - 1
		}
		blocks[i].Rows = append(blocks[i].Rows, p)
	}
	return blocks
}

func (r *SalesItemRepo) AttachSpecSummaries(items []model.SalesItem) []model.SalesItem {
	if len(items) == 0 {
		return items
	}
	rows, err := r.db.Query(`
		SELECT item_id, COUNT(*), COALESCE(MIN(CASE WHEN price>0 THEN price END),0),
			COALESCE(MAX(price),0)
		FROM sales_item_specs WHERE COALESCE(is_active,1)=1
		GROUP BY item_id`)
	if err != nil {
		return items
	}
	defer rows.Close()
	type sum struct{ n, min, max int }
	m := map[string]sum{}
	for rows.Next() {
		var id string
		var s sum
		if err := rows.Scan(&id, &s.n, &s.min, &s.max); err != nil {
			continue
		}
		m[id] = s
	}
	for i := range items {
		if s, ok := m[items[i].ItemID]; ok {
			items[i].SpecCount, items[i].PriceMin, items[i].PriceMax = s.n, s.min, s.max
		}
	}
	return items
}

func (r *SalesItemRepo) specIDsUsed(ids []string) map[string]bool {
	used := map[string]bool{}
	if len(ids) == 0 {
		return used
	}
	ph := strings.Repeat("?,", len(ids))
	ph = strings.TrimSuffix(ph, ",")
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	q := `SELECT DISTINCT spec_id FROM sales_quote_lines WHERE spec_id IN (` + ph + `) AND TRIM(COALESCE(spec_id,''))!=''`
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return used
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil && id != "" {
			used[id] = true
		}
	}
	return used
}

func (r *SalesItemRepo) ReplaceSpecsAndPrices(itemID string, specs []model.SalesItemSpec, prices []model.SalesItemSupplierPrice) (deactivated []string, err error) {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return nil, fmt.Errorf("item_id 필요")
	}
	existing, err := r.ListSpecs(itemID, true)
	if err != nil {
		return nil, err
	}
	keep := map[string]bool{}
	for _, sp := range specs {
		id := strings.TrimSpace(sp.SpecID)
		if id != "" && !strings.HasPrefix(id, "tmp-") {
			keep[id] = true
		}
	}
	var oldIDs []string
	for _, sp := range existing {
		oldIDs = append(oldIDs, sp.SpecID)
	}
	used := r.specIDsUsed(oldIDs)

	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	idMap := map[string]string{}
	for _, sp := range existing {
		if keep[sp.SpecID] {
			idMap[sp.SpecID] = sp.SpecID
			continue
		}
		if used[sp.SpecID] {
			if _, err := tx.Exec(`UPDATE sales_item_specs SET is_active=0 WHERE spec_id=?`, sp.SpecID); err != nil {
				return nil, err
			}
			deactivated = append(deactivated, sp.SpecID)
			continue
		}
		if _, err := tx.Exec(`DELETE FROM sales_item_supplier_prices WHERE spec_id=?`, sp.SpecID); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(`DELETE FROM sales_item_specs WHERE spec_id=?`, sp.SpecID); err != nil {
			return nil, err
		}
	}

	for i := range specs {
		sp := specs[i]
		sp.ItemID = itemID
		sp.Spec = strings.TrimSpace(sp.Spec)
		sp.Unit = strings.TrimSpace(sp.Unit)
		if sp.Unit == "" {
			sp.Unit = "EA"
		}
		sp.SortOrder = i
		oldID := strings.TrimSpace(sp.SpecID)
		realID := idMap[oldID]
		if realID == "" {
			n, err := nextSeqTx(tx, "sales_item_specs")
			if err != nil {
				return nil, err
			}
			realID = fmt.Sprintf("SS-%03d", n)
			if _, err := tx.Exec(`
				INSERT INTO sales_item_specs (spec_id, item_id, spec, unit, price, sort_order, is_active)
				VALUES (?,?,?,?,?,?,1)`, realID, itemID, sp.Spec, sp.Unit, sp.Price, sp.SortOrder); err != nil {
				return nil, err
			}
		} else {
			if _, err := tx.Exec(`
				UPDATE sales_item_specs SET spec=?, unit=?, price=?, sort_order=?, is_active=1
				WHERE spec_id=?`, sp.Spec, sp.Unit, sp.Price, sp.SortOrder, realID); err != nil {
				return nil, err
			}
		}
		if oldID != "" {
			idMap[oldID] = realID
		}
		idMap[realID] = realID
	}

	if _, err := tx.Exec(`DELETE FROM sales_item_supplier_prices WHERE item_id=?`, itemID); err != nil {
		return nil, err
	}
	for i, p := range prices {
		sid := strings.TrimSpace(p.SupplierID)
		specID := strings.TrimSpace(p.SpecID)
		if mapped, ok := idMap[specID]; ok {
			specID = mapped
		}
		if sid == "" || specID == "" {
			continue
		}
		n, err := nextSeqTx(tx, "sales_item_supplier_prices")
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(`
			INSERT INTO sales_item_supplier_prices (sp_id, item_id, supplier_id, spec_id, unit, price, sort_order)
			VALUES (?,?,?,?,?,?,?)`,
			fmt.Sprintf("ISP-%03d", n), itemID, sid, specID, strings.TrimSpace(p.Unit), p.Price, i); err != nil {
			return nil, err
		}
	}
	return deactivated, tx.Commit()
}

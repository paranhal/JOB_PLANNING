package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"customer-support/internal/model"
)

func (r *MaintenanceRepo) ListInspectionProducts(orgID string, includeInactive bool) ([]model.InspectionProduct, error) {
	orgID = strings.TrimSpace(orgID)
	if err := RequireOrgID(orgID); err != nil || orgID == OrgAll {
		return nil, ErrEmptyOrgID
	}
	q := `SELECT product_id, org_id, code, name, sort_order, is_active,
	             COALESCE(created_at,''), COALESCE(updated_at,'')
	      FROM inspection_products WHERE org_id=?`
	if !includeInactive {
		q += ` AND is_active=1`
	}
	q += ` ORDER BY sort_order, name`
	rows, err := r.db.Query(q, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.InspectionProduct
	for rows.Next() {
		var p model.InspectionProduct
		var active int
		if err := rows.Scan(&p.ProductID, &p.OrgID, &p.Code, &p.Name, &p.SortOrder, &active, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		p.IsActive = active == 1
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		out[i].InUse = r.inspectionProductInUse(out[i])
	}
	return out, nil
}

func (r *MaintenanceRepo) CountInspectionProducts(orgID string) int {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" || orgID == OrgAll {
		return 0
	}
	var n int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM inspection_products WHERE org_id=?`, orgID).Scan(&n)
	return n
}

func (r *MaintenanceRepo) GetInspectionProduct(id string) (*model.InspectionProduct, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil
	}
	var p model.InspectionProduct
	var active int
	err := r.db.QueryRow(`SELECT product_id, org_id, code, name, sort_order, is_active,
		COALESCE(created_at,''), COALESCE(updated_at,'')
		FROM inspection_products WHERE product_id=?`, id).
		Scan(&p.ProductID, &p.OrgID, &p.Code, &p.Name, &p.SortOrder, &active, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.IsActive = active == 1
	p.InUse = r.inspectionProductInUse(p)
	return &p, nil
}

func (r *MaintenanceRepo) inspectionProductInUse(p model.InspectionProduct) bool {
	var n int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM site_inspection_products WHERE product_id=?`, p.ProductID).Scan(&n)
	if n > 0 {
		return true
	}
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM maintenance_visits
		WHERE TRIM(COALESCE(product_type,'')) IN (?, ?)`, p.Name, p.Code).Scan(&n)
	return n > 0
}

func (r *MaintenanceRepo) CreateInspectionProduct(p *model.InspectionProduct) error {
	if p == nil {
		return fmt.Errorf("제품이 없습니다")
	}
	org, err := RequireInsertOrg(p.OrgID)
	if err != nil {
		return err
	}
	p.OrgID = org
	p.Code = strings.TrimSpace(p.Code)
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return fmt.Errorf("제품 이름은 필수입니다")
	}
	if !model.ValidInspectionProductCode(p.Code) {
		return fmt.Errorf("코드는 영문·숫자·밑줄 1~30자입니다")
	}
	if p.ProductID == "" {
		p.ProductID = newID("IP")
	}
	now := time.Now().Format(time.RFC3339)
	p.CreatedAt, p.UpdatedAt = now, now
	active := 1
	if !p.IsActive {
		active = 0
	}
	_, err = r.db.Exec(`INSERT INTO inspection_products
		(product_id, org_id, code, name, sort_order, is_active, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?)`,
		p.ProductID, p.OrgID, p.Code, p.Name, p.SortOrder, active, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return fmt.Errorf("같은 조직에 이미 있는 코드입니다")
		}
		return err
	}
	logCreateWithReason(r.db, "inspection_products", "product_id", p.ProductID, p.Name, "점검제품 등록")
	return nil
}

func (r *MaintenanceRepo) UpdateInspectionProduct(p *model.InspectionProduct) error {
	if p == nil {
		return fmt.Errorf("제품이 없습니다")
	}
	cur, err := r.GetInspectionProduct(p.ProductID)
	if err != nil || cur == nil {
		return fmt.Errorf("제품이 없습니다")
	}
	p.Name = strings.TrimSpace(p.Name)
	p.Code = strings.TrimSpace(p.Code)
	if p.Name == "" {
		return fmt.Errorf("제품 이름은 필수입니다")
	}
	if !model.ValidInspectionProductCode(p.Code) {
		return fmt.Errorf("코드는 영문·숫자·밑줄 1~30자입니다")
	}
	if cur.InUse && !strings.EqualFold(cur.Code, p.Code) {
		return fmt.Errorf("사용 중인 제품의 코드는 바꿀 수 없습니다")
	}
	now := time.Now().Format(time.RFC3339)
	_, err = r.db.Exec(`UPDATE inspection_products SET code=?, name=?, sort_order=?, updated_at=? WHERE product_id=?`,
		p.Code, p.Name, p.SortOrder, now, p.ProductID)
	return err
}

func (r *MaintenanceRepo) SetInspectionProductActive(id string, active bool) error {
	v := 0
	if active {
		v = 1
	}
	_, err := r.db.Exec(`UPDATE inspection_products SET is_active=?, updated_at=? WHERE product_id=?`,
		v, time.Now().Format(time.RFC3339), id)
	if err == nil {
		r.invalidateOpenUnassignedSlotsCache()
	}
	return err
}

func (r *MaintenanceRepo) ReplaceSiteProducts(customerID, orgID string, productIDs []string) error {
	customerID = strings.TrimSpace(customerID)
	orgID = strings.TrimSpace(orgID)
	if customerID == "" {
		return fmt.Errorf("고객이 없습니다")
	}
	if err := RequireOrgID(orgID); err != nil || orgID == OrgAll {
		return ErrEmptyOrgID
	}
	seen := map[string]bool{}
	var ids []string
	for _, id := range productIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		p, err := r.GetInspectionProduct(id)
		if err != nil {
			return err
		}
		if p == nil || p.OrgID != orgID {
			return errOtherOrgInspectionProduct
		}
		seen[id] = true
		ids = append(ids, id)
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM site_inspection_products WHERE customer_id=?`, customerID); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := tx.Exec(`INSERT INTO site_inspection_products (customer_id, product_id) VALUES (?,?)`, customerID, id); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	r.invalidateOpenUnassignedSlotsCache()
	return nil
}

var errOtherOrgInspectionProduct = fmt.Errorf("다른 조직의 점검 제품입니다")

func IsOtherOrgInspectionProduct(err error) bool {
	return err != nil && strings.Contains(err.Error(), "다른 조직의 점검 제품")
}

func (r *MaintenanceRepo) slotCatalogForCustomer(customerID string) []model.InspectionProduct {
	cfg, err := r.GetSiteConfig(customerID)
	if err != nil || cfg == nil {
		return nil
	}
	return cfg.Products
}

func (r *MaintenanceRepo) visitSlotKey(customerID, product string) string {
	return model.VisitProductSlotKeyFor(product, r.slotCatalogForCustomer(customerID))
}

func (r *MaintenanceRepo) hydrateSiteConfigs(cfgs []model.MaintenanceSiteConfig) error {
	if len(cfgs) == 0 {
		return nil
	}
	orgCount := map[string]int{}
	rows, err := r.db.Query(`SELECT org_id, COUNT(*) FROM inspection_products GROUP BY org_id`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var org string
		var n int
		if err := rows.Scan(&org, &n); err != nil {
			rows.Close()
			return err
		}
		orgCount[org] = n
	}
	rows.Close()

	type link struct {
		CustomerID string
		P          model.InspectionProduct
	}
	lrows, err := r.db.Query(`
		SELECT s.customer_id, p.product_id, p.org_id, p.code, p.name, p.sort_order, p.is_active
		FROM site_inspection_products s
		JOIN inspection_products p ON p.product_id = s.product_id
		ORDER BY p.sort_order, p.name`)
	if err != nil {
		return err
	}
	byCust := map[string][]model.InspectionProduct{}
	for lrows.Next() {
		var customerID string
		var p model.InspectionProduct
		var active int
		if err := lrows.Scan(&customerID, &p.ProductID, &p.OrgID, &p.Code, &p.Name, &p.SortOrder, &active); err != nil {
			lrows.Close()
			return err
		}
		p.IsActive = active == 1
		byCust[customerID] = append(byCust[customerID], p)
	}
	lrows.Close()
	if err := lrows.Err(); err != nil {
		return err
	}

	for i := range cfgs {
		if model.UseLegacyInspectionSlots(cfgs[i].OrgID, orgCount[cfgs[i].OrgID]) {
			cfgs[i].Products = nil
			continue
		}
		if linked, ok := byCust[cfgs[i].CustomerID]; ok {
			cfgs[i].Products = linked
		} else {
			cfgs[i].Products = []model.InspectionProduct{}
		}
	}
	return nil
}

func (r *MaintenanceRepo) ImportLegacyLibraryProducts() (int, error) {
	org := model.OrgIDLibrary
	specs := []struct{ Code, Name string }{
		{"klas", "KLAS"},
		{"rfid", "앤로보틱스"},
	}
	existing, err := r.ListInspectionProducts(org, true)
	if err != nil {
		return 0, err
	}
	byCode := map[string]model.InspectionProduct{}
	for _, p := range existing {
		byCode[strings.ToLower(p.Code)] = p
	}
	created := 0
	for i, spec := range specs {
		if _, ok := byCode[spec.Code]; ok {
			continue
		}
		p := &model.InspectionProduct{
			OrgID: org, Code: spec.Code, Name: spec.Name, SortOrder: (i + 1) * 10, IsActive: true,
		}
		if err := r.CreateInspectionProduct(p); err != nil {
			return created, err
		}
		byCode[spec.Code] = *p
		created++
	}
	klasID := byCode["klas"].ProductID
	rfidID := byCode["rfid"].ProductID
	cfgs, err := r.ListSiteConfigs()
	if err != nil {
		return created, err
	}
	for _, c := range cfgs {
		if c.OrgID != "" && c.OrgID != org {
			continue
		}
		var ids []string
		if c.HasKlas && klasID != "" {
			ids = append(ids, klasID)
		}
		if c.HasRfid && rfidID != "" {
			ids = append(ids, rfidID)
		}
		if len(ids) == 0 {
			continue
		}
		cur := map[string]bool{}
		for _, p := range c.Products {
			cur[p.ProductID] = true
		}
		merged := ids
		for _, p := range c.Products {
			if !cur[p.ProductID] {
				continue
			}
			skip := false
			for _, id := range merged {
				if id == p.ProductID {
					skip = true
					break
				}
			}
			if !skip {
				merged = append(merged, p.ProductID)
			}
		}
		if err := r.ReplaceSiteProducts(c.CustomerID, org, merged); err != nil {
			return created, err
		}
	}
	logCreateWithReason(r.db, "inspection_products", "org_id", org, "KLAS·앤로보틱스", "기본 제품 가져오기")
	return created, nil
}

func FilterSiteConfigsByOrg(items []model.MaintenanceSiteConfig, orgID string) []model.MaintenanceSiteConfig {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" || orgID == OrgAll {
		return items
	}
	var out []model.MaintenanceSiteConfig
	for _, it := range items {
		row := strings.TrimSpace(it.OrgID)
		if row == orgID || (row == "" && orgID == model.OrgIDLibrary) {
			out = append(out, it)
		}
	}
	return out
}

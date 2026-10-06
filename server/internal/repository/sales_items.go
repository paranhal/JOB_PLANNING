package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"customer-support/internal/model"
)

type SalesItemRepo struct {
	db *sql.DB
}

func NewSalesItemRepo(db *sql.DB) *SalesItemRepo {
	return &SalesItemRepo{db: db}
}

const salesItemSelect = `
	SELECT i.item_id, COALESCE(i.item_kind,'goods'), COALESCE(i.category,''), i.name,
		COALESCE(i.spec,''), COALESCE(i.model,''), COALESCE(i.manufacturer,''),
		COALESCE(i.unit,'EA'), COALESCE(i.gov_item_no,''),
		COALESCE(i.list_price,0), COALESCE(i.last_price,0),
		COALESCE(i.last_quoted_at,''), COALESCE(i.last_customer,''),
		COALESCE(i.default_supplier,''), COALESCE(i.is_active,1), COALESCE(i.needs_review,0),
		COALESCE(i.notes,''), COALESCE(i.created_at,''), COALESCE(i.updated_at,''),
		COALESCE(i.manufacturer_id,''), COALESCE(i.supplier_id,''),
		COALESCE(m.org_name,''), COALESCE(s.org_name,'')
	FROM sales_items i
	LEFT JOIN customers m ON m.customer_id = i.manufacturer_id
	LEFT JOIN customers s ON s.customer_id = i.supplier_id`

type SalesItemFilter struct {
	Search   string
	Kind     string
	Category string
	Review   string // 1 = 확인 필요만
	Active   string // 1 / 0
}

func (r *SalesItemRepo) List(f SalesItemFilter) ([]model.SalesItem, error) {
	q := salesItemSelect + ` WHERE 1=1`
	var args []interface{}
	if s := strings.TrimSpace(f.Search); s != "" {
		like := "%" + s + "%"
		q += ` AND (i.name LIKE ? OR COALESCE(i.spec,'') LIKE ? OR COALESCE(i.model,'') LIKE ?
			OR COALESCE(i.manufacturer,'') LIKE ? OR COALESCE(i.default_supplier,'') LIKE ?
			OR COALESCE(m.org_name,'') LIKE ? OR COALESCE(s.org_name,'') LIKE ?
			OR EXISTS (SELECT 1 FROM sales_item_specs sp WHERE sp.item_id=i.item_id AND COALESCE(sp.spec,'') LIKE ?))`
		args = append(args, like, like, like, like, like, like, like, like)
	}
	if s := strings.TrimSpace(f.Kind); s != "" {
		q += ` AND i.item_kind=?`
		args = append(args, model.NormalizeSalesItemKind(s))
	}
	if s := strings.TrimSpace(f.Category); s != "" {
		q += ` AND i.category=?`
		args = append(args, s)
	}
	switch strings.TrimSpace(f.Review) {
	case "1", "yes", "true":
		q += ` AND COALESCE(i.needs_review,0)=1`
	case "0", "no", "false":
		q += ` AND COALESCE(i.needs_review,0)=0`
	}
	switch strings.TrimSpace(f.Active) {
	case "1", "yes", "true":
		q += ` AND COALESCE(i.is_active,0)=1`
	case "0", "no", "false":
		q += ` AND COALESCE(i.is_active,0)=0`
	}
	q += ` ORDER BY COALESCE(i.needs_review,0) DESC, i.name COLLATE NOCASE, i.item_id`
	rows, err := r.db.Query(q, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	return scanSalesItems(rows)
}

func (r *SalesItemRepo) Get(id string) (*model.SalesItem, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, sql.ErrNoRows
	}
	row := r.db.QueryRow(salesItemSelect+` WHERE i.item_id=?`, id)
	return scanSalesItem(row)
}

func (r *SalesItemRepo) FindByName(name string) (*model.SalesItem, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, sql.ErrNoRows
	}
	row := r.db.QueryRow(salesItemSelect+`
		WHERE lower(i.name)=lower(?)
		ORDER BY COALESCE(i.is_active,0) DESC, i.item_id LIMIT 1`, name)
	return scanSalesItem(row)
}

func (r *SalesItemRepo) Suggest(q string, limit int) ([]model.SalesItem, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	q = strings.TrimSpace(q)
	sqlQ := salesItemSelect + ` WHERE 1=1`
	var args []interface{}
	if q != "" {
		like := "%" + q + "%"
		sqlQ += ` AND (i.name LIKE ? OR COALESCE(i.spec,'') LIKE ? OR COALESCE(i.model,'') LIKE ?
			OR COALESCE(i.manufacturer,'') LIKE ? OR COALESCE(i.default_supplier,'') LIKE ?
			OR COALESCE(m.org_name,'') LIKE ? OR COALESCE(s.org_name,'') LIKE ?
			OR EXISTS (SELECT 1 FROM sales_item_specs sp WHERE sp.item_id=i.item_id AND COALESCE(sp.spec,'') LIKE ?))`
		args = append(args, like, like, like, like, like, like, like, like)
	}
	sqlQ += ` ORDER BY COALESCE(i.is_active,0) DESC, COALESCE(i.needs_review,0) ASC, i.name COLLATE NOCASE LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.Query(sqlQ, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	return scanSalesItems(rows)
}

func (r *SalesItemRepo) Create(p *model.SalesItem) error {
	if p == nil {
		return fmt.Errorf("품목이 필요합니다")
	}
	normalizeSalesItem(p)
	if p.Name == "" {
		return fmt.Errorf("품명을 입력하세요")
	}
	n, err := NextSeq(r.db, "sales_items")
	if err != nil {
		return err
	}
	p.ItemID = fmt.Sprintf("SI-%03d", n)
	_, err = r.db.Exec(`
		INSERT INTO sales_items (
			item_id, item_kind, category, name, spec, model, manufacturer, manufacturer_id, supplier_id, unit, gov_item_no,
			list_price, last_price, last_quoted_at, last_customer, default_supplier,
			is_active, needs_review, notes
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.ItemID, p.ItemKind, p.Category, p.Name, p.Spec, p.Model, p.Manufacturer, p.ManufacturerID, p.SupplierID, p.Unit, p.GovItemNo,
		p.ListPrice, p.LastPrice, p.LastQuotedAt, p.LastCustomer, p.DefaultSupplier,
		boolToInt(p.IsActive), boolToInt(p.NeedsReview), p.Notes)
	if err != nil {
		return err
	}
	logCreate(r.db, "sales_items", "item_id", p.ItemID, p.Name)
	r.insertDefaultSpecIfNeeded(p)
	return nil
}

func (r *SalesItemRepo) Update(p *model.SalesItem) error {
	if p == nil || strings.TrimSpace(p.ItemID) == "" {
		return fmt.Errorf("item_id 필요")
	}
	normalizeSalesItem(p)
	if p.Name == "" {
		return fmt.Errorf("품명을 입력하세요")
	}
	return touchUpdate(r.db, "sales_items", "item_id", p.ItemID, p.Name, func() error {
		_, err := r.db.Exec(`
			UPDATE sales_items SET
				item_kind=?, category=?, name=?, spec=?, model=?, manufacturer=?, manufacturer_id=?, supplier_id=?, unit=?, gov_item_no=?,
				list_price=?, default_supplier=?,
				is_active=?, needs_review=?, notes=?, updated_at=CURRENT_TIMESTAMP
			WHERE item_id=?`,
			p.ItemKind, p.Category, p.Name, p.Spec, p.Model, p.Manufacturer, p.ManufacturerID, p.SupplierID, p.Unit, p.GovItemNo,
			p.ListPrice, p.DefaultSupplier,
			boolToInt(p.IsActive), boolToInt(p.NeedsReview), p.Notes, p.ItemID)
		return err
	})
}

func (r *SalesItemRepo) SetKind(id, kind string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("item_id 필요")
	}
	kind = model.NormalizeSalesItemKind(kind)
	p, err := r.Get(id)
	if err != nil {
		return err
	}
	if p.ItemKind == kind {
		return nil
	}
	p.ItemKind = kind
	return r.Update(p)
}

func (r *SalesItemRepo) Confirm(id string) error {
	p, err := r.Get(id)
	if err != nil {
		return err
	}
	p.NeedsReview = false
	p.IsActive = true
	return r.Update(p)
}

func normalizeSalesItem(p *model.SalesItem) {
	p.Name = strings.TrimSpace(p.Name)
	p.Spec = strings.TrimSpace(p.Spec)
	p.Model = strings.TrimSpace(p.Model)
	p.Manufacturer = strings.TrimSpace(p.Manufacturer)
	p.ManufacturerID = strings.TrimSpace(p.ManufacturerID)
	p.SupplierID = strings.TrimSpace(p.SupplierID)
	p.GovItemNo = strings.TrimSpace(p.GovItemNo)
	p.DefaultSupplier = strings.TrimSpace(p.DefaultSupplier)
	p.Notes = strings.TrimSpace(p.Notes)
	p.LastCustomer = strings.TrimSpace(p.LastCustomer)
	p.LastQuotedAt = strings.TrimSpace(p.LastQuotedAt)
	p.ItemKind = model.NormalizeSalesItemKind(p.ItemKind)
	p.Category = strings.TrimSpace(p.Category)
	p.Unit = strings.TrimSpace(p.Unit)
	if p.Unit == "" {
		p.Unit = "EA"
	}
}

func (r *SalesItemRepo) insertDefaultSpecIfNeeded(p *model.SalesItem) {
	if r == nil || p == nil || strings.TrimSpace(p.ItemID) == "" {
		return
	}
	if strings.TrimSpace(p.Spec) == "" && p.ListPrice == 0 {
		return
	}
	var n int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM sales_item_specs WHERE item_id=?`, p.ItemID).Scan(&n); err != nil || n > 0 {
		return
	}
	seq, err := NextSeq(r.db, "sales_item_specs")
	if err != nil {
		return
	}
	unit := strings.TrimSpace(p.Unit)
	if unit == "" {
		unit = "EA"
	}
	_, _ = r.db.Exec(`
		INSERT INTO sales_item_specs (spec_id, item_id, spec, unit, price, sort_order, is_active)
		VALUES (?,?,?,?,?,0,1)`, fmt.Sprintf("SS-%03d", seq), p.ItemID, strings.TrimSpace(p.Spec), unit, p.ListPrice)
}

type salesItemScanner interface {
	Scan(dest ...interface{}) error
}

func scanSalesItem(row salesItemScanner) (*model.SalesItem, error) {
	var p model.SalesItem
	var active, review int
	err := row.Scan(
		&p.ItemID, &p.ItemKind, &p.Category, &p.Name,
		&p.Spec, &p.Model, &p.Manufacturer, &p.Unit, &p.GovItemNo,
		&p.ListPrice, &p.LastPrice, &p.LastQuotedAt, &p.LastCustomer, &p.DefaultSupplier,
		&active, &review, &p.Notes, &p.CreatedAt, &p.UpdatedAt,
		&p.ManufacturerID, &p.SupplierID, &p.MfrPartyName, &p.SupPartyName,
	)
	if err != nil {
		return nil, err
	}
	p.IsActive = active != 0
	p.NeedsReview = review != 0
	return &p, nil
}

func scanSalesItems(rows *sql.Rows) ([]model.SalesItem, error) {
	var items []model.SalesItem
	for rows.Next() {
		p, err := scanSalesItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *p)
	}
	return items, rows.Err()
}

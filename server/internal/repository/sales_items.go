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
	SELECT item_id, COALESCE(item_kind,'goods'), COALESCE(category,''), name,
		COALESCE(spec,''), COALESCE(model,''), COALESCE(manufacturer,''),
		COALESCE(unit,'EA'), COALESCE(gov_item_no,''),
		COALESCE(list_price,0), COALESCE(last_price,0),
		COALESCE(last_quoted_at,''), COALESCE(last_customer,''),
		COALESCE(default_supplier,''), COALESCE(is_active,1), COALESCE(needs_review,0),
		COALESCE(notes,''), COALESCE(created_at,''), COALESCE(updated_at,'')
	FROM sales_items`

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
		q += ` AND (name LIKE ? OR COALESCE(spec,'') LIKE ? OR COALESCE(model,'') LIKE ? OR COALESCE(manufacturer,'') LIKE ?)`
		args = append(args, like, like, like, like)
	}
	if s := strings.TrimSpace(f.Kind); s != "" {
		q += ` AND item_kind=?`
		args = append(args, model.NormalizeSalesItemKind(s))
	}
	if s := strings.TrimSpace(f.Category); s != "" {
		q += ` AND category=?`
		args = append(args, s)
	}
	switch strings.TrimSpace(f.Review) {
	case "1", "yes", "true":
		q += ` AND COALESCE(needs_review,0)=1`
	case "0", "no", "false":
		q += ` AND COALESCE(needs_review,0)=0`
	}
	switch strings.TrimSpace(f.Active) {
	case "1", "yes", "true":
		q += ` AND COALESCE(is_active,0)=1`
	case "0", "no", "false":
		q += ` AND COALESCE(is_active,0)=0`
	}
	q += ` ORDER BY COALESCE(needs_review,0) DESC, name COLLATE NOCASE, item_id`
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
	row := r.db.QueryRow(salesItemSelect+` WHERE item_id=?`, id)
	return scanSalesItem(row)
}

func (r *SalesItemRepo) FindByName(name string) (*model.SalesItem, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, sql.ErrNoRows
	}
	row := r.db.QueryRow(salesItemSelect+`
		WHERE lower(name)=lower(?)
		ORDER BY COALESCE(is_active,0) DESC, item_id LIMIT 1`, name)
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
		sqlQ += ` AND (name LIKE ? OR COALESCE(spec,'') LIKE ? OR COALESCE(model,'') LIKE ? OR COALESCE(manufacturer,'') LIKE ?)`
		args = append(args, like, like, like, like)
	}
	sqlQ += ` ORDER BY COALESCE(is_active,0) DESC, COALESCE(needs_review,0) ASC, name COLLATE NOCASE LIMIT ?`
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
			item_id, item_kind, category, name, spec, model, manufacturer, unit, gov_item_no,
			list_price, last_price, last_quoted_at, last_customer, default_supplier,
			is_active, needs_review, notes
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.ItemID, p.ItemKind, p.Category, p.Name, p.Spec, p.Model, p.Manufacturer, p.Unit, p.GovItemNo,
		p.ListPrice, p.LastPrice, p.LastQuotedAt, p.LastCustomer, p.DefaultSupplier,
		boolToInt(p.IsActive), boolToInt(p.NeedsReview), p.Notes)
	if err != nil {
		return err
	}
	logCreate(r.db, "sales_items", "item_id", p.ItemID, p.Name)
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
				item_kind=?, category=?, name=?, spec=?, model=?, manufacturer=?, unit=?, gov_item_no=?,
				list_price=?, last_price=?, last_quoted_at=?, last_customer=?, default_supplier=?,
				is_active=?, needs_review=?, notes=?, updated_at=CURRENT_TIMESTAMP
			WHERE item_id=?`,
			p.ItemKind, p.Category, p.Name, p.Spec, p.Model, p.Manufacturer, p.Unit, p.GovItemNo,
			p.ListPrice, p.LastPrice, p.LastQuotedAt, p.LastCustomer, p.DefaultSupplier,
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

func (r *SalesItemRepo) TouchLastQuoted(id string, price int, customer, quotedAt string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("item_id 필요")
	}
	if _, err := r.Get(id); err != nil {
		return err
	}
	customer = strings.TrimSpace(customer)
	quotedAt = strings.TrimSpace(quotedAt)
	_, err := r.db.Exec(`
		UPDATE sales_items SET last_price=?, last_quoted_at=?, last_customer=?, updated_at=CURRENT_TIMESTAMP
		WHERE item_id=?`, price, quotedAt, customer, id)
	return err
}

func normalizeSalesItem(p *model.SalesItem) {
	p.Name = strings.TrimSpace(p.Name)
	p.Spec = strings.TrimSpace(p.Spec)
	p.Model = strings.TrimSpace(p.Model)
	p.Manufacturer = strings.TrimSpace(p.Manufacturer)
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

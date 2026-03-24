package repository

import (
	"database/sql"
	"fmt"
	"time"

	"customer-support/internal/model"
)

type CustomerRepo struct {
	db *sql.DB
}

func NewCustomerRepo(db *sql.DB) *CustomerRepo {
	return &CustomerRepo{db: db}
}

// List 고객 목록 조회 (검색, 페이징)
func (r *CustomerRepo) List(search string, page, pageSize int) ([]model.CustomerListItem, int, error) {
	offset := (page - 1) * pageSize

	baseQuery := `
		SELECT c.customer_id, c.org_name, c.official_name,
		       COALESCE(c.industry,''), COALESCE(c.main_phone,''), c.is_active,
		       COUNT(DISTINCT a.asset_id) AS asset_count,
		       COUNT(DISTINCT ar.as_id) AS as_count
		FROM customers c
		LEFT JOIN assets a ON a.customer_id = c.customer_id
		LEFT JOIN as_receipts ar ON ar.customer_id = c.customer_id
		WHERE 1=1`

	countQuery := `SELECT COUNT(*) FROM customers WHERE 1=1`
	args := []interface{}{}
	countArgs := []interface{}{}

	if search != "" {
		like := "%" + search + "%"
		baseQuery += ` AND (c.org_name LIKE ? OR c.official_name LIKE ? OR c.main_phone LIKE ?)`
		countQuery += ` AND (org_name LIKE ? OR official_name LIKE ? OR main_phone LIKE ?)`
		args = append(args, like, like, like)
		countArgs = append(countArgs, like, like, like)
	}

	baseQuery += ` GROUP BY c.customer_id ORDER BY c.org_name LIMIT ? OFFSET ?`
	args = append(args, pageSize, offset)

	var total int
	if err := r.db.QueryRow(countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.db.Query(baseQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []model.CustomerListItem
	for rows.Next() {
		var item model.CustomerListItem
		var isActive int
		if err := rows.Scan(
			&item.CustomerID, &item.OrgName, &item.OfficialName,
			&item.Industry, &item.MainPhone, &isActive,
			&item.AssetCount, &item.AsCount,
		); err != nil {
			return nil, 0, err
		}
		item.IsActive = isActive == 1
		items = append(items, item)
	}
	return items, total, rows.Err()
}

// GetByID 고객 단건 조회
func (r *CustomerRepo) GetByID(id string) (*model.Customer, error) {
	query := `
		SELECT customer_id, org_name, official_name,
		       COALESCE(org_email,''), COALESCE(main_phone,''),
		       COALESCE(website,''), COALESCE(business_number,''),
		       COALESCE(representative,''), COALESCE(industry,''),
		       has_parent, COALESCE(parent_customer_id,''),
		       COALESCE(address,''), COALESCE(address_detail,''),
		       is_active, COALESCE(notes,''), created_at, updated_at
		FROM customers WHERE customer_id = ?`

	var c model.Customer
	var hasParent, isActive int
	var createdAt, updatedAt string

	err := r.db.QueryRow(query, id).Scan(
		&c.CustomerID, &c.OrgName, &c.OfficialName, &c.OrgEmail,
		&c.MainPhone, &c.Website, &c.BusinessNumber, &c.Representative,
		&c.Industry, &hasParent, &c.ParentCustomerID,
		&c.Address, &c.AddressDetail,
		&isActive, &c.Notes, &createdAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.HasParent = hasParent == 1
	c.IsActive = isActive == 1
	c.CreatedAt = parseTime(createdAt)
	c.UpdatedAt = parseTime(updatedAt)
	return &c, nil
}

// Create 고객 등록
func (r *CustomerRepo) Create(c *model.Customer) error {
	c.CustomerID = newID("CUST")
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := r.db.Exec(`
		INSERT INTO customers (
			customer_id, org_name, official_name, org_email, main_phone,
			website, business_number, representative, industry,
			has_parent, parent_customer_id, address, address_detail,
			is_active, notes, created_at, updated_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.CustomerID, c.OrgName, c.OfficialName, c.OrgEmail, c.MainPhone,
		c.Website, c.BusinessNumber, c.Representative, c.Industry,
		boolToInt(c.HasParent), nullStr(c.ParentCustomerID),
		c.Address, c.AddressDetail,
		boolToInt(c.IsActive), c.Notes, now, now,
	)
	return err
}

// Update 고객 수정
func (r *CustomerRepo) Update(c *model.Customer) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := r.db.Exec(`
		UPDATE customers SET
			org_name=?, official_name=?, org_email=?, main_phone=?,
			website=?, business_number=?, representative=?, industry=?,
			has_parent=?, parent_customer_id=?, address=?, address_detail=?,
			is_active=?, notes=?, updated_at=?
		WHERE customer_id=?`,
		c.OrgName, c.OfficialName, c.OrgEmail, c.MainPhone,
		c.Website, c.BusinessNumber, c.Representative, c.Industry,
		boolToInt(c.HasParent), nullStr(c.ParentCustomerID),
		c.Address, c.AddressDetail,
		boolToInt(c.IsActive), c.Notes, now, c.CustomerID,
	)
	return err
}

// Delete 고객 비활성화 (실제 삭제 안 함)
func (r *CustomerRepo) Delete(id string) error {
	_, err := r.db.Exec(
		`UPDATE customers SET is_active=0, updated_at=? WHERE customer_id=?`,
		time.Now().Format("2006-01-02 15:04:05"), id,
	)
	return err
}

// ListAll 전체 고객 목록 (드롭다운용)
func (r *CustomerRepo) ListAll() ([]model.Customer, error) {
	rows, err := r.db.Query(
		`SELECT customer_id, org_name FROM customers WHERE is_active=1 ORDER BY org_name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var customers []model.Customer
	for rows.Next() {
		var c model.Customer
		if err := rows.Scan(&c.CustomerID, &c.OrgName); err != nil {
			return nil, err
		}
		customers = append(customers, c)
	}
	return customers, rows.Err()
}

// ── 유틸 ──────────────────────────────────────────────────────────

func newID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func parseTime(s string) time.Time {
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05.999999",
		time.RFC3339,
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

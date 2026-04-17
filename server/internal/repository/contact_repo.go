package repository

import (
	"database/sql"
	"fmt"
	"time"

	"customer-support/internal/model"
)

type ContactRepo struct {
	db *sql.DB
}

func NewContactRepo(db *sql.DB) *ContactRepo {
	return &ContactRepo{db: db}
}

// List 담당자 목록 조회 (검색, 페이징)
func (r *ContactRepo) List(search string, page, pageSize int) ([]model.Contact, int, error) {
	offset := (page - 1) * pageSize

	baseQuery := `
		SELECT c.contact_id, c.customer_id, c.full_name,
		       COALESCE(c.job_role,''), COALESCE(c.title,''),
		       COALESCE(c.phone,''), COALESCE(c.mobile,''),
		       COALESCE(c.email,''), COALESCE(c.start_date,''),
		       COALESCE(c.end_date,''), COALESCE(c.status,'active'),
		       c.is_primary, COALESCE(c.notes,''),
		       cu.org_name
		FROM contacts c
		JOIN customers cu ON cu.customer_id = c.customer_id
		WHERE 1=1`

	countQuery := `
		SELECT COUNT(*)
		FROM contacts c
		JOIN customers cu ON cu.customer_id = c.customer_id
		WHERE 1=1`

	args := []interface{}{}
	countArgs := []interface{}{}

	if search != "" {
		like := "%" + search + "%"
		filter := ` AND (c.full_name LIKE ? OR cu.org_name LIKE ? OR c.phone LIKE ? OR c.mobile LIKE ? OR c.email LIKE ?)`
		baseQuery += filter
		countQuery += filter
		args = append(args, like, like, like, like, like)
		countArgs = append(countArgs, like, like, like, like, like)
	}

	baseQuery += ` ORDER BY cu.org_name, c.full_name LIMIT ? OFFSET ?`
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

	var items []model.Contact
	for rows.Next() {
		var item model.Contact
		var isPrimary int
		if err := rows.Scan(
			&item.ContactID, &item.CustomerID, &item.FullName,
			&item.JobRole, &item.Title,
			&item.Phone, &item.Mobile,
			&item.Email, &item.StartDate,
			&item.EndDate, &item.Status,
			&isPrimary, &item.Notes,
			&item.OrgName,
		); err != nil {
			return nil, 0, err
		}
		item.IsPrimary = isPrimary == 1
		items = append(items, item)
	}
	return items, total, rows.Err()
}

// ListByCustomer 특정 고객의 담당자 목록
func (r *ContactRepo) ListByCustomer(customerID string) ([]model.Contact, error) {
	query := `
		SELECT contact_id, customer_id, full_name,
		       COALESCE(job_role,''), COALESCE(title,''),
		       COALESCE(phone,''), COALESCE(mobile,''),
		       COALESCE(email,''), COALESCE(start_date,''),
		       COALESCE(end_date,''), COALESCE(status,'active'),
		       is_primary, COALESCE(notes,'')
		FROM contacts
		WHERE customer_id = ?
		ORDER BY is_primary DESC, full_name`

	rows, err := r.db.Query(query, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []model.Contact
	for rows.Next() {
		var item model.Contact
		var isPrimary int
		if err := rows.Scan(
			&item.ContactID, &item.CustomerID, &item.FullName,
			&item.JobRole, &item.Title,
			&item.Phone, &item.Mobile,
			&item.Email, &item.StartDate,
			&item.EndDate, &item.Status,
			&isPrimary, &item.Notes,
		); err != nil {
			return nil, err
		}
		item.IsPrimary = isPrimary == 1
		items = append(items, item)
	}
	return items, rows.Err()
}

// GetByID 담당자 단건 조회
func (r *ContactRepo) GetByID(id string) (*model.Contact, error) {
	query := `
		SELECT c.contact_id, c.customer_id, c.full_name,
		       COALESCE(c.job_role,''), COALESCE(c.title,''),
		       COALESCE(c.phone,''), COALESCE(c.mobile,''),
		       COALESCE(c.email,''), COALESCE(c.start_date,''),
		       COALESCE(c.end_date,''), COALESCE(c.status,'active'),
		       c.is_primary, COALESCE(c.notes,''),
		       cu.org_name
		FROM contacts c
		JOIN customers cu ON cu.customer_id = c.customer_id
		WHERE c.contact_id = ?`

	var ct model.Contact
	var isPrimary int
	err := r.db.QueryRow(query, id).Scan(
		&ct.ContactID, &ct.CustomerID, &ct.FullName,
		&ct.JobRole, &ct.Title,
		&ct.Phone, &ct.Mobile,
		&ct.Email, &ct.StartDate,
		&ct.EndDate, &ct.Status,
		&isPrimary, &ct.Notes,
		&ct.OrgName,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ct.IsPrimary = isPrimary == 1
	return &ct, nil
}

// Create 담당자 등록
func (r *ContactRepo) Create(ct *model.Contact) error {
	ct.ContactID = fmt.Sprintf("CONT-%d", time.Now().UnixNano())
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := r.db.Exec(`
		INSERT INTO contacts (
			contact_id, customer_id, full_name, job_role, title,
			phone, mobile, email, start_date, end_date,
			status, is_primary, notes, created_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		ct.ContactID, ct.CustomerID, ct.FullName, ct.JobRole, ct.Title,
		ct.Phone, ct.Mobile, ct.Email, ct.StartDate, ct.EndDate,
		ct.Status, boolToInt(ct.IsPrimary), ct.Notes, now,
	)
	return err
}

// Update 담당자 수정
func (r *ContactRepo) Update(ct *model.Contact) error {
	_, err := r.db.Exec(`
		UPDATE contacts SET
			customer_id=?, full_name=?, job_role=?, title=?,
			phone=?, mobile=?, email=?, start_date=?, end_date=?,
			status=?, is_primary=?, notes=?
		WHERE contact_id=?`,
		ct.CustomerID, ct.FullName, ct.JobRole, ct.Title,
		ct.Phone, ct.Mobile, ct.Email, ct.StartDate, ct.EndDate,
		ct.Status, boolToInt(ct.IsPrimary), ct.Notes, ct.ContactID,
	)
	return err
}

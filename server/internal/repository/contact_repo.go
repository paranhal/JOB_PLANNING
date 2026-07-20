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

func normalizeContactRole(role string, isPrimary bool) string {
	switch role {
	case "primary", "secondary", "regular":
		return role
	}
	if isPrimary {
		return "primary"
	}
	return "regular"
}

func normalizeAffiliation(a string) string {
	switch a {
	case "institution", "integrator":
		return a
	default:
		return "institution"
	}
}

func applyContactFlags(ct *model.Contact, isPrimary int, role, affiliation string) {
	ct.IsPrimary = isPrimary == 1
	ct.ContactRole = normalizeContactRole(role, ct.IsPrimary)
	ct.IsPrimary = ct.ContactRole == "primary"
	ct.Affiliation = normalizeAffiliation(affiliation)
}

// List 담당자 목록 조회 (검색, 페이징)
func (r *ContactRepo) List(search string, page, pageSize int) ([]model.Contact, int, error) {
	offset := (page - 1) * pageSize

	baseQuery := `
		SELECT c.contact_id, c.customer_id, c.full_name,
		       COALESCE(c.affiliation,'institution'),
		       COALESCE(c.job_role,''), COALESCE(c.title,''), COALESCE(c.job_grade,''),
		       COALESCE(c.phone,''), COALESCE(c.mobile,''),
		       COALESCE(c.email,''), COALESCE(c.start_date,''),
		       COALESCE(c.end_date,''), COALESCE(c.status,'active'),
		       COALESCE(c.contact_role,''), c.is_primary, COALESCE(c.notes,''),
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

	baseQuery += ` ORDER BY cu.org_name, CASE COALESCE(c.contact_role,'') WHEN 'primary' THEN 0 WHEN 'secondary' THEN 1 ELSE 2 END, c.full_name LIMIT ? OFFSET ?`
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
		var role, affiliation string
		if err := rows.Scan(
			&item.ContactID, &item.CustomerID, &item.FullName,
			&affiliation,
			&item.JobRole, &item.Title, &item.JobGrade,
			&item.Phone, &item.Mobile,
			&item.Email, &item.StartDate,
			&item.EndDate, &item.Status,
			&role, &isPrimary, &item.Notes,
			&item.OrgName,
		); err != nil {
			return nil, 0, err
		}
		applyContactFlags(&item, isPrimary, role, affiliation)
		items = append(items, item)
	}
	return items, total, rows.Err()
}

// ListByCustomer 특정 고객의 담당자 목록
func (r *ContactRepo) ListByCustomer(customerID string) ([]model.Contact, error) {
	query := `
		SELECT contact_id, customer_id, full_name,
		       COALESCE(affiliation,'institution'),
		       COALESCE(job_role,''), COALESCE(title,''), COALESCE(job_grade,''),
		       COALESCE(phone,''), COALESCE(mobile,''),
		       COALESCE(email,''), COALESCE(start_date,''),
		       COALESCE(end_date,''), COALESCE(status,'active'),
		       COALESCE(contact_role,''), is_primary, COALESCE(notes,'')
		FROM contacts
		WHERE customer_id = ?
		ORDER BY CASE COALESCE(contact_role,'') WHEN 'primary' THEN 0 WHEN 'secondary' THEN 1 ELSE 2 END, full_name`

	rows, err := r.db.Query(query, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []model.Contact
	for rows.Next() {
		var item model.Contact
		var isPrimary int
		var role, affiliation string
		if err := rows.Scan(
			&item.ContactID, &item.CustomerID, &item.FullName,
			&affiliation,
			&item.JobRole, &item.Title, &item.JobGrade,
			&item.Phone, &item.Mobile,
			&item.Email, &item.StartDate,
			&item.EndDate, &item.Status,
			&role, &isPrimary, &item.Notes,
		); err != nil {
			return nil, err
		}
		applyContactFlags(&item, isPrimary, role, affiliation)
		items = append(items, item)
	}
	return items, rows.Err()
}

// GetByID 담당자 단건 조회
func (r *ContactRepo) GetByID(id string) (*model.Contact, error) {
	query := `
		SELECT c.contact_id, c.customer_id, c.full_name,
		       COALESCE(c.affiliation,'institution'),
		       COALESCE(c.job_role,''), COALESCE(c.title,''), COALESCE(c.job_grade,''),
		       COALESCE(c.phone,''), COALESCE(c.mobile,''),
		       COALESCE(c.email,''), COALESCE(c.start_date,''),
		       COALESCE(c.end_date,''), COALESCE(c.status,'active'),
		       COALESCE(c.contact_role,''), c.is_primary, COALESCE(c.notes,''),
		       cu.org_name
		FROM contacts c
		JOIN customers cu ON cu.customer_id = c.customer_id
		WHERE c.contact_id = ?`

	var ct model.Contact
	var isPrimary int
	var role, affiliation string
	err := r.db.QueryRow(query, id).Scan(
		&ct.ContactID, &ct.CustomerID, &ct.FullName,
		&affiliation,
		&ct.JobRole, &ct.Title, &ct.JobGrade,
		&ct.Phone, &ct.Mobile,
		&ct.Email, &ct.StartDate,
		&ct.EndDate, &ct.Status,
		&role, &isPrimary, &ct.Notes,
		&ct.OrgName,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	applyContactFlags(&ct, isPrimary, role, affiliation)
	return &ct, nil
}

// Create 담당자 등록
func (r *ContactRepo) Create(ct *model.Contact) error {
	ct.ContactID = fmt.Sprintf("CONT-%d", time.Now().UnixNano())
	ct.ContactRole = normalizeContactRole(ct.ContactRole, ct.IsPrimary)
	ct.IsPrimary = ct.ContactRole == "primary"
	ct.Affiliation = normalizeAffiliation(ct.Affiliation)
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := r.db.Exec(`
		INSERT INTO contacts (
			contact_id, customer_id, full_name, affiliation, job_role, title, job_grade,
			phone, mobile, email, start_date, end_date,
			status, contact_role, is_primary, notes, created_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		ct.ContactID, ct.CustomerID, ct.FullName, ct.Affiliation, ct.JobRole, ct.Title, ct.JobGrade,
		ct.Phone, ct.Mobile, ct.Email, ct.StartDate, ct.EndDate,
		ct.Status, ct.ContactRole, boolToInt(ct.IsPrimary), ct.Notes, now,
	)
	return err
}

// Update 담당자 수정
func (r *ContactRepo) Update(ct *model.Contact) error {
	ct.ContactRole = normalizeContactRole(ct.ContactRole, ct.IsPrimary)
	ct.IsPrimary = ct.ContactRole == "primary"
	ct.Affiliation = normalizeAffiliation(ct.Affiliation)
	_, err := r.db.Exec(`
		UPDATE contacts SET
			customer_id=?, full_name=?, affiliation=?, job_role=?, title=?, job_grade=?,
			phone=?, mobile=?, email=?, start_date=?, end_date=?,
			status=?, contact_role=?, is_primary=?, notes=?
		WHERE contact_id=?`,
		ct.CustomerID, ct.FullName, ct.Affiliation, ct.JobRole, ct.Title, ct.JobGrade,
		ct.Phone, ct.Mobile, ct.Email, ct.StartDate, ct.EndDate,
		ct.Status, ct.ContactRole, boolToInt(ct.IsPrimary), ct.Notes, ct.ContactID,
	)
	return err
}

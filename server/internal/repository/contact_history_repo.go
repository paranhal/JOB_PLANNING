package repository

import (
	"database/sql"
	"time"

	"customer-support/internal/model"
)

type ContactHistoryRepo struct{ db *sql.DB }

func NewContactHistoryRepo(db *sql.DB) *ContactHistoryRepo { return &ContactHistoryRepo{db: db} }

func (r *ContactHistoryRepo) ListByContact(contactID string) ([]model.ContactHistory, error) {
	rows, err := r.db.Query(
		`SELECT history_id, contact_id, customer_id,
		        COALESCE(start_date,''), COALESCE(end_date,''), COALESCE(department,''),
		        COALESCE(job_role,''), COALESCE(title,''), COALESCE(phone,''),
		        COALESCE(email,''), COALESCE(status,''), COALESCE(change_reason,'')
		 FROM contact_history WHERE contact_id=? ORDER BY start_date DESC`, contactID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []model.ContactHistory
	for rows.Next() {
		var h model.ContactHistory
		if err := rows.Scan(&h.HistoryID, &h.ContactID, &h.CustomerID,
			&h.StartDate, &h.EndDate, &h.Department,
			&h.JobRole, &h.Title, &h.Phone,
			&h.Email, &h.Status, &h.ChangeReason); err != nil {
			return nil, err
		}
		items = append(items, h)
	}
	return items, rows.Err()
}

func (r *ContactHistoryRepo) ListByCustomer(customerID string) ([]model.ContactHistory, error) {
	rows, err := r.db.Query(
		`SELECT ch.history_id, ch.contact_id, ch.customer_id,
		        COALESCE(ch.start_date,''), COALESCE(ch.end_date,''), COALESCE(ch.department,''),
		        COALESCE(ch.job_role,''), COALESCE(ch.title,''), COALESCE(ch.phone,''),
		        COALESCE(ch.email,''), COALESCE(ch.status,''), COALESCE(ch.change_reason,'')
		 FROM contact_history ch
		 WHERE ch.customer_id=? ORDER BY ch.start_date DESC`, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []model.ContactHistory
	for rows.Next() {
		var h model.ContactHistory
		if err := rows.Scan(&h.HistoryID, &h.ContactID, &h.CustomerID,
			&h.StartDate, &h.EndDate, &h.Department,
			&h.JobRole, &h.Title, &h.Phone,
			&h.Email, &h.Status, &h.ChangeReason); err != nil {
			return nil, err
		}
		items = append(items, h)
	}
	return items, rows.Err()
}

func (r *ContactHistoryRepo) Create(h *model.ContactHistory) error {
	h.HistoryID = newID("CH")
	_, err := r.db.Exec(`
		INSERT INTO contact_history
		(history_id,contact_id,customer_id,start_date,end_date,department,
		 job_role,title,phone,email,status,change_reason,created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		h.HistoryID, h.ContactID, h.CustomerID,
		h.StartDate, h.EndDate, h.Department,
		h.JobRole, h.Title, h.Phone, h.Email,
		h.Status, h.ChangeReason,
		time.Now().Format("2006-01-02 15:04:05"))
	return err
}

// SnapshotFromContact 현재 담당자 정보를 이력으로 저장
func (r *ContactHistoryRepo) SnapshotFromContact(ct *model.Contact, reason string) error {
	h := &model.ContactHistory{
		ContactID:    ct.ContactID,
		CustomerID:   ct.CustomerID,
		StartDate:    ct.StartDate,
		EndDate:      ct.EndDate,
		JobRole:      ct.JobRole,
		Title:        ct.Title,
		Phone:        ct.Phone,
		Email:        ct.Email,
		Status:       ct.Status,
		ChangeReason: reason,
	}
	return r.Create(h)
}

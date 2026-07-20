package repository

import (
	"database/sql"
	"time"

	"customer-support/internal/model"
)

type ContactHistoryRepo struct{ db *sql.DB }

func NewContactHistoryRepo(db *sql.DB) *ContactHistoryRepo { return &ContactHistoryRepo{db: db} }

func scanHistory(rows *sql.Rows) ([]model.ContactHistory, error) {
	var items []model.ContactHistory
	for rows.Next() {
		var h model.ContactHistory
		if err := rows.Scan(&h.HistoryID, &h.ContactID, &h.CustomerID,
			&h.StartDate, &h.EndDate, &h.Department,
			&h.JobRole, &h.Title, &h.Phone, &h.Mobile,
			&h.Email, &h.Status, &h.Affiliation, &h.ContactRole, &h.ChangeReason,
			&h.ContactName); err != nil {
			return nil, err
		}
		items = append(items, h)
	}
	return items, rows.Err()
}

const historySelect = `SELECT ch.history_id, ch.contact_id, ch.customer_id,
		        COALESCE(ch.start_date,''), COALESCE(ch.end_date,''), COALESCE(ch.department,''),
		        COALESCE(ch.job_role,''), COALESCE(ch.title,''), COALESCE(ch.phone,''),
		        COALESCE(ch.mobile,''), COALESCE(ch.email,''), COALESCE(ch.status,''),
		        COALESCE(ch.affiliation,''), COALESCE(ch.contact_role,''),
		        COALESCE(ch.change_reason,''),
		        COALESCE(c.full_name,'')
		 FROM contact_history ch
		 LEFT JOIN contacts c ON c.contact_id = ch.contact_id`

func (r *ContactHistoryRepo) ListByContact(contactID string) ([]model.ContactHistory, error) {
	rows, err := r.db.Query(
		historySelect+` WHERE ch.contact_id=? ORDER BY ch.start_date DESC, ch.created_at DESC`, contactID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanHistory(rows)
}

func (r *ContactHistoryRepo) ListByCustomer(customerID string) ([]model.ContactHistory, error) {
	rows, err := r.db.Query(
		historySelect+` WHERE ch.customer_id=? ORDER BY ch.start_date DESC, ch.created_at DESC`, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanHistory(rows)
}

func (r *ContactHistoryRepo) Create(h *model.ContactHistory) error {
	h.HistoryID = newID("CH")
	if h.Affiliation == "" {
		h.Affiliation = "institution"
	}
	_, err := r.db.Exec(`
		INSERT INTO contact_history
		(history_id,contact_id,customer_id,start_date,end_date,department,
		 job_role,title,phone,mobile,email,status,affiliation,contact_role,change_reason,created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		h.HistoryID, h.ContactID, h.CustomerID,
		h.StartDate, h.EndDate, h.Department,
		h.JobRole, h.Title, h.Phone, h.Mobile, h.Email,
		h.Status, h.Affiliation, h.ContactRole, h.ChangeReason,
		time.Now().Format("2006-01-02 15:04:05"))
	return err
}

// SnapshotFromContact 담당자 변경 이력을 저장 (전보/퇴직/업무조정)
func (r *ContactHistoryRepo) SnapshotFromContact(ct *model.Contact, reason string) error {
	role := ct.ContactRole
	if role == "" {
		if ct.IsPrimary {
			role = "primary"
		} else {
			role = "regular"
		}
	}
	aff := ct.Affiliation
	if aff == "" {
		aff = "institution"
	}
	h := &model.ContactHistory{
		ContactID:    ct.ContactID,
		CustomerID:   ct.CustomerID,
		StartDate:    ct.StartDate,
		EndDate:      ct.EndDate,
		JobRole:      ct.JobRole,
		Title:        ct.Title,
		Phone:        ct.Phone,
		Mobile:       ct.Mobile,
		Email:        ct.Email,
		Status:       ct.Status,
		Affiliation:  aff,
		ContactRole:  role,
		ChangeReason: reason,
	}
	return r.Create(h)
}

// IsMeaningfulChangeReason 담당자 변경 이력으로 취급할 사유인지
func IsMeaningfulChangeReason(reason string) bool {
	switch reason {
	case "transfer", "resign", "role_adjust", "전보", "퇴직", "업무조정":
		return true
	default:
		return false
	}
}

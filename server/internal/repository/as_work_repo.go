package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"customer-support/internal/model"
	"github.com/google/uuid"
)

type ASWorkRepo struct {
	db *sql.DB
}

func NewASWorkRepo(db *sql.DB) *ASWorkRepo {
	return &ASWorkRepo{db: db}
}

func (r *ASWorkRepo) Create(w *model.ASWorkItem) error {
	if w.WorkID == "" {
		w.WorkID = "W-" + uuid.New().String()[:8]
	}
	now := time.Now()
	if w.CreatedAt.IsZero() {
		w.CreatedAt = now
	}
	w.UpdatedAt = now
	if w.Status == "" {
		w.Status = "open"
	}
	var asNumber string
	if err := r.db.QueryRow(`SELECT as_number FROM as_receipts WHERE as_id=?`, w.ASID).Scan(&asNumber); err != nil {
		return err
	}
	num, err := NextWorkNumber(r.db, asNumber)
	if err != nil {
		return err
	}
	w.WorkNumber = num
	_, err = r.db.Exec(`INSERT INTO as_work_items (
		work_id, work_number, as_id, work_kind, scheduled_date, schedule_confirmed,
		confirm_target, confirm_contact, status, notes, created_at, updated_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		w.WorkID, w.WorkNumber, w.ASID, w.WorkKind, w.ScheduledDate, boolToInt(w.ScheduleConfirmed),
		w.ConfirmTarget, w.ConfirmContact, w.Status, w.Notes,
		w.CreatedAt.Format("2006-01-02 15:04:05"), w.UpdatedAt.Format("2006-01-02 15:04:05"),
	)
	return err
}

func (r *ASWorkRepo) ListByAS(asID string) ([]model.ASWorkItem, error) {
	rows, err := r.db.Query(`SELECT work_id, work_number, as_id, work_kind, COALESCE(scheduled_date,''),
		COALESCE(schedule_confirmed,0), COALESCE(confirm_target,''), COALESCE(confirm_contact,''),
		COALESCE(status,'open'), COALESCE(notes,''), COALESCE(created_at,''), COALESCE(updated_at,'')
		FROM as_work_items WHERE as_id=? ORDER BY scheduled_date, work_number`, asID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanWorkItems(rows)
}

// ListByASIDs 목록 들여쓰기용 일괄 조회
func (r *ASWorkRepo) ListByASIDs(asIDs []string) (map[string][]model.ASWorkItem, error) {
	out := map[string][]model.ASWorkItem{}
	if len(asIDs) == 0 {
		return out, nil
	}
	ph := make([]string, len(asIDs))
	args := make([]interface{}, len(asIDs))
	for i, id := range asIDs {
		ph[i] = "?"
		args[i] = id
	}
	q := fmt.Sprintf(`SELECT work_id, work_number, as_id, work_kind, COALESCE(scheduled_date,''),
		COALESCE(schedule_confirmed,0), COALESCE(confirm_target,''), COALESCE(confirm_contact,''),
		COALESCE(status,'open'), COALESCE(notes,''), COALESCE(created_at,''), COALESCE(updated_at,'')
		FROM as_work_items WHERE as_id IN (%s) ORDER BY as_id, scheduled_date, work_number`, strings.Join(ph, ","))
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanWorkItems(rows)
	if err != nil {
		return nil, err
	}
	for _, w := range items {
		out[w.ASID] = append(out[w.ASID], w)
	}
	return out, nil
}

func (r *ASWorkRepo) DeleteByAS(asID string) error {
	_, err := r.db.Exec(`DELETE FROM as_work_items WHERE as_id=?`, asID)
	return err
}

func scanWorkItems(rows *sql.Rows) ([]model.ASWorkItem, error) {
	var items []model.ASWorkItem
	for rows.Next() {
		var w model.ASWorkItem
		var conf int
		var created, updated string
		if err := rows.Scan(&w.WorkID, &w.WorkNumber, &w.ASID, &w.WorkKind, &w.ScheduledDate,
			&conf, &w.ConfirmTarget, &w.ConfirmContact, &w.Status, &w.Notes, &created, &updated); err != nil {
			return nil, err
		}
		w.ScheduleConfirmed = conf == 1
		w.CreatedAt = parseTime(created)
		w.UpdatedAt = parseTime(updated)
		items = append(items, w)
	}
	return items, rows.Err()
}

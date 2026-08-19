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

const asWorkSelect = `SELECT work_id, work_number, as_id, work_kind, COALESCE(scheduled_date,''),
		COALESCE(schedule_confirmed,0), COALESCE(confirm_target,''), COALESCE(confirm_contact,''),
		COALESCE(assigned_to,''), COALESCE(assigned_user_id,''),
		COALESCE(status,'open'), COALESCE(notes,''), COALESCE(created_at,''), COALESCE(updated_at,'')
		FROM as_work_items`

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
	// 담당자 미지정 시 원 접수 담당자를 초기값으로만 복사(이후 독립)
	if strings.TrimSpace(w.AssignedTo) == "" && strings.TrimSpace(w.AssignedUserID) == "" {
		var aTo, aUID string
		_ = r.db.QueryRow(`SELECT COALESCE(assigned_to,''), COALESCE(assigned_user_id,'') FROM as_receipts WHERE as_id=?`,
			w.ASID).Scan(&aTo, &aUID)
		w.AssignedTo, w.AssignedUserID = aTo, aUID
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
		confirm_target, confirm_contact, assigned_to, assigned_user_id,
		status, notes, created_at, updated_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		w.WorkID, w.WorkNumber, w.ASID, w.WorkKind, w.ScheduledDate, boolToInt(w.ScheduleConfirmed),
		w.ConfirmTarget, w.ConfirmContact, w.AssignedTo, w.AssignedUserID,
		w.Status, w.Notes,
		w.CreatedAt.Format("2006-01-02 15:04:05"), w.UpdatedAt.Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		return err
	}
	logCreate(r.db, "as_work_items", "work_id", w.WorkID, w.WorkNumber)
	return nil
}

func (r *ASWorkRepo) GetByID(workID string) (*model.ASWorkItem, error) {
	rows, err := r.db.Query(asWorkSelect+` WHERE work_id=?`, workID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanWorkItems(rows)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	w := items[0]
	return &w, nil
}

func (r *ASWorkRepo) ListByAS(asID string) ([]model.ASWorkItem, error) {
	rows, err := r.db.Query(asWorkSelect+` WHERE as_id=? ORDER BY scheduled_date, work_number`, asID)
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
	q := asWorkSelect + ` WHERE as_id IN (` + strings.Join(ph, ",") + `) ORDER BY as_id, scheduled_date, work_number`
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

// Update 하부업무만 수정한다. 원 접수(as_receipts)는 변경하지 않는다.
func (r *ASWorkRepo) Update(w *model.ASWorkItem) error {
	if w == nil || strings.TrimSpace(w.WorkID) == "" {
		return fmt.Errorf("work_id 필요")
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	return touchUpdate(r.db, "as_work_items", "work_id", w.WorkID, w.WorkNumber, func() error {
		_, err := r.db.Exec(`UPDATE as_work_items SET
		scheduled_date=?, schedule_confirmed=?, confirm_target=?, confirm_contact=?,
		assigned_to=?, assigned_user_id=?, status=?, notes=?, updated_at=?
		WHERE work_id=?`,
			w.ScheduledDate, boolToInt(w.ScheduleConfirmed), w.ConfirmTarget, w.ConfirmContact,
			w.AssignedTo, w.AssignedUserID, w.Status, w.Notes, now, w.WorkID)
		return err
	})
}

// SetScheduledDate 미계획 업무함에서 하부업무 예정일만 넣는다.
func (r *ASWorkRepo) SetScheduledDate(workID, date string) error {
	workID = strings.TrimSpace(workID)
	parsed, err := model.ParseAppDate(date)
	if err != nil {
		return err
	}
	date = parsed
	if workID == "" || date == "" {
		return fmt.Errorf("날짜가 필요합니다")
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := r.db.Exec(`UPDATE as_work_items SET scheduled_date=?, schedule_confirmed=1, updated_at=? WHERE work_id=?`,
		date, now, workID)
	return err
}

// MarkDone 하부업무만 완료. 원 접수 상태는 그대로 둔다.
func (r *ASWorkRepo) MarkDone(workID string) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := r.db.Exec(`UPDATE as_work_items SET status='done', updated_at=? WHERE work_id=?`, now, workID)
	return err
}

func (r *ASWorkRepo) DeleteByAS(asID string) error {
	_, err := r.db.Exec(`DELETE FROM as_work_items WHERE as_id=?`, asID)
	return err
}

// CloseOpenByAS 접수 완료·종료 시 남은 확인·재방문 하부업무를 완료 처리한다.
func (r *ASWorkRepo) CloseOpenByAS(asID string) error {
	if strings.TrimSpace(asID) == "" {
		return nil
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := r.db.Exec(`UPDATE as_work_items SET status='done', updated_at=? WHERE as_id=? AND status='open'`,
		now, asID)
	return err
}

// CloseOpenUnderClosedReceipts 이미 완료·종료·취소된 접수의 open 하부업무를 일괄 정리한다.
func (r *ASWorkRepo) CloseOpenUnderClosedReceipts() (int64, error) {
	now := time.Now().Format("2006-01-02 15:04:05")
	res, err := r.db.Exec(`
		UPDATE as_work_items
		SET status='done', updated_at=?
		WHERE status='open'
		  AND as_id IN (
			SELECT as_id FROM as_receipts
			WHERE status IN ('completed','closed','cancelled')
		  )`, now)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func scanWorkItems(rows *sql.Rows) ([]model.ASWorkItem, error) {
	var items []model.ASWorkItem
	for rows.Next() {
		var w model.ASWorkItem
		var conf int
		var created, updated string
		if err := rows.Scan(&w.WorkID, &w.WorkNumber, &w.ASID, &w.WorkKind, &w.ScheduledDate,
			&conf, &w.ConfirmTarget, &w.ConfirmContact, &w.AssignedTo, &w.AssignedUserID,
			&w.Status, &w.Notes, &created, &updated); err != nil {
			return nil, err
		}
		w.ScheduleConfirmed = conf == 1
		w.CreatedAt = parseTime(created)
		w.UpdatedAt = parseTime(updated)
		items = append(items, w)
	}
	return items, rows.Err()
}

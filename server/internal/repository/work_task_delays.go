package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"customer-support/internal/model"
)

const (
	DelayKindCarry  = "carry"
	DelayKindReason = "reason"
)

type WorkTaskDelay struct {
	DelayID         string
	TaskID          string
	OrgID           string
	Kind            string
	PrevWorkDate    string
	PrevDueDate     string
	NewWorkDate     string
	NewDueDate      string
	Reason          string
	OverdueDays     int
	CreatedByUserID string
	CreatedByName   string
	CreatedAt       string
}

func (r *WBRepo) InsertWorkTaskDelay(d WorkTaskDelay) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("저장소 없음")
	}
	return insertWorkTaskDelay(r.db, r, d)
}

func insertWorkTaskDelay(db *sql.DB, seq *WBRepo, d WorkTaskDelay) error {
	id := strings.TrimSpace(d.DelayID)
	if id == "" && seq != nil {
		var err error
		id, err = seq.nextID("work_delay", "WD")
		if err != nil {
			return err
		}
	}
	if id == "" {
		id = "WD-" + time.Now().Format("150405")
	}
	kind := strings.TrimSpace(d.Kind)
	if kind == "" {
		kind = DelayKindReason
	}
	_, err := db.Exec(`
		INSERT INTO work_task_delays (
			delay_id, task_id, org_id, kind, prev_work_date, prev_due_date,
			new_work_date, new_due_date, reason, overdue_days,
			created_by_user_id, created_by_name)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		id, d.TaskID, d.OrgID, kind, d.PrevWorkDate, d.PrevDueDate,
		d.NewWorkDate, d.NewDueDate, d.Reason, d.OverdueDays,
		d.CreatedByUserID, d.CreatedByName)
	return err
}

func (r *WBRepo) ListDelaysByTask(taskID string) ([]WorkTaskDelay, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	rows, err := r.db.Query(`
		SELECT delay_id, task_id, COALESCE(org_id,''), COALESCE(kind,''),
		       COALESCE(prev_work_date,''), COALESCE(prev_due_date,''),
		       COALESCE(new_work_date,''), COALESCE(new_due_date,''),
		       COALESCE(reason,''), COALESCE(overdue_days,0),
		       COALESCE(created_by_user_id,''), COALESCE(created_by_name,''),
		       COALESCE(created_at,'')
		FROM work_task_delays WHERE task_id=? ORDER BY created_at DESC, delay_id DESC`, taskID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []WorkTaskDelay
	for rows.Next() {
		var d WorkTaskDelay
		if err := rows.Scan(&d.DelayID, &d.TaskID, &d.OrgID, &d.Kind,
			&d.PrevWorkDate, &d.PrevDueDate, &d.NewWorkDate, &d.NewDueDate,
			&d.Reason, &d.OverdueDays, &d.CreatedByUserID, &d.CreatedByName, &d.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *WBRepo) LatestDelayReasons(taskIDs []string) (map[string]string, error) {
	return latestDelayReasons(r.db, taskIDs)
}

func latestDelayReasons(db *sql.DB, taskIDs []string) (map[string]string, error) {
	out := map[string]string{}
	if db == nil || len(taskIDs) == 0 {
		return out, nil
	}
	seen := map[string]bool{}
	var ids []string
	for _, id := range taskIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return out, nil
	}
	const chunk = 400
	for i := 0; i < len(ids); i += chunk {
		end := i + chunk
		if end > len(ids) {
			end = len(ids)
		}
		part := ids[i:end]
		ph := strings.TrimRight(strings.Repeat("?,", len(part)), ",")
		q := `SELECT d.task_id, d.reason FROM work_task_delays d
			INNER JOIN (
			  SELECT task_id, MAX(created_at) AS mx FROM work_task_delays
			  WHERE task_id IN (` + ph + `) GROUP BY task_id
			) m ON m.task_id=d.task_id AND m.mx=d.created_at`
		args := make([]interface{}, len(part))
		for j, id := range part {
			args[j] = id
		}
		rows, err := db.Query(q, args...)
		if err != nil {
			if strings.Contains(err.Error(), "no such table") {
				return out, nil
			}
			return nil, err
		}
		for rows.Next() {
			var id, reason string
			if err := rows.Scan(&id, &reason); err != nil {
				rows.Close()
				return nil, err
			}
			out[id] = strings.TrimSpace(reason)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return out, nil
}

func (r *WorkBoardRepo) LatestDelayReasons(taskIDs []string) (map[string]string, error) {
	if r == nil {
		return map[string]string{}, nil
	}
	return latestDelayReasons(r.db, taskIDs)
}

func (r *WorkBoardRepo) stampDelayReasons(items []model.WorkListItem) {
	var ids []string
	for _, it := range items {
		if strings.HasPrefix(strings.TrimSpace(it.RefID), "WT") || strings.Contains(it.Href, "/workboard/tasks/") {
			ids = append(ids, it.RefID)
		}
	}
	m, err := r.LatestDelayReasons(ids)
	if err != nil || len(m) == 0 {
		return
	}
	for i := range items {
		if s := m[items[i].RefID]; s != "" {
			items[i].DelayReasonFull = s
			items[i].DelayReason = model.TruncRunes(s, 40)
		}
	}
}

func (r *WBRepo) CarryTask(old *model.WorkTask, newWork, newDue, reason, byID, byName string, overdueDays int, kind string) error {
	if r == nil || old == nil || strings.TrimSpace(old.TaskID) == "" {
		return fmt.Errorf("업무가 없습니다")
	}
	newWork = strings.TrimSpace(newWork)
	newDue = strings.TrimSpace(newDue)
	if kind == "" {
		kind = DelayKindCarry
	}
	id, err := r.nextID("work_delay", "WD")
	if err != nil {
		return err
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		if !ok {
			_ = tx.Rollback()
		}
	}()
	origW := strings.TrimSpace(old.OrigWorkDate)
	origD := strings.TrimSpace(old.OrigDueDate)
	if origW == "" {
		origW = strings.TrimSpace(old.WorkDate)
	}
	if origD == "" {
		origD = strings.TrimSpace(old.DueDate)
	}
	if _, err := tx.Exec(`
		UPDATE work_tasks SET work_date=?, due_date=?,
			orig_work_date=CASE WHEN TRIM(COALESCE(orig_work_date,''))='' THEN ? ELSE orig_work_date END,
			orig_due_date=CASE WHEN TRIM(COALESCE(orig_due_date,''))='' THEN ? ELSE orig_due_date END,
			carry_count=COALESCE(carry_count,0)+?,
			updated_at=CURRENT_TIMESTAMP
		WHERE task_id=?`,
		newWork, newDue, origW, origD, boolToInt(kind == DelayKindCarry), old.TaskID); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO work_task_delays (
			delay_id, task_id, org_id, kind, prev_work_date, prev_due_date,
			new_work_date, new_due_date, reason, overdue_days,
			created_by_user_id, created_by_name)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		id, old.TaskID, old.OrgID, kind, old.WorkDate, old.DueDate,
		newWork, newDue, reason, overdueDays, byID, byName); err != nil {
		return err
	}
	if kind == DelayKindCarry && strings.TrimSpace(old.WorkDate) != "" && newWork != "" && old.WorkDate != newWork {
		delta, err := daysBetweenYMD(old.WorkDate, newWork)
		if err == nil && delta != 0 {
			sign := "+"
			n := delta
			if n < 0 {
				sign = "-"
				n = -n
			}
			mod := fmt.Sprintf("%s%d days", sign, n)
			if _, err := tx.Exec(`UPDATE work_task_segments SET work_date=date(work_date, ?), updated_at=CURRENT_TIMESTAMP
				WHERE task_id=? AND TRIM(work_date)!=''`, mod, old.TaskID); err != nil && !strings.Contains(err.Error(), "no such table") {
				return err
			}
			if _, err := tx.Exec(`UPDATE work_task_segments SET work_date=?, updated_at=CURRENT_TIMESTAMP
				WHERE task_id=? AND TRIM(work_date)=''`, newWork, old.TaskID); err != nil && !strings.Contains(err.Error(), "no such table") {
				return err
			}
		}
	}
	srcDate := newWork
	if srcDate == "" {
		srcDate = newDue
	}
	switch strings.TrimSpace(old.SourceType) {
	case model.WBSourceMaintenance:
		if srcDate != "" {
			var completed int
			_ = tx.QueryRow(`SELECT COALESCE(completed,0) FROM maintenance_visits WHERE visit_id=?`, old.SourceID).Scan(&completed)
			if completed != 0 {
				return fmt.Errorf("완료된 정기점검 방문은 이월할 수 없습니다")
			}
			if _, err := tx.Exec(`UPDATE maintenance_visits SET visit_date=? WHERE visit_id=? AND COALESCE(completed,0)=0`, srcDate, old.SourceID); err != nil {
				return err
			}
		}
	case model.WBSourceAS:
		if srcDate != "" {
			now := time.Now().Format("2006-01-02 15:04:05")
			if _, err := tx.Exec(`UPDATE as_receipts SET visit_scheduled_date=?, schedule_confirmed=1, updated_at=? WHERE as_id=?`, srcDate, now, old.SourceID); err != nil {
				return err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	ok = true
	return nil
}

func (r *WBRepo) RecordDateMoveDelay(old *model.WorkTask, newWork, newDue, reason, byID, byName string, overdueDays int) error {
	if r == nil || old == nil {
		return nil
	}
	d := WorkTaskDelay{
		TaskID: old.TaskID, OrgID: old.OrgID, Kind: DelayKindCarry,
		PrevWorkDate: old.WorkDate, PrevDueDate: old.DueDate,
		NewWorkDate: newWork, NewDueDate: newDue, Reason: reason,
		OverdueDays: overdueDays, CreatedByUserID: byID, CreatedByName: byName,
	}
	if err := r.InsertWorkTaskDelay(d); err != nil {
		return err
	}
	_, err := r.db.Exec(`
		UPDATE work_tasks SET
			orig_work_date=CASE WHEN TRIM(COALESCE(orig_work_date,''))='' THEN ? ELSE orig_work_date END,
			orig_due_date=CASE WHEN TRIM(COALESCE(orig_due_date,''))='' THEN ? ELSE orig_due_date END,
			carry_count=COALESCE(carry_count,0)+1,
			updated_at=CURRENT_TIMESTAMP
		WHERE task_id=?`, strings.TrimSpace(old.WorkDate), strings.TrimSpace(old.DueDate), old.TaskID)
	return err
}

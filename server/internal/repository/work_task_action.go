package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"customer-support/internal/model"
)

func (r *WBRepo) LoadTaskActionFields(t *model.WorkTask) error {
	if r == nil || r.db == nil || t == nil || strings.TrimSpace(t.TaskID) == "" {
		return nil
	}
	var recurrence int
	err := r.db.QueryRow(`
		SELECT COALESCE(work_place,''), COALESCE(action_taken,''), COALESCE(result_code,''),
		       COALESCE(complete_datetime,''), COALESCE(customer_confirmer,''),
		       COALESCE(hold_next_action,''), COALESCE(transfer_to,''),
		       COALESCE(target_category_id,''), COALESCE(work_reason,''),
		       COALESCE(action_start_at,''), COALESCE(action_duration_min,0),
		       COALESCE(action_remark,''), COALESCE(recurrence_possible,0),
		       COALESCE(customer_confirmed_at,'')
		FROM work_tasks WHERE task_id=? AND org_id=?`, t.TaskID, t.OrgID).
		Scan(&t.WorkPlace, &t.ActionTaken, &t.ResultCode, &t.CompleteDatetime,
			&t.CustomerConfirmer, &t.HoldNextAction, &t.TransferTo,
			&t.TargetCategoryID, &t.WorkReason,
			&t.ActionStartAt, &t.ActionDurationMin, &t.ActionRemark, &recurrence, &t.CustomerConfirmedAt)
	if err == nil {
		t.RecurrencePossible = recurrence != 0
	}
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil && strings.Contains(err.Error(), "no such column") {
		return nil
	}
	return err
}

func (r *WBRepo) SetTaskWorkPlace(taskID, place string) error {
	taskID = strings.TrimSpace(taskID)
	if r == nil || r.db == nil || taskID == "" {
		return nil
	}
	place = model.NormalizeWorkPlace(place)
	_, err := r.db.Exec(`UPDATE work_tasks SET work_place=?, updated_at=datetime('now','localtime') WHERE task_id=?`, place, taskID)
	if err != nil && strings.Contains(err.Error(), "no such column") {
		return nil
	}
	return err
}

func (r *WBRepo) SaveTaskActionFields(t *model.WorkTask) error {
	if r == nil || t == nil || strings.TrimSpace(t.TaskID) == "" {
		return fmt.Errorf("업무가 없습니다")
	}
	orgID, err := RequireInsertOrg(t.OrgID)
	if err != nil {
		return err
	}
	recurrence := 0
	if t.RecurrencePossible {
		recurrence = 1
	}
	_, err = r.db.Exec(`
		UPDATE work_tasks SET
			work_place=?, action_taken=?, result_code=?, complete_datetime=?,
			customer_confirmer=?, hold_reason=?, hold_next_action=?, transfer_to=?,
			cancel_reason=?, status=?, complete_date=?, complete_note=?,
			progress=?, work_subtype=?,
			target_category_id=?, work_reason=?,
			assignee=?, assignee_user_id=?, title=?, description=?,
			action_start_at=?, action_duration_min=?, action_remark=?,
			recurrence_possible=?, customer_confirmed_at=?,
			updated_at=datetime('now','localtime')
		WHERE task_id=? AND org_id=?`,
		t.WorkPlace, t.ActionTaken, t.ResultCode, t.CompleteDatetime,
		t.CustomerConfirmer, t.HoldReason, t.HoldNextAction, t.TransferTo,
		t.CancelReason, t.Status, t.CompleteDate, t.CompleteNote,
		t.Progress, t.WorkSubtype,
		t.TargetCategoryID, t.WorkReason,
		t.Assignee, t.AssigneeUserID, t.Title, t.Description,
		t.ActionStartAt, t.ActionDurationMin, t.ActionRemark,
		recurrence, t.CustomerConfirmedAt, t.TaskID, orgID)
	return err
}

func (r *WBRepo) InsertWorkTaskProcess(p model.WorkTaskProcess) (string, error) {
	orgID, err := RequireInsertOrg(p.OrgID)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(p.TaskID) == "" {
		return "", fmt.Errorf("업무가 없습니다")
	}
	if p.ProcessID == "" {
		p.ProcessID = "wtp_" + uuid.NewString()
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	if p.ProcessAt == "" {
		p.ProcessAt = now
	}
	if p.CreatedAt == "" {
		p.CreatedAt = now
	}
	_, err = r.db.Exec(`
		INSERT INTO work_task_processes
		(process_id, task_id, org_id, process_at, worker, worker_user_id, work_place,
		 result_code, work_content, note, next_date, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.ProcessID, p.TaskID, orgID, p.ProcessAt, p.Worker, p.WorkerUserID, p.WorkPlace,
		p.ResultCode, p.WorkContent, p.Note, p.NextDate, p.CreatedAt)
	return p.ProcessID, err
}

func (r *WBRepo) ListWorkTaskProcesses(orgID, taskID string) ([]model.WorkTaskProcess, error) {
	orgID, err := RequireInsertOrg(orgID)
	if err != nil {
		return nil, err
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, nil
	}
	rows, err := r.db.Query(`
		SELECT process_id, task_id, COALESCE(org_id,''), COALESCE(process_at,''),
		       COALESCE(worker,''), COALESCE(worker_user_id,''), COALESCE(work_place,''),
		       COALESCE(result_code,''), COALESCE(work_content,''), COALESCE(note,''),
		       COALESCE(next_date,''), COALESCE(created_at,'')
		FROM work_task_processes
		WHERE org_id=? AND task_id=?
		ORDER BY process_at DESC, created_at DESC`, orgID, taskID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.WorkTaskProcess
	for rows.Next() {
		var p model.WorkTaskProcess
		if err := rows.Scan(&p.ProcessID, &p.TaskID, &p.OrgID, &p.ProcessAt, &p.Worker, &p.WorkerUserID,
			&p.WorkPlace, &p.ResultCode, &p.WorkContent, &p.Note, &p.NextDate, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ListTasksWithProcessBetweenForOrg 조치 이력(process_at)이 기간 안인 업무. 완료일만 있는 건 제외(95-94 Q-4).
func (r *WBRepo) ListTasksWithProcessBetweenForOrg(from, to, orgID string) ([]model.WorkTask, map[string][]string, error) {
	from, to = strings.TrimSpace(from), strings.TrimSpace(to)
	dates := map[string][]string{}
	if from == "" || to == "" {
		return nil, dates, nil
	}
	rows, err := r.db.Query(`
		SELECT p.task_id, substr(p.process_at,1,10)
		FROM work_task_processes p
		INNER JOIN work_tasks t ON t.task_id=p.task_id
		WHERE substr(p.process_at,1,10) >= ? AND substr(p.process_at,1,10) <= ?
		  AND COALESCE(t.recurrence_role,'') != 'parent'`, from, to)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, dates, nil
		}
		return nil, dates, err
	}
	defer rows.Close()
	seenID := map[string]bool{}
	var ids []string
	for rows.Next() {
		var id, day string
		if err := rows.Scan(&id, &day); err != nil {
			return nil, dates, err
		}
		id, day = strings.TrimSpace(id), strings.TrimSpace(day)
		if id == "" || len(day) < 10 {
			continue
		}
		day = day[:10]
		dup := false
		for _, d := range dates[id] {
			if d == day {
				dup = true
				break
			}
		}
		if !dup {
			dates[id] = append(dates[id], day)
		}
		if !seenID[id] {
			seenID[id] = true
			ids = append(ids, id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, dates, err
	}
	if len(ids) == 0 {
		return nil, dates, nil
	}
	ph := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]interface{}, 0, len(ids)+2)
	for _, id := range ids {
		args = append(args, id)
	}
	frag, extra := workTaskVisibleOrgSQL("t", orgID)
	args = append(args, extra...)
	list, err := r.listStoredTasks(`
		WHERE t.task_id IN (`+ph+`)`+frag+workTaskOrder, args...)
	if err != nil {
		return nil, dates, err
	}
	return list, dates, nil
}

// CountCompleteWithoutProcessBetweenForOrg 완료일은 기간 안인데 조치 이력이 없는 건수.
func (r *WBRepo) CountCompleteWithoutProcessBetweenForOrg(from, to, orgID string) (int, error) {
	from, to = strings.TrimSpace(from), strings.TrimSpace(to)
	if from == "" || to == "" {
		return 0, nil
	}
	frag, extra := workTaskVisibleOrgSQL("t", orgID)
	args := append([]interface{}{from, to}, extra...)
	var n int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM work_tasks t
		WHERE TRIM(COALESCE(t.complete_date,'')) != ''
		  AND t.complete_date >= ? AND t.complete_date <= ?
		  AND COALESCE(t.recurrence_role,'') != 'parent'
		  AND NOT EXISTS (SELECT 1 FROM work_task_processes p WHERE p.task_id=t.task_id)`+frag, args...).Scan(&n)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return 0, nil
		}
		return 0, err
	}
	return n, nil
}

func (r *WBRepo) ListWorkTaskParts(orgID, taskID string) ([]model.ASPartUsed, error) {
	if _, err := RequireInsertOrg(orgID); err != nil {
		return nil, err
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, nil
	}
	rows, err := r.db.Query(`
		SELECT p.part_id, p.task_id, COALESCE(p.process_id,''), p.item_id, p.part_name,
		       COALESCE(p.part_code,''), p.qty, p.sort_order,
		       COALESCE(p.created_at,''), COALESCE(p.updated_at,'')
		FROM work_task_parts_used p
		INNER JOIN work_tasks t ON t.task_id=p.task_id AND t.org_id=?
		WHERE p.task_id=?
		ORDER BY p.sort_order ASC, p.part_id ASC`, orgID, taskID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []model.ASPartUsed
	for rows.Next() {
		var p model.ASPartUsed
		var itemID sql.NullString
		var task string
		if err := rows.Scan(&p.PartID, &task, &p.ProcessID, &itemID, &p.PartName,
			&p.PartCode, &p.Qty, &p.SortOrder, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		p.ASID = task
		if itemID.Valid {
			p.ItemID = itemID.String
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *WBRepo) ReplaceWorkTaskParts(orgID, taskID, processID string, parts []model.ASPartUsed) error {
	orgID, err := RequireInsertOrg(orgID)
	if err != nil {
		return err
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("업무가 없습니다")
	}
	var n int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM work_tasks WHERE task_id=? AND org_id=?`, taskID, orgID).Scan(&n); err != nil || n == 0 {
		return fmt.Errorf("업무가 없습니다")
	}
	if _, err := r.db.Exec(`DELETE FROM work_task_parts_used WHERE task_id=?`, taskID); err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil
		}
		return err
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	for i, p := range parts {
		name := strings.TrimSpace(p.PartName)
		if name == "" {
			continue
		}
		qty := p.Qty
		if qty < 1 {
			qty = 1
		}
		id := strings.TrimSpace(p.PartID)
		if id == "" {
			id = "wtpu_" + uuid.NewString()
		}
		var item any
		if strings.TrimSpace(p.ItemID) != "" {
			item = p.ItemID
		}
		if _, err := r.db.Exec(`
			INSERT INTO work_task_parts_used
			(part_id, task_id, process_id, item_id, part_name, part_code, qty, sort_order, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?)`,
			id, taskID, processID, item, name, strings.TrimSpace(p.PartCode), qty, i, now, now); err != nil {
			return err
		}
	}
	return nil
}

func (r *WBRepo) CountPartialFollowups(orgID, parentID, workDate string) (int, error) {
	orgID, err := RequireInsertOrg(orgID)
	if err != nil {
		return 0, err
	}
	var n int
	err = r.db.QueryRow(`
		SELECT COUNT(*) FROM work_tasks
		WHERE org_id=? AND parent_task_id=? AND COALESCE(source_role,'')=?
		  AND COALESCE(work_date,'')=?`,
		orgID, strings.TrimSpace(parentID), model.WorkSourcePartial, strings.TrimSpace(workDate)).Scan(&n)
	return n, err
}

func FirstLine(s string, max int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	if max > 0 && utf8.RuneCountInString(s) > max {
		runes := []rune(s)
		s = string(runes[:max])
	}
	return s
}

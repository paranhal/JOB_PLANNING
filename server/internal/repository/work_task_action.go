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
	err := r.db.QueryRow(`
		SELECT COALESCE(work_place,''), COALESCE(action_taken,''), COALESCE(result_code,''),
		       COALESCE(complete_datetime,''), COALESCE(customer_confirmer,''),
		       COALESCE(hold_next_action,''), COALESCE(transfer_to,''),
		       COALESCE(target_category_id,''), COALESCE(work_reason,'')
		FROM work_tasks WHERE task_id=? AND org_id=?`, t.TaskID, t.OrgID).
		Scan(&t.WorkPlace, &t.ActionTaken, &t.ResultCode, &t.CompleteDatetime,
			&t.CustomerConfirmer, &t.HoldNextAction, &t.TransferTo,
			&t.TargetCategoryID, &t.WorkReason)
	if err == sql.ErrNoRows {
		return nil
	}
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
	_, err = r.db.Exec(`
		UPDATE work_tasks SET
			work_place=?, action_taken=?, result_code=?, complete_datetime=?,
			customer_confirmer=?, hold_reason=?, hold_next_action=?, transfer_to=?,
			cancel_reason=?, status=?, complete_date=?, complete_note=?,
			work_date=?, progress=?, work_subtype=?,
			target_category_id=?, work_reason=?,
			assignee=?, assignee_user_id=?, updated_at=datetime('now','localtime')
		WHERE task_id=? AND org_id=?`,
		t.WorkPlace, t.ActionTaken, t.ResultCode, t.CompleteDatetime,
		t.CustomerConfirmer, t.HoldReason, t.HoldNextAction, t.TransferTo,
		t.CancelReason, t.Status, t.CompleteDate, t.CompleteNote,
		t.WorkDate, t.Progress, t.WorkSubtype,
		t.TargetCategoryID, t.WorkReason,
		t.Assignee, t.AssigneeUserID, t.TaskID, orgID)
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

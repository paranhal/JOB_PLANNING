package repository

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"customer-support/internal/model"
)

var (
	ErrASMoveReason    = errors.New("이관 사유가 필요합니다")
	ErrASAlreadyMoved  = errors.New("이미 행정/지원으로 옮긴 접수입니다")
	ErrASMoveCancelled = errors.New("취소된 접수는 옮길 수 없습니다")
)

// MoveToAdminWork AS 접수를 지우지 않고 행정/지원 업무로 옮긴다. §48.4
func (r *ASRepo) MoveToAdminWork(asID, reason string) (*model.WorkTask, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, ErrASMoveReason
	}
	as, err := r.GetByID(asID)
	if err != nil {
		return nil, err
	}
	if as == nil {
		return nil, fmt.Errorf("접수를 찾을 수 없습니다")
	}
	if strings.TrimSpace(as.MovedTaskID) != "" || as.Status == model.StatusAdminWork {
		return nil, ErrASAlreadyMoved
	}
	if as.Status == "cancelled" {
		return nil, ErrASMoveCancelled
	}

	title := strings.TrimSpace(as.Symptom)
	if title == "" {
		title = as.ASNumber
	}
	var desc strings.Builder
	if s := strings.TrimSpace(as.Symptom); s != "" {
		desc.WriteString(s)
	}
	if a := strings.TrimSpace(as.ActionTaken); a != "" {
		if desc.Len() > 0 {
			desc.WriteString("\n\n")
		}
		desc.WriteString(a)
	}
	now := time.Now()
	today := now.Format("2006-01-02")
	receiptDate := today
	if !as.ReceiptDatetime.IsZero() {
		receiptDate = as.ReceiptDatetime.Format("2006-01-02")
	}
	t := &model.WorkTask{
		WorkType:       model.WBWorkAdmin,
		Title:          title,
		Description:    desc.String(),
		DueDate:        today,
		WorkDate:       today,
		DurationMin:    30,
		Status:         model.WBTaskWaiting,
		Priority:       model.WBPriorityNormal,
		Assignee:       as.AssignedTo,
		AssigneeUserID: as.AssignedUserID,
		CustomerID:     as.CustomerID,
		ReceiptDate:    receiptDate,
	}

	before := rowJSON(r.db, "as_receipts", "as_id", as.ASID)
	wb := NewWBRepo(r.db)
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if err := wb.assignNewTaskID(t); err != nil {
		return nil, err
	}
	normalizeWorkTask(t)
	t.Assignee, t.AssigneeUserID = bindStaff(r.db, t.Assignee, t.AssigneeUserID)
	stampNewWorkTaskDates(t)
	_, err = tx.Exec(`
		INSERT INTO work_tasks (task_id, work_type, project_id, title, description, due_date,
			work_date, start_time, end_time, duration_min, status, priority, assignee, assignee_user_id, assignee_source, tags, progress,
			source_type, source_id, source_role, parent_task_id, customer_id, customer_name,
			hold_reason, review_date, cancel_reason, wait_party_kind, wait_party, wait_request,
			reply_due_date, next_check_date, complete_note, receipt_date, complete_date,
			recurrence_role, occurrence_seq, occurrence_status, not_done_reason)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.TaskID, t.WorkType, nullStr(t.ProjectID), t.Title, t.Description, t.DueDate,
		t.WorkDate, t.StartTime, t.EndTime, t.DurationMin, t.Status, t.Priority, t.Assignee, t.AssigneeUserID, t.AssigneeSource, t.Tags, t.Progress,
		t.SourceType, t.SourceID, t.SourceRole, nullStr(t.ParentTaskID), nullStr(t.CustomerID), nullIfEmpty(t.CustomerName),
		nullIfEmpty(t.HoldReason), nullIfEmpty(t.ReviewDate), nullIfEmpty(t.CancelReason),
		nullIfEmpty(t.WaitPartyKind), nullIfEmpty(t.WaitParty), nullIfEmpty(t.WaitRequest),
		nullIfEmpty(t.ReplyDueDate), nullIfEmpty(t.NextCheckDate), nullIfEmpty(t.CompleteNote),
		nullIfEmpty(t.ReceiptDate), nullIfEmpty(t.CompleteDate),
		nullIfEmpty(t.RecurrenceRole), t.OccurrenceSeq, nullIfEmpty(t.OccurrenceStatus), nullIfEmpty(t.NotDoneReason))
	if err != nil {
		return nil, err
	}
	if aid := strings.TrimSpace(as.AssetID); aid != "" {
		if _, err := tx.Exec(`INSERT INTO work_task_assets (task_id, asset_id) VALUES (?,?)`, t.TaskID, aid); err != nil {
			return nil, err
		}
	}
	nowStr := now.Format("2006-01-02 15:04:05")
	if _, err := tx.Exec(`
		UPDATE as_receipts
		SET status=?, moved_task_id=?, complete_datetime=COALESCE(NULLIF(TRIM(complete_datetime),''), ?),
		    updated_at=?
		WHERE as_id=?`,
		model.StatusAdminWork, t.TaskID, nowStr, nowStr, as.ASID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	logCreate(r.db, "work_tasks", "task_id", t.TaskID, t.Title)
	logUpdateWithReason(r.db, "as_receipts", "as_id", as.ASID, as.ASNumber, before, reason)
	return t, nil
}

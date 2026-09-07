package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"customer-support/internal/model"
)

const workActionSelect = `
		SELECT action_id, task_id, title, COALESCE(status,'todo'), COALESCE(required,1),
		       COALESCE(scheduled_date,''), COALESCE(due_date,''), COALESCE(assignee,''),
		       COALESCE(wait_party_kind,''), COALESCE(wait_party,''), COALESCE(wait_request,''),
		       COALESCE(reply_due_date,''), COALESCE(next_check_date,''), COALESCE(confirmed,0),
		       COALESCE(sort_order,0), COALESCE(created_at,''), COALESCE(updated_at,'')
		FROM work_actions`

func scanWorkActions(rows *sql.Rows) ([]model.WorkAction, error) {
	var items []model.WorkAction
	for rows.Next() {
		var a model.WorkAction
		var req, conf int
		if err := rows.Scan(&a.ActionID, &a.TaskID, &a.Title, &a.Status, &req,
			&a.ScheduledDate, &a.DueDate, &a.Assignee,
			&a.WaitPartyKind, &a.WaitParty, &a.WaitRequest,
			&a.ReplyDueDate, &a.NextCheckDate, &conf, &a.SortOrder,
			&a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		a.Required = req != 0
		a.Confirmed = conf != 0
		items = append(items, a)
	}
	return items, rows.Err()
}

func (r *WBRepo) ListActions(taskID string) ([]model.WorkAction, error) {
	rows, err := r.db.Query(workActionSelect+`
		WHERE task_id=? ORDER BY sort_order ASC, created_at ASC, action_id ASC`, taskID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	return scanWorkActions(rows)
}

func (r *WBRepo) GetAction(id string) (*model.WorkAction, error) {
	rows, err := r.db.Query(workActionSelect+` WHERE action_id=?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanWorkActions(rows)
	if err != nil || len(items) == 0 {
		return nil, err
	}
	a := items[0]
	return &a, nil
}

func (r *WBRepo) CreateAction(a *model.WorkAction) error {
	if a == nil || strings.TrimSpace(a.TaskID) == "" || strings.TrimSpace(a.Title) == "" {
		return fmt.Errorf("다음 행동 제목이 필요합니다")
	}
	if a.Status == "" {
		a.Status = model.WBActionTodo
	}
	id, err := r.nextID("work_action", "WA")
	if err != nil {
		return err
	}
	a.ActionID = id
	req := 0
	if a.Required {
		req = 1
	}
	conf := 0
	if a.Confirmed {
		conf = 1
	}
	_, err = r.db.Exec(`
		INSERT INTO work_actions (action_id, task_id, title, status, required, scheduled_date, due_date, assignee,
			wait_party_kind, wait_party, wait_request, reply_due_date, next_check_date, confirmed, sort_order)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		a.ActionID, a.TaskID, a.Title, a.Status, req, nullIfEmpty(a.ScheduledDate), nullIfEmpty(a.DueDate),
		nullIfEmpty(a.Assignee), nullIfEmpty(a.WaitPartyKind), nullIfEmpty(a.WaitParty), nullIfEmpty(a.WaitRequest),
		nullIfEmpty(a.ReplyDueDate), nullIfEmpty(a.NextCheckDate), conf, a.SortOrder)
	if err != nil {
		return err
	}
	logCreate(r.db, "work_actions", "action_id", a.ActionID, a.Title)
	return nil
}

func (r *WBRepo) UpdateAction(a *model.WorkAction) error {
	if a == nil || a.ActionID == "" {
		return fmt.Errorf("action_id 필요")
	}
	req := 0
	if a.Required {
		req = 1
	}
	conf := 0
	if a.Confirmed {
		conf = 1
	}
	return touchUpdate(r.db, "work_actions", "action_id", a.ActionID, a.Title, func() error {
		_, err := r.db.Exec(`
			UPDATE work_actions SET title=?, status=?, required=?, scheduled_date=?, due_date=?, assignee=?,
				wait_party_kind=?, wait_party=?, wait_request=?, reply_due_date=?, next_check_date=?,
				confirmed=?, sort_order=?, updated_at=CURRENT_TIMESTAMP
			WHERE action_id=?`,
			a.Title, a.Status, req, nullIfEmpty(a.ScheduledDate), nullIfEmpty(a.DueDate), nullIfEmpty(a.Assignee),
			nullIfEmpty(a.WaitPartyKind), nullIfEmpty(a.WaitParty), nullIfEmpty(a.WaitRequest),
			nullIfEmpty(a.ReplyDueDate), nullIfEmpty(a.NextCheckDate), conf, a.SortOrder, a.ActionID)
		return err
	})
}

func (r *WBRepo) CountOpenRequiredActions(taskID string) (int, error) {
	var n int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM work_actions
		WHERE task_id=? AND COALESCE(required,1)=1
		  AND COALESCE(status,'todo') NOT IN ('complete','cancelled')`, taskID).Scan(&n)
	if err != nil && strings.Contains(err.Error(), "no such table") {
		return 0, nil
	}
	return n, err
}

func (r *WBRepo) CountUnconfirmedWaiting(taskID string) (int, error) {
	var n int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM work_actions
		WHERE task_id=? AND status='waiting' AND COALESCE(confirmed,0)=0
		  AND COALESCE(status,'') != 'cancelled'`, taskID).Scan(&n)
	if err != nil && strings.Contains(err.Error(), "no such table") {
		return 0, nil
	}
	return n, err
}

func (r *WBRepo) ListActivities(taskID string) ([]model.WorkActivity, error) {
	rows, err := r.db.Query(`
		SELECT activity_id, task_id, COALESCE(action_id,''), COALESCE(activity_type,'other'),
		       content, COALESCE(actor,''), COALESCE(spent_minutes,0), COALESCE(created_at,'')
		FROM work_activities WHERE task_id=? ORDER BY created_at DESC, activity_id DESC`, taskID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var items []model.WorkActivity
	for rows.Next() {
		var a model.WorkActivity
		if err := rows.Scan(&a.ActivityID, &a.TaskID, &a.ActionID, &a.ActivityType,
			&a.Content, &a.Actor, &a.SpentMinutes, &a.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, a)
	}
	return items, rows.Err()
}

func (r *WBRepo) CreateActivity(a *model.WorkActivity) error {
	if a == nil || strings.TrimSpace(a.TaskID) == "" || strings.TrimSpace(a.Content) == "" {
		return fmt.Errorf("조치 내용이 필요합니다")
	}
	if a.ActivityType == "" {
		a.ActivityType = model.WBActivityOther
	}
	if a.SpentMinutes <= 0 {
		a.SpentMinutes = model.WBActivityDefaultSpent
	}
	id, err := r.nextID("work_activity", "WY")
	if err != nil {
		return err
	}
	a.ActivityID = id
	_, err = r.db.Exec(`
		INSERT INTO work_activities (activity_id, task_id, action_id, activity_type, content, actor, spent_minutes)
		VALUES (?,?,?,?,?,?,?)`,
		a.ActivityID, a.TaskID, nullIfEmpty(a.ActionID), a.ActivityType, a.Content, nullIfEmpty(a.Actor), a.SpentMinutes)
	if err != nil {
		return err
	}
	logCreate(r.db, "work_activities", "activity_id", a.ActivityID, a.Content)
	return nil
}

// AdminCompleteBlockers 미완료 필수 다음 행동·미확인 회신 대기 건수.
func (r *WBRepo) AdminCompleteBlockers(taskID string) (openRequired, unconfirmedWait int, err error) {
	openRequired, err = r.CountOpenRequiredActions(taskID)
	if err != nil {
		return 0, 0, err
	}
	unconfirmedWait, err = r.CountUnconfirmedWaiting(taskID)
	return openRequired, unconfirmedWait, err
}

func (r *WBRepo) CountActionProgress(taskID string) (doneRequired, required int, err error) {
	err = r.db.QueryRow(`
		SELECT
			COALESCE(SUM(CASE WHEN COALESCE(required,1)=1 AND COALESCE(status,'todo')='complete' THEN 1 ELSE 0 END),0),
			COALESCE(SUM(CASE WHEN COALESCE(required,1)=1 AND COALESCE(status,'todo') != 'cancelled' THEN 1 ELSE 0 END),0)
		FROM work_actions WHERE task_id=?`, taskID).Scan(&doneRequired, &required)
	if err != nil && strings.Contains(err.Error(), "no such table") {
		return 0, 0, nil
	}
	return doneRequired, required, err
}

// SyncAdminActionProgress 진행률 = 완료 필수 다음 행동 ÷ 필수 다음 행동. 필수 행동이 없으면 건드리지 않는다.
func (r *WBRepo) SyncAdminActionProgress(taskID string) error {
	done, req, err := r.CountActionProgress(taskID)
	if err != nil || req <= 0 {
		return err
	}
	pct := int(model.PlanningRatePct(done, req) + 0.5)
	if pct > 100 {
		pct = 100
	}
	_, err = r.db.Exec(`UPDATE work_tasks SET progress=?, updated_at=CURRENT_TIMESTAMP WHERE task_id=?`, pct, taskID)
	return err
}

// ResumeInProgressIfReady 필수 다음 행동·미확인 대기가 없으면 회신 대기를 진행중으로 되돌린다(§13.5).
func (r *WBRepo) ResumeInProgressIfReady(taskID string) error {
	open, wait, err := r.AdminCompleteBlockers(taskID)
	if err != nil || open > 0 || wait > 0 {
		return err
	}
	_, err = r.db.Exec(`
		UPDATE work_tasks SET status=?, updated_at=CURRENT_TIMESTAMP
		WHERE task_id=? AND status=?`,
		model.WBTaskInProgress, taskID, model.WBTaskWaitingFor)
	return err
}

func (r *WBRepo) CountWaitingActionsByTasks(ids []string) (map[string]int, error) {
	out := map[string]int{}
	if len(ids) == 0 {
		return out, nil
	}
	ph := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		ph[i] = "?"
		args[i] = id
	}
	rows, err := r.db.Query(`
		SELECT task_id, COUNT(*) FROM work_actions
		WHERE task_id IN (`+strings.Join(ph, ",")+`)
		  AND status='waiting' AND COALESCE(confirmed,0)=0
		GROUP BY task_id`, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return out, nil
		}
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// ListWaitingActionsDueCheck 다음 확인일이 그날인 회신 대기 행동(일정표에는 올리지 않고 할 일로만 표시).
func (r *WBRepo) ListWaitingActionsDueCheck(today string) ([]model.WorkAction, error) {
	rows, err := r.db.Query(workActionSelect+`
		WHERE status='waiting' AND COALESCE(confirmed,0)=0
		  AND TRIM(COALESCE(next_check_date,'')) = ?
		ORDER BY action_id`, today)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	return scanWorkActions(rows)
}

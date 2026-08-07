package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"customer-support/internal/model"
)

type WBRepo struct{ db *sql.DB }

func NewWBRepo(db *sql.DB) *WBRepo { return &WBRepo{db: db} }

const workTaskSelect = `
		SELECT t.task_id, COALESCE(t.work_type,'admin'), COALESCE(t.project_id,''), t.title, COALESCE(t.description,''),
		       COALESCE(t.due_date,''), COALESCE(t.work_date,''), COALESCE(t.start_time,''), COALESCE(t.end_time,''),
		       COALESCE(t.duration_min,30),
		       COALESCE(t.status,'waiting'), COALESCE(t.priority,'normal'),
		       COALESCE(t.assignee,''), COALESCE(t.tags,''), COALESCE(t.progress,0),
		       COALESCE(t.source_type,''), COALESCE(t.source_id,''), COALESCE(t.parent_task_id,''),
		       t.created_at, t.updated_at,
		       COALESCE(p.name,''), COALESCE(p.color,'')
		FROM work_tasks t
		LEFT JOIN work_projects p ON p.project_id = t.project_id`

func (r *WBRepo) Summary() (model.WBSummary, error) {
	return r.SummaryOnDate("")
}

// SummaryOnDate date(YYYY-MM-DD)가 있으면 그날 업무만 집계. 빈 값이면 전체(상위만).
func (r *WBRepo) SummaryOnDate(date string) (model.WBSummary, error) {
	var s model.WBSummary
	q := `SELECT status, priority FROM work_tasks WHERE 1=1`
	args := []interface{}{}
	if date != "" {
		q += ` AND COALESCE(NULLIF(TRIM(work_date),''), NULLIF(TRIM(due_date),'')) = ?`
		args = append(args, date)
	} else {
		q += ` AND COALESCE(parent_task_id,'')=''`
	}
	rows, err := r.db.Query(q, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return s, nil
		}
		return s, err
	}
	defer rows.Close()
	for rows.Next() {
		var st, pri string
		if err := rows.Scan(&st, &pri); err != nil {
			return s, err
		}
		s.TotalTasks++
		switch model.WBKanbanBucket(st) {
		case model.WBTaskWaiting:
			s.Waiting++
		case model.WBTaskInProgress:
			s.InProgress++
		case model.WBTaskReview:
			s.Review++
		case model.WBTaskComplete:
			s.Complete++
		}
		if pri == model.WBPriorityUrgent {
			s.Urgent++
		}
	}
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM work_projects WHERE status != ?`, model.WBProjectArchived).Scan(&s.ProjectCount)
	return s, rows.Err()
}

func (r *WBRepo) ListProjects(activeOnly bool) ([]model.WorkProject, error) {
	q := `SELECT p.project_id, p.name, COALESCE(p.short_name,''), COALESCE(p.plan_year,0),
		COALESCE(p.is_paid,1), COALESCE(p.sort_order,0),
		COALESCE(p.ordering_party_id,''), COALESCE(p.ordering_party,''), COALESCE(p.customer_id,''),
		COALESCE(p.contract_type,''), COALESCE(p.billing_type,''),
		COALESCE(p.start_date,''), COALESCE(p.end_date,''),
		COALESCE(p.notes,''), COALESCE(p.contact_id,''),
		COALESCE(p.color,'#3B82F6'), COALESCE(p.status,'active'),
		p.created_at, p.updated_at,
		COALESCE(cu.org_name,''), COALESCE(ct.full_name,''), COALESCE(op.org_name,'')
		FROM work_projects p
		LEFT JOIN customers cu ON cu.customer_id = p.customer_id
		LEFT JOIN contacts ct ON ct.contact_id = p.contact_id
		LEFT JOIN customers op ON op.customer_id = p.ordering_party_id`
	if activeOnly {
		q += ` WHERE p.status = 'active'`
	}
	q += ` ORDER BY COALESCE(p.sort_order,0), p.name`
	rows, err := r.db.Query(q)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	return scanProjects(rows)
}

func (r *WBRepo) ListTasks() ([]model.WorkTask, error) {
	rows, err := r.db.Query(workTaskSelect + `
		ORDER BY
		  CASE t.status WHEN 'waiting' THEN 1 WHEN 'in_progress' THEN 2 WHEN 'review' THEN 3 ELSE 4 END,
		  CASE t.priority WHEN 'urgent' THEN 1 WHEN 'high' THEN 2 WHEN 'normal' THEN 3 ELSE 4 END,
		  t.due_date, t.title`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	return scanWorkTasks(rows)
}

func (r *WBRepo) GetTask(id string) (*model.WorkTask, error) {
	rows, err := r.db.Query(workTaskSelect+` WHERE t.task_id=?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanWorkTasks(rows)
	if err != nil || len(items) == 0 {
		return nil, err
	}
	t := items[0]
	return &t, nil
}

// ListChildren 상위 업무의 일자별 하위업무
func (r *WBRepo) ListChildren(parentID string) ([]model.WorkTask, error) {
	rows, err := r.db.Query(workTaskSelect+`
		WHERE t.parent_task_id=?
		ORDER BY COALESCE(NULLIF(t.work_date,''), t.due_date), t.start_time, t.title`, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanWorkTasks(rows)
}

// ListTasksBetween 수행일이 기간 안에 있고 시작시간이 정해진 업무 (업무 등록 시간표용)
func (r *WBRepo) ListTasksBetween(from, to string) ([]model.WorkTask, error) {
	all, err := r.ListTasks()
	if err != nil {
		return nil, err
	}
	var items []model.WorkTask
	for _, t := range all {
		if t.WorkDate == "" || t.StartTime == "" {
			continue
		}
		if t.WorkDate >= from && t.WorkDate <= to {
			items = append(items, t)
		}
	}
	return items, nil
}

func (r *WBRepo) ListTasksByDate(date string) ([]model.WorkTask, error) {
	return r.ListTasksBetween(date, date)
}

// ListTasksOnDate 배정일(없으면 예정일)이 해당일인 업무 — 일일 업무 현황용(시간 유무 무관).
func (r *WBRepo) ListTasksOnDate(date string) ([]model.WorkTask, error) {
	rows, err := r.db.Query(workTaskSelect+`
		WHERE COALESCE(NULLIF(TRIM(t.work_date),''), NULLIF(TRIM(t.due_date),'')) = ?
		ORDER BY
		  CASE t.status
		    WHEN 'waiting' THEN 1 WHEN 'in_progress' THEN 2
		    WHEN 'hold' THEN 3 WHEN 'transfer' THEN 3 WHEN 'review' THEN 3
		    ELSE 4 END,
		  CASE t.priority WHEN 'urgent' THEN 1 WHEN 'high' THEN 2 WHEN 'normal' THEN 3 ELSE 4 END,
		  COALESCE(NULLIF(t.start_time,''),'99:99'), t.title`, date)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	return scanWorkTasks(rows)
}

// ASStatusesByIDs as_id → status
func (r *WBRepo) ASStatusesByIDs(ids []string) (map[string]string, error) {
	out := map[string]string{}
	if len(ids) == 0 {
		return out, nil
	}
	ph := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		ph[i] = "?"
		args[i] = id
	}
	q := `SELECT as_id, status FROM as_receipts WHERE as_id IN (` + strings.Join(ph, ",") + `)`
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, st string
		if err := rows.Scan(&id, &st); err != nil {
			return out, err
		}
		out[id] = st
	}
	return out, rows.Err()
}

// ListUnplacedAdminTasks 시간표에 아직 올리지 않은 행정/지원 업무(상위·하위 포함)
func (r *WBRepo) ListUnplacedAdminTasks() ([]model.WorkTask, error) {
	all, err := r.ListTasks()
	if err != nil {
		return nil, err
	}
	var items []model.WorkTask
	for _, t := range all {
		if t.SourceType != "" || t.Status == model.WBTaskComplete {
			continue
		}
		if t.WorkType != model.WBWorkAdmin && t.WorkType != model.WBWorkSupport {
			continue
		}
		if t.WorkDate == "" || t.StartTime == "" {
			items = append(items, t)
		}
	}
	return items, nil
}

func (r *WBRepo) GetTaskBySource(sourceType, sourceID string) (*model.WorkTask, error) {
	all, err := r.ListTasks()
	if err != nil {
		return nil, err
	}
	for _, t := range all {
		if t.SourceType == sourceType && t.SourceID == sourceID {
			task := t
			return &task, nil
		}
	}
	return nil, nil
}

// NextFreeSlot 해당 날짜에서 이미 배치된 업무 뒤를 이은 다음 시작시각(전체).
func (r *WBRepo) NextFreeSlot(date string, durationMin int) (start, end string, err error) {
	return r.nextFreeSlotFiltered(date, durationMin, "", false, "")
}

// NextFreeSlotForAssignee 같은 담당자 배치만 피해 다음 빈 시각을 찾는다(일·주 동일).
func (r *WBRepo) NextFreeSlotForAssignee(date, assignee string, durationMin int, excludeTaskID string) (start, end string, err error) {
	return r.nextFreeSlotFiltered(date, durationMin, strings.TrimSpace(assignee), true, excludeTaskID)
}

func (r *WBRepo) nextFreeSlotFiltered(date string, durationMin int, assignee string, byAssignee bool, excludeTaskID string) (start, end string, err error) {
	durationMin = model.NormalizeDurationMin(durationMin)
	placed, err := r.ListTasksByDate(date)
	if err != nil {
		return "", "", err
	}
	startMin := workdayStartMin
	for _, t := range placed {
		if excludeTaskID != "" && t.TaskID == excludeTaskID {
			continue
		}
		if strings.TrimSpace(t.StartTime) == "" {
			continue
		}
		if byAssignee && strings.TrimSpace(t.Assignee) != assignee {
			continue
		}
		em := model.ParseHHMMMinutes(t.EndTime)
		if em < 0 {
			em = model.ParseHHMMMinutes(t.StartTime) + model.NormalizeDurationMin(t.DurationMin)
		}
		if em > startMin {
			startMin = em
		}
	}
	if rem := startMin % 15; rem != 0 {
		startMin += 15 - rem
	}
	if startMin+durationMin > workdayEndMin {
		if byAssignee {
			return "", "", nil // 같은 담당자 일정 포화 — 겹침 배치 안 함
		}
		startMin = workdayStartMin
	}
	endMin := startMin + durationMin
	if endMin > workdayEndMin {
		endMin = workdayEndMin
	}
	return model.FormatHHMMMinutes(startMin), model.FormatHHMMMinutes(endMin), nil
}

// AssigneeTimeOverlaps 같은 담당자·같은 날 시간이 겹치면 true.
func (r *WBRepo) AssigneeTimeOverlaps(date, assignee, start, end, excludeTaskID string) (bool, error) {
	assignee = strings.TrimSpace(assignee)
	sm := model.ParseHHMMMinutes(start)
	em := model.ParseHHMMMinutes(end)
	if sm < 0 || em <= sm {
		return false, nil
	}
	placed, err := r.ListTasksByDate(date)
	if err != nil {
		return false, err
	}
	for _, t := range placed {
		if excludeTaskID != "" && t.TaskID == excludeTaskID {
			continue
		}
		if strings.TrimSpace(t.Assignee) != assignee {
			continue
		}
		if strings.TrimSpace(t.StartTime) == "" {
			continue
		}
		ts := model.ParseHHMMMinutes(t.StartTime)
		te := model.ParseHHMMMinutes(t.EndTime)
		if te < 0 {
			te = ts + model.NormalizeDurationMin(t.DurationMin)
		}
		if ts < 0 || te <= ts {
			continue
		}
		if sm < te && ts < em {
			return true, nil
		}
	}
	return false, nil
}

const (
	workdayStartMin = 7 * 60
	workdayEndMin   = 20 * 60
)

// PlaceTask 업무를 시간표의 특정 날짜·시각에 배치한다.
// 예정일(due_date)이 비어 있으면 배정일과 같게 채운다.
func (r *WBRepo) PlaceTask(taskID, workDate, startTime, endTime string) error {
	dur := model.DurationFromTimes(startTime, endTime)
	_, err := r.db.Exec(`
		UPDATE work_tasks SET work_date=?, start_time=?, end_time=?, duration_min=?,
		due_date=CASE WHEN TRIM(COALESCE(due_date,''))='' THEN ? ELSE due_date END,
		updated_at=CURRENT_TIMESTAMP
		WHERE task_id=?`, workDate, startTime, endTime, dur, workDate, taskID)
	return err
}

// SetTaskDueDate 예정일만 갱신한다.
func (r *WBRepo) SetTaskDueDate(taskID, dueDate string) error {
	_, err := r.db.Exec(`UPDATE work_tasks SET due_date=?, updated_at=CURRENT_TIMESTAMP WHERE task_id=?`, dueDate, taskID)
	return err
}

// UnplaceTask 시간표에서 내린다. 출처가 있는 카드(AS·점검)는 행 자체를 지운다.
func (r *WBRepo) UnplaceTask(taskID string) error {
	var sourceType string
	err := r.db.QueryRow(`SELECT COALESCE(source_type,'') FROM work_tasks WHERE task_id=?`, taskID).Scan(&sourceType)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if sourceType != "" {
		_, err = r.db.Exec(`DELETE FROM work_tasks WHERE task_id=?`, taskID)
		return err
	}
	// 행정/지원: 예정일·배정일·시간도 모두 비워 대기열로 되돌린다.
	_, err = r.db.Exec(`
		UPDATE work_tasks SET due_date='', work_date='', start_time='', end_time='', updated_at=CURRENT_TIMESTAMP
		WHERE task_id=?`, taskID)
	return err
}

func daysUntil(dateStr string, today time.Time) int {
	dateStr = strings.TrimSpace(dateStr)
	if dateStr == "" {
		return 0
	}
	d, err := time.ParseInLocation("2006-01-02", dateStr, today.Location())
	if err != nil {
		return 0
	}
	return int(d.Sub(today).Hours() / 24)
}

func (r *WBRepo) CreateProject(p *model.WorkProject) error {
	id, err := r.nextID("work_project", "WP")
	if err != nil {
		return err
	}
	p.ProjectID = id
	if p.Status == "" {
		p.Status = model.WBProjectActive
	}
	if p.Color == "" {
		p.Color = "#3B82F6"
	}
	if p.PlanYear == 0 && len(p.StartDate) >= 4 {
		fmt.Sscanf(p.StartDate[:4], "%d", &p.PlanYear)
	}
	paid := 1
	if !p.IsPaid {
		paid = 0
	}
	_, err = r.db.Exec(`
		INSERT INTO work_projects (project_id, name, short_name, plan_year, is_paid, sort_order,
			ordering_party_id, ordering_party, customer_id,
			contract_type, billing_type, start_date, end_date, notes, contact_id, color, status)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.ProjectID, p.Name, p.ShortName, p.PlanYear, paid, p.SortOrder,
		nullStr(p.OrderingPartyID), p.OrderingParty, nullStr(p.CustomerID),
		p.ContractType, p.BillingType, p.StartDate, p.EndDate, p.Notes, nullStr(p.ContactID), p.Color, p.Status)
	return err
}

func (r *WBRepo) CreateTask(t *model.WorkTask) error {
	id, err := r.nextID("work_task", "WT")
	if err != nil {
		return err
	}
	t.TaskID = id
	normalizeWorkTask(t)
	_, err = r.db.Exec(`
		INSERT INTO work_tasks (task_id, work_type, project_id, title, description, due_date,
			work_date, start_time, end_time, duration_min, status, priority, assignee, tags, progress,
			source_type, source_id, parent_task_id)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.TaskID, t.WorkType, nullStr(t.ProjectID), t.Title, t.Description, t.DueDate,
		t.WorkDate, t.StartTime, t.EndTime, t.DurationMin, t.Status, t.Priority, t.Assignee, t.Tags, t.Progress,
		t.SourceType, t.SourceID, nullStr(t.ParentTaskID))
	return err
}

// UpdateTask 업무 내용·배정일·소요시간 등을 수정한다. source·parent 는 유지.
func (r *WBRepo) UpdateTask(t *model.WorkTask) error {
	if t == nil || t.TaskID == "" {
		return fmt.Errorf("task_id 필요")
	}
	normalizeWorkTask(t)
	_, err := r.db.Exec(`
		UPDATE work_tasks SET work_type=?, project_id=?, title=?, description=?, due_date=?,
			work_date=?, start_time=?, end_time=?, duration_min=?, status=?, priority=?,
			assignee=?, tags=?, progress=?, updated_at=CURRENT_TIMESTAMP
		WHERE task_id=?`,
		t.WorkType, nullStr(t.ProjectID), t.Title, t.Description, t.DueDate,
		t.WorkDate, t.StartTime, t.EndTime, t.DurationMin, t.Status, t.Priority,
		t.Assignee, t.Tags, t.Progress, t.TaskID)
	return err
}

func normalizeWorkTask(t *model.WorkTask) {
	if t.Status == "" {
		t.Status = model.WBTaskWaiting
	}
	if t.Priority == "" {
		t.Priority = model.WBPriorityNormal
	}
	if t.Status == model.WBTaskComplete && t.Progress == 0 {
		t.Progress = 100
	}
	t.DurationMin = model.NormalizeDurationMin(t.DurationMin)
	switch t.WorkType {
	case model.WBWorkSupport:
		// project 유지
	case model.WBWorkAS, model.WBWorkMaintenance:
		t.ProjectID = ""
	default:
		if t.WorkType != model.WBWorkAdmin {
			t.WorkType = model.WBWorkAdmin
		}
		t.ProjectID = ""
	}
	if t.StartTime != "" && t.EndTime != "" {
		t.DurationMin = model.DurationFromTimes(t.StartTime, t.EndTime)
	} else if t.StartTime != "" && t.EndTime == "" {
		t.EndTime = model.FormatHHMMMinutes(model.ParseHHMMMinutes(t.StartTime) + t.DurationMin)
	}
}

// CreateSubtasks 상위 업무에 일자별 하위업무를 만든다. dates는 YYYY-MM-DD 목록.
func (r *WBRepo) CreateSubtasks(parent *model.WorkTask, dates []string) (int, error) {
	if parent == nil || parent.TaskID == "" {
		return 0, fmt.Errorf("상위 업무가 없습니다")
	}
	n := 0
	seen := map[string]bool{}
	for _, d := range dates {
		d = strings.TrimSpace(d)
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		child := &model.WorkTask{
			WorkType:     parent.WorkType,
			ProjectID:    parent.ProjectID,
			Title:        parent.Title,
			Description:  parent.Description,
			DueDate:      d,
			WorkDate:     "", // 시간표 배치 전
			DurationMin:  parent.DurationMin,
			Status:       model.WBTaskWaiting,
			Priority:     parent.Priority,
			Assignee:     parent.Assignee,
			Tags:         parent.Tags,
			ParentTaskID: parent.TaskID,
		}
		if err := r.CreateTask(child); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// ExpandDailyDates from~to(포함) 매일 날짜 목록. 최대 60일.
func ExpandDailyDates(from, to string) ([]string, error) {
	f, err := time.ParseInLocation("2006-01-02", from, time.Local)
	if err != nil {
		return nil, fmt.Errorf("시작일을 확인하세요")
	}
	t, err := time.ParseInLocation("2006-01-02", to, time.Local)
	if err != nil {
		return nil, fmt.Errorf("종료일을 확인하세요")
	}
	if t.Before(f) {
		return nil, fmt.Errorf("종료일이 시작일보다 앞입니다")
	}
	var out []string
	for d := f; !d.After(t); d = d.AddDate(0, 0, 1) {
		out = append(out, d.Format("2006-01-02"))
		if len(out) > 60 {
			return nil, fmt.Errorf("하위업무는 한 번에 60일까지 만들 수 있습니다")
		}
	}
	return out, nil
}

func (r *WBRepo) nextID(seqKey, prefix string) (string, error) {
	n, err := NextSeq(r.db, seqKey)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%03d", prefix, n), nil
}

func scanWorkTasks(rows *sql.Rows) ([]model.WorkTask, error) {
	var items []model.WorkTask
	today := time.Now().Truncate(24 * time.Hour)
	for rows.Next() {
		var t model.WorkTask
		var created, updated string
		if err := rows.Scan(
			&t.TaskID, &t.WorkType, &t.ProjectID, &t.Title, &t.Description,
			&t.DueDate, &t.WorkDate, &t.StartTime, &t.EndTime, &t.DurationMin,
			&t.Status, &t.Priority, &t.Assignee, &t.Tags, &t.Progress,
			&t.SourceType, &t.SourceID, &t.ParentTaskID,
			&created, &updated,
			&t.ProjectName, &t.ProjectColor,
		); err != nil {
			return nil, err
		}
		t.CreatedAt = parseTime(created)
		t.UpdatedAt = parseTime(updated)
		t.DaysLeft = daysUntil(t.DueDate, today)
		if t.DurationMin <= 0 {
			t.DurationMin = 30
		}
		items = append(items, t)
	}
	return items, rows.Err()
}

func scanProjects(rows *sql.Rows) ([]model.WorkProject, error) {
	var items []model.WorkProject
	for rows.Next() {
		var p model.WorkProject
		var created, updated string
		var paid int
		if err := rows.Scan(
			&p.ProjectID, &p.Name, &p.ShortName, &p.PlanYear, &paid, &p.SortOrder,
			&p.OrderingPartyID, &p.OrderingParty, &p.CustomerID,
			&p.ContractType, &p.BillingType,
			&p.StartDate, &p.EndDate,
			&p.Notes, &p.ContactID,
			&p.Color, &p.Status,
			&created, &updated,
			&p.CustomerName, &p.ContactName, &p.OrderingPartyName,
		); err != nil {
			return nil, err
		}
		p.IsPaid = paid != 0
		p.CreatedAt = parseTime(created)
		p.UpdatedAt = parseTime(updated)
		items = append(items, p)
	}
	return items, rows.Err()
}

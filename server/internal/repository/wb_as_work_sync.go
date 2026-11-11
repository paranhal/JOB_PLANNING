package repository

import (
	"database/sql"
	"log"
	"strings"

	"customer-support/internal/model"
)

// ASWorkBackfillReport 하위업무 → 일정 업무 보정 결과.
// dry-run 은 Missing 만 세고 Created 는 0 이다.
type ASWorkBackfillReport struct {
	Missing int
	Created int
	Already int
	Skipped int
	Reasons []string
}

type asWorkReceipt struct {
	ASID        string
	OrgName     string
	OrgID       string
	CustomerID  string
	ProjectID   string
	WorkGroupID string
}

// SyncASWorkDailyTask 재방문·확인·지원 하위업무를 work_tasks(source_type=as_work)에 맞춘다.
// 날짜가 있으면 일정표에 두고, 없으면 미배치로 남긴다. 담당자가 비어도 행은 지우지 않는다.
// 접수 본체(source_type=as) 정리와 별개라 ReconcileASDailyTasks 가 이 행을 고아로 지우지 않는다.
func (r *WBRepo) SyncASWorkDailyTask(w *model.ASWorkItem) error {
	if r == nil || r.db == nil || w == nil || strings.TrimSpace(w.WorkID) == "" {
		return nil
	}
	ctx, err := r.loadASWorkReceipt(w.ASID)
	if err != nil {
		return err
	}
	if ctx == nil {
		return r.DeleteTasksBySource(model.WBSourceASWork, w.WorkID)
	}
	existing, err := r.GetTaskBySource(model.WBSourceASWork, w.WorkID)
	if err != nil {
		return err
	}
	want := asWorkTaskFrom(w, ctx, existing)
	if existing == nil {
		return r.CreateTask(want)
	}
	oldDate := strings.TrimSpace(existing.WorkDate)
	want.TaskID = existing.TaskID
	if asWorkTaskUnchanged(existing, want) {
		return nil
	}
	if err := r.UpdateTask(want); err != nil {
		return err
	}
	newDate := strings.TrimSpace(want.WorkDate)
	if oldDate != "" && newDate != "" && oldDate != newDate {
		_, _ = r.db.Exec(`UPDATE work_task_segments SET work_date=? WHERE task_id=? AND work_date=?`,
			newDate, existing.TaskID, oldDate)
	}
	return nil
}

func (r *WBRepo) loadASWorkReceipt(asID string) (*asWorkReceipt, error) {
	asID = strings.TrimSpace(asID)
	if asID == "" {
		return nil, nil
	}
	var ctx asWorkReceipt
	err := r.db.QueryRow(`
		SELECT ar.as_id, COALESCE(c.org_name,''), COALESCE(ar.org_id,''),
		       COALESCE(ar.customer_id,''), COALESCE(ar.project_id,''), COALESCE(ar.work_group_id,'')
		  FROM as_receipts ar
		  JOIN customers c ON c.customer_id = ar.customer_id
		 WHERE ar.as_id=?`, asID).Scan(
		&ctx.ASID, &ctx.OrgName, &ctx.OrgID, &ctx.CustomerID, &ctx.ProjectID, &ctx.WorkGroupID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ctx, nil
}

func asWorkTaskFrom(w *model.ASWorkItem, ctx *asWorkReceipt, existing *model.WorkTask) *model.WorkTask {
	day := strings.TrimSpace(w.ScheduledDate)
	if model.RequireAppDateYear(day) != nil {
		day = ""
	}
	status := model.WBTaskWaiting
	progress := 0
	if strings.TrimSpace(w.Status) == "done" {
		status = model.WBTaskComplete
		progress = 100
	}
	t := &model.WorkTask{
		WorkType:       model.WBWorkAS,
		Title:          model.FormatASFollowupTitle(model.WorkKindLabel(w.WorkKind), ctx.OrgName, w.WorkNumber),
		Description:    strings.TrimSpace(w.Notes),
		DueDate:        day,
		WorkDate:       day,
		DurationMin:    30,
		Status:         status,
		Priority:       model.WBPriorityNormal,
		Assignee:       strings.TrimSpace(w.AssignedTo),
		AssigneeUserID: strings.TrimSpace(w.AssignedUserID),
		AssigneeSource: model.WBAssigneeSourceAS,
		SourceType:     model.WBSourceASWork,
		SourceID:       w.WorkID,
		CustomerID:     ctx.CustomerID,
		CustomerName:   strings.TrimSpace(ctx.OrgName),
		OrgID:          strings.TrimSpace(ctx.OrgID),
		WorkGroupID:    strings.TrimSpace(ctx.WorkGroupID),
		ProjectID:      strings.TrimSpace(ctx.ProjectID),
		Progress:       progress,
	}
	if existing == nil {
		return t
	}
	if day != "" {
		t.StartTime = existing.StartTime
		t.EndTime = existing.EndTime
		if existing.DurationMin > 0 {
			t.DurationMin = existing.DurationMin
		}
	}
	if status != model.WBTaskComplete && existing.Progress > 0 && existing.Status != model.WBTaskComplete {
		t.Progress = existing.Progress
	}
	return t
}

func asWorkTaskUnchanged(a, b *model.WorkTask) bool {
	if a == nil || b == nil {
		return false
	}
	return a.Title == b.Title &&
		a.Description == b.Description &&
		a.CustomerID == b.CustomerID &&
		a.CustomerName == b.CustomerName &&
		a.ProjectID == b.ProjectID &&
		a.DueDate == b.DueDate &&
		a.WorkDate == b.WorkDate &&
		a.StartTime == b.StartTime &&
		a.EndTime == b.EndTime &&
		a.Assignee == b.Assignee &&
		a.AssigneeUserID == b.AssigneeUserID &&
		a.AssigneeSource == b.AssigneeSource &&
		a.Status == b.Status &&
		a.Progress == b.Progress &&
		a.OrgID == b.OrgID &&
		a.WorkGroupID == b.WorkGroupID
}

// ListUndatedASWorkTasksForOrg 날짜가 없는 하위업무 일정. 담당자가 없어도 미배치에 둔다.
func (r *WBRepo) ListUndatedASWorkTasksForOrg(orgID string) ([]model.WorkTask, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	where := `
		WHERE t.source_type = ?
		  AND TRIM(COALESCE(t.work_date,'')) = ''
		  AND TRIM(COALESCE(t.due_date,'')) = ''
		  AND COALESCE(t.status,'') NOT IN ('complete','cancelled')`
	args := []interface{}{model.WBSourceASWork}
	frag, extra := workTaskVisibleOrgSQL("t", orgID)
	rows, err := r.db.Query(workTaskSelect+where+frag+`
		ORDER BY t.title, t.task_id`, append(args, extra...)...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	return scanWorkTasks(rows)
}

// ASWorkLinks 일정에 올라온 하위업무의 접수 번호와 미확정 여부.
func (r *WBRepo) ASWorkLinks(workIDs []string) (asID map[string]string, unconfirmed map[string]bool) {
	asID = map[string]string{}
	unconfirmed = map[string]bool{}
	ids := uniqueNonEmpty(workIDs)
	if r == nil || r.db == nil || len(ids) == 0 {
		return asID, unconfirmed
	}
	for i := 0; i < len(ids); i += inClauseChunk {
		end := i + inClauseChunk
		if end > len(ids) {
			end = len(ids)
		}
		part := ids[i:end]
		q := `SELECT work_id, as_id, COALESCE(schedule_confirmed,0) FROM as_work_items WHERE work_id IN (` + placeholders(len(part)) + `)`
		rows, err := r.db.Query(q, iface(part)...)
		if err != nil {
			return asID, unconfirmed
		}
		for rows.Next() {
			var id, as string
			var flag int
			if err := rows.Scan(&id, &as, &flag); err != nil {
				rows.Close()
				return asID, unconfirmed
			}
			asID[id] = as
			if flag == 0 {
				unconfirmed[id] = true
			}
		}
		rows.Close()
	}
	return asID, unconfirmed
}

// ReconcileASWorkDailyTasks 저장 경로에서 놓친 하위업무를 일정 업무에 맞춘다.
// 두 번 돌려도 내용이 같으면 다시 쓰지 않는다. source_type=as 행은 건드리지 않는다.
func ReconcileASWorkDailyTasks(db *sql.DB) (int, error) {
	if db == nil {
		return 0, nil
	}
	wb := NewWBRepo(db)
	works := NewASWorkRepo(db)
	rows, err := db.Query(`SELECT work_id FROM as_work_items ORDER BY work_number`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return 0, nil
		}
		return 0, err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			log.Printf("하위업무 일정 정리 행 읽기 실패: %v", err)
			continue
		}
		w, err := works.GetByID(id)
		if err != nil || w == nil {
			log.Printf("하위업무 일정 정리 조회 실패 %s: %v", id, err)
			continue
		}
		if err := wb.SyncASWorkDailyTask(w); err != nil {
			log.Printf("하위업무 일정 정리 실패 %s: %v", id, err)
			continue
		}
		n++
	}
	if err := rows.Err(); err != nil {
		return n, err
	}
	if _, err := wb.deleteOrphanASWorkTasks(); err != nil {
		return n, err
	}
	return n, nil
}

func (r *WBRepo) deleteOrphanASWorkTasks() (int, error) {
	rows, err := r.db.Query(`
		SELECT task_id FROM work_tasks
		 WHERE source_type=?
		   AND TRIM(COALESCE(source_id,'')) != ''
		   AND source_id NOT IN (SELECT work_id FROM as_work_items)`, model.WBSourceASWork)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return 0, nil
		}
		return 0, err
	}
	defer rows.Close()
	n := 0
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return n, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return n, err
	}
	for _, id := range ids {
		if err := r.DeleteTask(id); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// BackfillASWorkTasks 이미 만들어진 하위업무 중 일정 업무가 없는 건을 채운다.
// dry-run 은 조회만 하고 work_tasks 를 쓰지 않는다. apply 는 한 트랜잭션이며 이미 있으면 건너뛴다.
func (r *WBRepo) BackfillASWorkTasks(dryRun bool) (ASWorkBackfillReport, error) {
	var rep ASWorkBackfillReport
	if r == nil || r.db == nil {
		return rep, nil
	}
	rows, err := r.db.Query(`
		SELECT w.work_id,
		       CASE WHEN t.task_id IS NULL THEN 0 ELSE 1 END,
		       CASE WHEN ar.as_id IS NULL THEN 1 ELSE 0 END,
		       COALESCE(ar.org_id,'')
		  FROM as_work_items w
		  LEFT JOIN as_receipts ar ON ar.as_id = w.as_id
		  LEFT JOIN work_tasks t ON t.source_type=? AND t.source_id=w.work_id AND TRIM(COALESCE(t.source_role,''))=''
		 ORDER BY w.work_number`, model.WBSourceASWork)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return rep, nil
		}
		return rep, err
	}
	defer rows.Close()
	var todo []string
	for rows.Next() {
		var id, org string
		var hasTask, noReceipt int
		if err := rows.Scan(&id, &hasTask, &noReceipt, &org); err != nil {
			return rep, err
		}
		switch {
		case hasTask == 1:
			rep.Already++
		case noReceipt == 1:
			rep.Skipped++
			rep.Reasons = append(rep.Reasons, id+" 접수 없음")
		case strings.TrimSpace(org) == "":
			rep.Skipped++
			rep.Reasons = append(rep.Reasons, id+" 조직 없음")
		default:
			rep.Missing++
			todo = append(todo, id)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return rep, err
	}
	rows.Close()
	if dryRun || len(todo) == 0 {
		return rep, nil
	}
	works := NewASWorkRepo(r.db)
	var pending []*model.WorkTask
	for _, id := range todo {
		w, err := works.GetByID(id)
		if err != nil || w == nil {
			rep.Skipped++
			rep.Missing--
			rep.Reasons = append(rep.Reasons, id+" 조회 실패")
			continue
		}
		ctx, err := r.loadASWorkReceipt(w.ASID)
		if err != nil {
			return rep, err
		}
		if ctx == nil || strings.TrimSpace(ctx.OrgID) == "" {
			rep.Skipped++
			rep.Missing--
			rep.Reasons = append(rep.Reasons, id+" 조직 없음")
			continue
		}
		task := asWorkTaskFrom(w, ctx, nil)
		normalizeWorkTask(task)
		task.Assignee, task.AssigneeUserID = bindStaff(r.db, task.Assignee, task.AssigneeUserID)
		stampNewWorkTaskDates(task)
		if err := r.assignNewTaskID(task); err != nil {
			return rep, err
		}
		pending = append(pending, task)
	}
	if len(pending) == 0 {
		return rep, nil
	}
	tx, err := r.db.Begin()
	if err != nil {
		return rep, err
	}
	defer tx.Rollback()
	for _, task := range pending {
		var n int
		if err := tx.QueryRow(`
			SELECT COUNT(*) FROM work_tasks
			 WHERE source_type=? AND source_id=? AND TRIM(COALESCE(source_role,''))=''`,
			model.WBSourceASWork, task.SourceID).Scan(&n); err != nil {
			return rep, err
		}
		if n > 0 {
			rep.Already++
			rep.Missing--
			continue
		}
		if err := execWorkTaskInsert(tx, task); err != nil {
			return rep, err
		}
		rep.Created++
	}
	if err := tx.Commit(); err != nil {
		return rep, err
	}
	return rep, nil
}

// deleteASWorkTasksTx 접수를 지울 때 하위업무 일정 행을 같은 트랜잭션에서 지운다.
// as_work_items 를 지우기 전에 부른다.
func deleteASWorkTasksTx(tx *sql.Tx, asID string) error {
	taskSub := `SELECT task_id FROM work_tasks WHERE source_type='as_work' AND source_id IN (SELECT work_id FROM as_work_items WHERE as_id=?)`
	for _, q := range []string{
		`DELETE FROM work_task_segments WHERE task_id IN (` + taskSub + `)`,
		`DELETE FROM work_task_tags WHERE task_id IN (` + taskSub + `)`,
		`DELETE FROM work_task_sites WHERE task_id IN (` + taskSub + `)`,
		`DELETE FROM work_task_assets WHERE task_id IN (` + taskSub + `)`,
		`DELETE FROM work_activities WHERE task_id IN (` + taskSub + `)`,
		`DELETE FROM work_actions WHERE task_id IN (` + taskSub + `)`,
		`DELETE FROM work_tasks WHERE source_type='as_work' AND source_id IN (SELECT work_id FROM as_work_items WHERE as_id=?)`,
	} {
		if err := execIgnoreMissingTx(tx, q, asID); err != nil {
			return err
		}
	}
	return nil
}

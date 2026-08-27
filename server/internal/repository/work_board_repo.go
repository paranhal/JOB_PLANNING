package repository

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"customer-support/internal/model"
)

// WorkBoardRepo 담당자 대시보드·통합 업무 리스트
type WorkBoardRepo struct {
	db *sql.DB
}

func NewWorkBoardRepo(db *sql.DB) *WorkBoardRepo {
	return &WorkBoardRepo{db: db}
}

func (r *WorkBoardRepo) DashStats(mineUserID string, mineKeys []string) (*model.WorkDashStats, error) {
	s := &model.WorkDashStats{}
	today := time.Now().Format("2006-01-02")
	// 완료 접수에 남은 open 하부업무(확인·재방문) 정리 — 지연 오표시 방지
	_, _ = NewASWorkRepo(r.db).CloseOpenUnderClosedReceipts()

	openItems, err := r.collectOpen(mineUserID, mineKeys)
	if err != nil {
		return nil, err
	}
	s.OpenTotal = len(openItems)
	s.OpenByAssignee = buildAssigneePrefixStats(openItems)

	todayItems, err := r.collectByDate(mineUserID, mineKeys, today, false)
	if err != nil {
		return nil, err
	}
	for _, it := range todayItems {
		s.TodayScheduled.Add(it.Prefix, 1)
	}
	s.TodayByAssignee = buildAssigneePrefixStats(todayItems)

	delayed, err := r.collectDelayed(mineUserID, mineKeys, today)
	if err != nil {
		return nil, err
	}
	s.Delayed = len(delayed)
	s.DelayedByAssignee = buildAssigneePrefixStats(delayed)

	done, err := r.collectCompletedOn(mineUserID, mineKeys, today)
	if err != nil {
		return nil, err
	}
	for _, it := range done {
		s.TodayCompleted.Add(it.Prefix, 1)
	}
	s.CompletedByAssignee = buildAssigneePrefixStats(done)

	pending, err := r.collectSchedulePending(mineUserID, mineKeys)
	if err != nil {
		return nil, err
	}
	s.SchedulePending = len(pending)
	s.PendingByAssignee = buildAssigneePrefixStats(pending)

	// 담당자 미배정은 항상 전체 — 담당자 대신 접두어 합계만
	unassigned, err := r.collectUnassigned()
	if err != nil {
		return nil, err
	}
	s.Unassigned = len(unassigned)
	var uc model.WorkPrefixCounts
	for _, it := range unassigned {
		uc.Add(it.Prefix, 1)
	}
	s.UnassignedPrefixLine = formatBracketPrefixCounts(uc)

	unplanned, _, err := r.ListUnplanned(mineUserID, mineKeys, "")
	if err != nil {
		return nil, err
	}
	s.Unplanned = len(unplanned)
	return s, nil
}

func (r *WorkBoardRepo) ListBucket(bucket, mineUserID string, mineKeys []string, limit int) ([]model.WorkListItem, error) {
	today := time.Now().Format("2006-01-02")
	if bucket == model.WorkBucketDelayed || bucket == "" || bucket == "open" {
		_, _ = NewASWorkRepo(r.db).CloseOpenUnderClosedReceipts()
	}
	var items []model.WorkListItem
	var err error
	switch bucket {
	case model.WorkBucketToday:
		items, err = r.collectByDate(mineUserID, mineKeys, today, false)
	case model.WorkBucketDelayed:
		items, err = r.collectDelayed(mineUserID, mineKeys, today)
	case model.WorkBucketCompletedToday:
		items, err = r.collectCompletedOn(mineUserID, mineKeys, today)
	case model.WorkBucketSchedulePending:
		items, err = r.collectSchedulePending(mineUserID, mineKeys)
	case model.WorkBucketUnassigned:
		items, err = r.collectUnassigned()
	default: // open
		items, err = r.collectOpen(mineUserID, mineKeys)
	}
	if err != nil {
		return nil, err
	}
	return sortLimitWorkItems(items, limit), nil
}

// ListScheduledOn 기준일 예정 목록(회의·업무 리스트 공용)
func (r *WorkBoardRepo) ListScheduledOn(date, mineUserID string, mineKeys []string, limit int) ([]model.WorkListItem, error) {
	items, err := r.collectByDate(mineUserID, mineKeys, date, false)
	if err != nil {
		return nil, err
	}
	return sortLimitWorkItems(items, limit), nil
}

// ListCompletedOn 기준일 완료(실적) 목록. AS는 부분완료 포함.
func (r *WorkBoardRepo) ListCompletedOn(date, mineUserID string, mineKeys []string, limit int) ([]model.WorkListItem, error) {
	items, err := r.collectCompletedOn(mineUserID, mineKeys, date)
	if err != nil {
		return nil, err
	}
	return sortLimitWorkItems(items, limit), nil
}

func sortLimitWorkItems(items []model.WorkListItem, limit int) []model.WorkListItem {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].ScheduledDate != items[j].ScheduledDate {
			return items[i].ScheduledDate < items[j].ScheduledDate
		}
		return items[i].RefNumber < items[j].RefNumber
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items
}

func (r *WorkBoardRepo) collectOpen(mineUserID string, mineKeys []string) ([]model.WorkListItem, error) {
	var out []model.WorkListItem
	asItems, err := r.queryAS(`ar.status IN ('received','assigned','in_progress','hold','transfer')`, mineUserID, mineKeys, nil)
	if err != nil {
		return nil, err
	}
	out = append(out, asItems...)

	wItems, err := r.queryWorkItems(
		`w.status='open' AND ar.status NOT IN ('completed','closed','cancelled')`,
		mineUserID, mineKeys, nil)
	if err != nil {
		return nil, err
	}
	out = append(out, wItems...)

	// 정기점검: 오늘 이후(포함) 미도래·오늘 — 진행중으로 간주
	today := time.Now().Format("2006-01-02")
	mItems, err := r.queryMaintenance(`mv.visit_date >= ? AND COALESCE(mv.completed,0)=0`, []interface{}{today}, 200)
	if err != nil {
		return nil, err
	}
	// 정기점검은 담당자 필드 없음 → 기술담당 mine일 때도 전원 노출(팀 공통 일정)
	out = append(out, mItems...)

	gItems, err := r.queryGeneral(`wo.phase != 'complete'`, nil)
	if err != nil {
		return nil, err
	}
	out = append(out, gItems...)
	return out, nil
}

func (r *WorkBoardRepo) collectByDate(mineUserID string, mineKeys []string, date string, completed bool) ([]model.WorkListItem, error) {
	var out []model.WorkListItem
	if !completed {
		asItems, err := r.queryAS(
			`ar.status IN ('received','assigned','in_progress','hold','transfer')
			 AND COALESCE(ar.schedule_confirmed,0)=1
			 AND date(ar.visit_scheduled_date)=date(?)`,
			mineUserID, mineKeys, []interface{}{date})
		if err != nil {
			return nil, err
		}
		out = append(out, asItems...)

		wItems, err := r.queryWorkItems(
			`w.status='open' AND ar.status NOT IN ('completed','closed','cancelled')
			 AND COALESCE(w.schedule_confirmed,0)=1 AND date(w.scheduled_date)=date(?)`,
			mineUserID, mineKeys, []interface{}{date})
		if err != nil {
			return nil, err
		}
		out = append(out, wItems...)

		mItems, err := r.queryMaintenance(
			`mv.visit_date = ? AND COALESCE(mv.completed,0)=0`, []interface{}{date}, 200)
		if err != nil {
			return nil, err
		}
		out = append(out, mItems...)

		gItems, err := r.queryGeneral(`wo.work_date = ? AND wo.phase != 'complete'`, []interface{}{date})
		if err != nil {
			return nil, err
		}
		out = append(out, gItems...)

		tItems, err := r.queryWorkTasks(
			`t.work_type IN ('admin','support')
			 AND t.status != 'complete'
			 AND date(COALESCE(NULLIF(TRIM(t.work_date),''), NULLIF(TRIM(t.due_date),''), ''))=date(?)`,
			mineUserID, mineKeys, []interface{}{date})
		if err != nil {
			return nil, err
		}
		out = append(out, tItems...)
	}
	return out, nil
}

func (r *WorkBoardRepo) collectDelayed(mineUserID string, mineKeys []string, today string) ([]model.WorkListItem, error) {
	var out []model.WorkListItem
	asItems, err := r.queryAS(
		`ar.status IN ('received','assigned','in_progress','hold','transfer')
		 AND COALESCE(ar.visit_scheduled_date,'') != ''
		 AND COALESCE(ar.schedule_confirmed,0)=1
		 AND date(ar.visit_scheduled_date) < date(?)
		 AND NOT `+visitAlreadyDone("ar."),
		mineUserID, mineKeys, []interface{}{today})
	if err != nil {
		return nil, err
	}
	for i := range asItems {
		asItems[i].DaysOverdue = daysBetween(asItems[i].ScheduledDate, today)
	}
	out = append(out, asItems...)

	// 원 접수가 이미 완료·종료면 하부 확인/재방문은 지연으로 세지 않는다.
	wItems, err := r.queryWorkItems(
		`w.status='open' AND ar.status NOT IN ('completed','closed','cancelled')
		 AND COALESCE(w.scheduled_date,'') != ''
		 AND COALESCE(w.schedule_confirmed,0)=1
		 AND date(w.scheduled_date) < date(?)`,
		mineUserID, mineKeys, []interface{}{today})
	if err != nil {
		return nil, err
	}
	for i := range wItems {
		wItems[i].DaysOverdue = daysBetween(wItems[i].ScheduledDate, today)
	}
	out = append(out, wItems...)

	mItems, err := r.queryMaintenance(
		`mv.visit_date < ? AND COALESCE(mv.completed,0)=0`, []interface{}{today}, 200)
	if err != nil {
		return nil, err
	}
	// 과거 정기점검은 계획상 남아 있을 수 있어 이번 달만
	monthStart := time.Now().Format("2006-01") + "-01"
	for _, it := range mItems {
		if it.ScheduledDate >= monthStart {
			it.DaysOverdue = daysBetween(it.ScheduledDate, today)
			out = append(out, it)
		}
	}

	wa, err := r.collectOverdueWaitingActions(mineUserID, mineKeys, today)
	if err != nil {
		return nil, err
	}
	out = append(out, wa...)

	_, _ = NewWBRepo(r.db).MarkPastOccurrencesOverdue(today)
	occItems, err := r.queryWorkTasks(
		`COALESCE(t.recurrence_role,'')='occurrence'
		 AND COALESCE(t.status,'') NOT IN ('complete','cancelled')
		 AND COALESCE(t.occurrence_status,'') NOT IN ('complete','skipped')
		 AND (
			COALESCE(t.occurrence_status,'')='overdue'
			OR (TRIM(COALESCE(t.work_date,'')) != '' AND date(t.work_date) < date(?)
			    AND COALESCE(t.occurrence_status,'') IN ('scheduled','in_progress',''))
			OR (COALESCE(t.occurrence_status,'')='deferred'
			    AND (TRIM(COALESCE(t.next_check_date,''))='' OR date(t.next_check_date) < date(?)))
		 )`,
		mineUserID, mineKeys, []interface{}{today, today})
	if err != nil {
		return nil, err
	}
	for i := range occItems {
		occItems[i].DaysOverdue = daysBetween(occItems[i].ScheduledDate, today)
		occItems[i].SubLabel = "미완료"
		out = append(out, occItems[i])
	}
	return out, nil
}

func (r *WorkBoardRepo) collectCompletedOn(mineUserID string, mineKeys []string, date string) ([]model.WorkListItem, error) {
	var out []model.WorkListItem
	asItems, err := r.queryAS(
		`ar.status IN `+model.SQLStatusStatsCompleted+`
		 AND date(COALESCE(ar.complete_datetime, ar.updated_at))=date(?)`,
		mineUserID, mineKeys, []interface{}{date})
	if err != nil {
		return nil, err
	}
	out = append(out, asItems...)

	wItems, err := r.queryWorkItems(
		`w.status='done' AND date(w.updated_at)=date(?)`,
		mineUserID, mineKeys, []interface{}{date})
	if err != nil {
		return nil, err
	}
	out = append(out, wItems...)

	mItems, err := r.queryMaintenance(
		`COALESCE(mv.completed,0)=1 AND COALESCE(NULLIF(mv.completed_date,''), mv.visit_date)=?`,
		[]interface{}{date}, 200)
	if err != nil {
		return nil, err
	}
	out = append(out, mItems...)

	gItems, err := r.queryGeneral(`wo.phase='complete' AND wo.work_date=?`, []interface{}{date})
	if err != nil {
		return nil, err
	}
	out = append(out, gItems...)

	tItems, err := r.queryWorkTasks(
		`t.work_type IN ('admin','support')
		 AND t.status='complete'
		 AND `+adminTaskCompleteDateSQL+`=date(?)`,
		mineUserID, mineKeys, []interface{}{date})
	if err != nil {
		return nil, err
	}
	out = append(out, tItems...)
	return out, nil
}

// collectSchedulePending 다음 방문 일정을 아직 잡지 않은 건.
// 일정 미확정 건과, 이미 다녀왔지만 예정일이 옛 날짜로 남아 있는 건을 함께 본다.
func (r *WorkBoardRepo) collectSchedulePending(mineUserID string, mineKeys []string) ([]model.WorkListItem, error) {
	return r.queryAS(
		`ar.status IN ('received','assigned','in_progress')
		 AND (COALESCE(ar.assigned_to,'') != '' OR COALESCE(ar.assigned_user_id,'') != '')
		 AND (COALESCE(ar.schedule_confirmed,0)=0
		      OR (COALESCE(ar.visit_scheduled_date,'') != ''
		          AND date(ar.visit_scheduled_date) < date('now','localtime')
		          AND `+visitAlreadyDone("ar.")+`))`,
		mineUserID, mineKeys, nil)
}

func (r *WorkBoardRepo) collectUnassigned() ([]model.WorkListItem, error) {
	return r.queryAS(
		`ar.status IN ('received','assigned','in_progress','hold','transfer')
		 AND COALESCE(ar.assigned_to,'') = '' AND COALESCE(ar.assigned_user_id,'') = ''`,
		"", nil, nil)
}

func (r *WorkBoardRepo) queryAS(extraWhere, mineUserID string, mineKeys []string, extraArgs []interface{}) ([]model.WorkListItem, error) {
	q := `
		SELECT ar.as_id, ar.as_number, c.org_name, COALESCE(ar.symptom,''),
		       COALESCE(ar.assigned_to,''), COALESCE(ar.visit_scheduled_date,''), ar.status
		FROM as_receipts ar
		JOIN customers c ON c.customer_id = ar.customer_id
		WHERE ` + extraWhere
	args := append([]interface{}{}, extraArgs...)
	if mineCond, mineArgs := mineAssigneeCond("ar.", mineUserID, mineKeys); mineCond != "" {
		q += mineCond
		args = append(args, mineArgs...)
	}
	q += ` ORDER BY ar.visit_scheduled_date, ar.as_number LIMIT 5000`
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []model.WorkListItem
	for rows.Next() {
		var it model.WorkListItem
		var status string
		if err := rows.Scan(&it.RefID, &it.RefNumber, &it.OrgName, &it.Title, &it.Assignee, &it.ScheduledDate, &status); err != nil {
			return nil, err
		}
		it.Prefix = model.WorkPrefixAS
		it.Status = status
		it.StatusLabel = asStatusLabel(status)
		it.Href = "/as/" + it.RefID
		items = append(items, it)
	}
	return items, rows.Err()
}

func (r *WorkBoardRepo) queryWorkItems(extraWhere, mineUserID string, mineKeys []string, extraArgs []interface{}) ([]model.WorkListItem, error) {
	q := `
		SELECT w.work_id, w.work_number, w.work_kind, w.as_id, c.org_name,
		       COALESCE(NULLIF(TRIM(w.assigned_to),''), ar.assigned_to,''),
		       COALESCE(w.scheduled_date,''), w.status,
		       COALESCE(w.confirm_target,''), COALESCE(w.notes,''), ar.as_number
		FROM as_work_items w
		JOIN as_receipts ar ON ar.as_id = w.as_id
		JOIN customers c ON c.customer_id = ar.customer_id
		WHERE ` + extraWhere
	args := append([]interface{}{}, extraArgs...)
	// 하부업무는 자체 담당자 기준(미배정 시 원 접수 담당자 fallback)
	if mineCond, mineArgs := mineAssigneeCondWork(mineUserID, mineKeys); mineCond != "" {
		q += mineCond
		args = append(args, mineArgs...)
	}
	q += ` ORDER BY w.scheduled_date, w.work_number LIMIT 5000`
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []model.WorkListItem
	for rows.Next() {
		var it model.WorkListItem
		var kind, asID, target, notes, asNum string
		if err := rows.Scan(&it.RefID, &it.RefNumber, &kind, &asID, &it.OrgName,
			&it.Assignee, &it.ScheduledDate, &it.Status, &target, &notes, &asNum); err != nil {
			return nil, err
		}
		if kind == model.WorkKindConfirm {
			it.Prefix = model.WorkPrefixConfirm
			it.SubLabel = "확인"
			it.Title = strings.TrimSpace(target)
			if it.Title == "" {
				it.Title = notes
			}
			if it.Title == "" {
				it.Title = "회신·결과 확인"
			}
		} else {
			// 재방문은 AS 계열로 표시
			it.Prefix = model.WorkPrefixAS
			it.SubLabel = "재방문"
			it.Title = notes
			if it.Title == "" {
				it.Title = "재방문 · " + asNum
			}
		}
		it.StatusLabel = workItemStatusLabel(it.Status)
		it.Href = "/as/work/" + it.RefID + "/action"
		items = append(items, it)
	}
	return items, rows.Err()
}

// mineAssigneeCondWork 하부업무 담당자(없으면 원 접수 담당자)로 "내 업무" 필터
func mineAssigneeCondWork(mineUserID string, mineKeys []string) (string, []interface{}) {
	names := cleanAssignees(mineKeys)
	userID := strings.TrimSpace(mineUserID)
	if userID == "" && len(names) == 0 {
		return "", nil
	}
	parts := []string{}
	args := []interface{}{}
	if userID != "" {
		parts = append(parts, `(TRIM(COALESCE(NULLIF(TRIM(w.assigned_user_id),''), ar.assigned_user_id,'')) != '' AND TRIM(COALESCE(NULLIF(TRIM(w.assigned_user_id),''), ar.assigned_user_id,'')) = ?)`)
		args = append(args, userID)
	}
	if len(names) > 0 {
		ph := make([]string, len(names))
		for i, n := range names {
			ph[i] = "?"
			args = append(args, n)
		}
		parts = append(parts, `TRIM(COALESCE(NULLIF(TRIM(w.assigned_to),''), ar.assigned_to,'')) IN (`+strings.Join(ph, ",")+`)`)
	}
	return ` AND (` + strings.Join(parts, " OR ") + `)`, args
}

func (r *WorkBoardRepo) queryMaintenance(extraWhere string, args []interface{}, limit int) ([]model.WorkListItem, error) {
	q := fmt.Sprintf(`
		SELECT mv.visit_id, mv.visit_date, COALESCE(cfg.short_name, c.org_name), c.org_name,
		       COALESCE(mv.entry_category,''), mv.plan_id,
		       COALESCE(mv.completed,0), COALESCE(mv.assignee,''), COALESCE(mv.product_type,'')
		FROM maintenance_visits mv
		JOIN customers c ON c.customer_id = mv.customer_id
		LEFT JOIN maintenance_site_config cfg ON cfg.customer_id = mv.customer_id
		WHERE %s
		ORDER BY mv.visit_date, c.org_name
		LIMIT %d`, extraWhere, limit)
	rows, err := r.db.Query(q, args...)
	if err != nil {
		// 테이블 없으면 빈 목록
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var items []model.WorkListItem
	for rows.Next() {
		var it model.WorkListItem
		var planID, cat, product string
		var completed int
		if err := rows.Scan(&it.RefID, &it.ScheduledDate, &it.Title, &it.OrgName, &cat, &planID,
			&completed, &it.Assignee, &product); err != nil {
			return nil, err
		}
		it.Prefix = model.WorkPrefixMaintenance
		it.RefNumber = it.ScheduledDate
		if completed == 1 {
			it.Status = "done"
			it.StatusLabel = "방문 완료"
		} else {
			it.Status = "planned"
			it.StatusLabel = "예정"
		}
		it.Href = "/maintenance/" + planID
		if product != "" {
			it.SubLabel = product
		} else if cat != "" {
			it.SubLabel = cat
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

func (r *WorkBoardRepo) queryGeneral(extraWhere string, args []interface{}) ([]model.WorkListItem, error) {
	q := `
		SELECT other_id, work_date, title, COALESCE(org_name,''), phase
		FROM work_other wo
		WHERE ` + extraWhere + `
		ORDER BY work_date, title LIMIT 200`
	rows, err := r.db.Query(q, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var items []model.WorkListItem
	for rows.Next() {
		var it model.WorkListItem
		var phase string
		if err := rows.Scan(&it.RefID, &it.ScheduledDate, &it.Title, &it.OrgName, &phase); err != nil {
			return nil, err
		}
		it.Prefix = model.WorkPrefixGeneral
		it.RefNumber = it.RefID
		it.Status = phase
		if phase == "complete" {
			it.StatusLabel = "완료"
		} else {
			it.StatusLabel = "진행"
		}
		it.Href = "/work-status"
		items = append(items, it)
	}
	return items, rows.Err()
}

func (r *WorkBoardRepo) queryWorkTasks(extraWhere, mineUserID string, mineKeys []string, extraArgs []interface{}) ([]model.WorkListItem, error) {
	q := `
		SELECT t.task_id, COALESCE(NULLIF(TRIM(t.work_date),''), NULLIF(TRIM(t.due_date),''), ''),
		       t.title, COALESCE(t.assignee,''), t.status, t.work_type,
		       COALESCE(p.short_name, p.name, '')
		FROM work_tasks t
		LEFT JOIN work_projects p ON p.project_id = t.project_id
		WHERE ` + extraWhere
	args := append([]interface{}{}, extraArgs...)
	if mineCond, mineArgs := mineTaskAssigneeCond(mineUserID, mineKeys); mineCond != "" {
		q += mineCond
		args = append(args, mineArgs...)
	}
	q += ` ORDER BY 2, t.task_id LIMIT 5000`
	rows, err := r.db.Query(q, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var items []model.WorkListItem
	for rows.Next() {
		var it model.WorkListItem
		var workType, project string
		if err := rows.Scan(&it.RefID, &it.ScheduledDate, &it.Title, &it.Assignee, &it.Status, &workType, &project); err != nil {
			return nil, err
		}
		it.Prefix = model.WorkPrefixGeneral
		it.RefNumber = it.RefID
		it.OrgName = project
		if workType == model.WBWorkSupport {
			it.SubLabel = "지원"
		} else {
			it.SubLabel = "행정"
		}
		it.StatusLabel = workTaskStatusLabel(it.Status)
		it.Href = "/workboard/tasks/" + it.RefID
		items = append(items, it)
	}
	return items, rows.Err()
}

// collectOverdueWaitingActions 회신 예정일이 지난 미확인 다음 행동(§13.6·§13.11).
func (r *WorkBoardRepo) collectOverdueWaitingActions(mineUserID string, mineKeys []string, today string) ([]model.WorkListItem, error) {
	q := `
		SELECT a.action_id, t.task_id, a.title, COALESCE(a.wait_party,''), COALESCE(t.assignee,''),
		       a.reply_due_date, COALESCE(t.work_type,'admin')
		FROM work_actions a
		JOIN work_tasks t ON t.task_id = a.task_id
		WHERE a.status='waiting' AND COALESCE(a.confirmed,0)=0
		  AND TRIM(COALESCE(a.reply_due_date,'')) != ''
		  AND date(a.reply_due_date) < date(?)
		  AND COALESCE(t.status,'') NOT IN ('complete','cancelled')
		  AND t.work_type IN ('admin','support')
		  AND TRIM(COALESCE(t.source_type,'')) = ''`
	args := []interface{}{today}
	if mineCond, mineArgs := mineTaskAssigneeCond(mineUserID, mineKeys); mineCond != "" {
		q += mineCond
		args = append(args, mineArgs...)
	}
	q += ` ORDER BY a.reply_due_date, a.action_id LIMIT 500`
	rows, err := r.db.Query(q, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var items []model.WorkListItem
	for rows.Next() {
		var it model.WorkListItem
		var taskID, waitParty, workType string
		if err := rows.Scan(&it.RefID, &taskID, &it.Title, &waitParty, &it.Assignee, &it.ScheduledDate, &workType); err != nil {
			return nil, err
		}
		it.Prefix = model.WorkPrefixGeneral
		it.RefNumber = taskID
		it.OrgName = waitParty
		it.SubLabel = "회신 대기"
		it.Status = model.WBActionWaiting
		it.StatusLabel = "회신 대기"
		it.Href = "/workboard/tasks/" + taskID
		it.DaysOverdue = daysBetween(it.ScheduledDate, today)
		items = append(items, it)
	}
	return items, rows.Err()
}

// mineTaskAssigneeCond work_tasks.assignee(표시명) 기준 본인 필터
func mineTaskAssigneeCond(mineUserID string, mineKeys []string) (string, []interface{}) {
	names := cleanAssignees(mineKeys)
	mineUserID = strings.TrimSpace(mineUserID)
	if mineUserID == "" && len(names) == 0 {
		return "", nil
	}
	parts := []string{}
	args := []interface{}{}
	if len(names) > 0 {
		ph := make([]string, len(names))
		for i, n := range names {
			ph[i] = "?"
			args = append(args, n)
		}
		parts = append(parts, fmt.Sprintf("TRIM(COALESCE(t.assignee,'')) IN (%s)", strings.Join(ph, ",")))
	}
	if mineUserID != "" {
		parts = append(parts, `TRIM(COALESCE(t.assignee,'')) = ?`)
		args = append(args, mineUserID)
	}
	return ` AND (` + strings.Join(parts, " OR ") + `)`, args
}

func workTaskStatusLabel(s string) string {
	switch s {
	case model.WBTaskComplete:
		return "완료"
	case model.WBTaskInProgress:
		return "진행중"
	case model.WBTaskHold:
		return "보류"
	case model.WBTaskTransfer:
		return "이관"
	case model.WBTaskReview:
		return "검토"
	case model.WBTaskWaiting:
		return "할 일"
	case model.WBTaskInbox:
		return "수집함"
	case model.WBTaskWaitingFor:
		return "회신 대기"
	case model.WBTaskCancelled:
		return "취소"
	default:
		return s
	}
}

func asStatusLabel(s string) string {
	m := map[string]string{
		"received": "접수", "assigned": "담당자 배정", "in_progress": "진행중", "hold": "보류",
		"transfer": "이관", "cancelled": "접수취소", "completed": "완료", "closed": "종료",
		"partial_complete": "부분완료",
	}
	if l, ok := m[s]; ok {
		return l
	}
	return s
}

func workItemStatusLabel(s string) string {
	if s == "done" {
		return "완료"
	}
	return "진행중"
}

func daysBetween(from, to string) int {
	if from == "" || to == "" {
		return 0
	}
	a, err1 := time.ParseInLocation("2006-01-02", from, time.Local)
	b, err2 := time.ParseInLocation("2006-01-02", to, time.Local)
	if err1 != nil || err2 != nil {
		return 0
	}
	return int(b.Sub(a).Hours() / 24)
}

func buildAssigneePrefixStats(items []model.WorkListItem) []model.AssigneePrefixStats {
	order := []string{}
	by := map[string]*model.WorkPrefixCounts{}
	for _, it := range items {
		name := strings.TrimSpace(it.Assignee)
		if name == "" {
			name = "(미배정)"
		}
		if _, ok := by[name]; !ok {
			by[name] = &model.WorkPrefixCounts{}
			order = append(order, name)
		}
		by[name].Add(it.Prefix, 1)
	}
	sort.Strings(order)
	// 미배정은 맨 뒤
	out := make([]model.AssigneePrefixStats, 0, len(order))
	var unassigned *model.AssigneePrefixStats
	for _, name := range order {
		c := *by[name]
		line := name + ": " + formatBracketPrefixCounts(c)
		row := model.AssigneePrefixStats{Name: name, Counts: c, Line: line}
		if name == "(미배정)" {
			unassigned = &row
			continue
		}
		out = append(out, row)
	}
	if unassigned != nil {
		out = append(out, *unassigned)
	}
	return out
}

func formatBracketPrefixCounts(c model.WorkPrefixCounts) string {
	parts := []string{}
	if c.AS > 0 {
		parts = append(parts, fmt.Sprintf("[AS] %d", c.AS))
	}
	if c.Confirm > 0 {
		parts = append(parts, fmt.Sprintf("[확인] %d", c.Confirm))
	}
	if c.Maintenance > 0 {
		parts = append(parts, fmt.Sprintf("[정기점검] %d", c.Maintenance))
	}
	if c.General > 0 {
		parts = append(parts, fmt.Sprintf("[일반업무] %d", c.General))
	}
	if len(parts) == 0 {
		return "—"
	}
	return strings.Join(parts, " / ")
}

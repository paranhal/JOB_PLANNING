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
	page, err := r.loadDash(mineUserID, mineKeys)
	if err != nil {
		return nil, err
	}
	return page.Stats, nil
}

type dashPage struct {
	Stats      *model.WorkDashStats
	Today      []model.WorkListItem
	Delayed    []model.WorkListItem
	Pending    []model.WorkListItem
	Unassigned []model.WorkListItem
}

// DashHome 대시보드 숫자와 미리보기 목록을 한 번에 만든다. collect* 를 다시 돌리지 않는다.
func (r *WorkBoardRepo) DashHome(mineUserID string, mineKeys []string, todayN, delayedN, pendingN, unassignedN int) (
	*model.WorkDashStats, []model.WorkListItem, []model.WorkListItem, []model.WorkListItem, []model.WorkListItem, error,
) {
	page, err := r.loadDash(mineUserID, mineKeys)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	return page.Stats,
		sortLimitWorkItems(page.Today, todayN),
		sortLimitWorkItems(page.Delayed, delayedN),
		sortLimitWorkItems(page.Pending, pendingN),
		sortLimitWorkItems(page.Unassigned, unassignedN),
		nil
}

func (r *WorkBoardRepo) loadDash(mineUserID string, mineKeys []string) (*dashPage, error) {
	s := &model.WorkDashStats{}
	today := time.Now().Format("2006-01-02")

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

	unplanned, _, err := r.listUnplanned(mineUserID, mineKeys, "", &unplannedBase{
		Delayed: delayed, Pending: pending, Open: openItems, Unassigned: unassigned,
	})
	if err != nil {
		return nil, err
	}
	s.Unplanned = len(unplanned)
	s.UpcomingOcc, _ = r.countUpcomingOccurrences(today, model.RecurrenceUpcomingDays)
	s.OverdueDeadline, _ = r.countOverdueRecurrenceParents(today)
	return &dashPage{
		Stats: s, Today: todayItems, Delayed: delayed, Pending: pending, Unassigned: unassigned,
	}, nil
}

func (r *WorkBoardRepo) countUpcomingOccurrences(today string, days int) (int, error) {
	if days < 1 {
		days = model.RecurrenceUpcomingDays
	}
	until, err := time.ParseInLocation("2006-01-02", today, time.Local)
	if err != nil {
		return 0, err
	}
	end := until.AddDate(0, 0, days).Format("2006-01-02")
	var n int
	err = r.db.QueryRow(`
		SELECT COUNT(*) FROM work_tasks t
		LEFT JOIN work_recurrence r ON r.task_id = t.parent_task_id
		WHERE COALESCE(t.recurrence_role,'')='occurrence'
		  AND COALESCE(t.status,'') NOT IN ('complete','cancelled')
		  AND COALESCE(t.occurrence_status,'') NOT IN ('complete','skipped')
		  AND TRIM(COALESCE(t.work_date,'')) != ''
		  AND t.work_date > ?
		  AND t.work_date <= ?
		  AND COALESCE(r.archived,0)=0`, today, end).Scan(&n)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") || strings.Contains(err.Error(), "no such column") {
			return 0, nil
		}
		return 0, err
	}
	return n, nil
}

func (r *WorkBoardRepo) countOverdueRecurrenceParents(today string) (int, error) {
	var n int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM work_tasks t
		JOIN work_recurrence r ON r.task_id = t.task_id
		WHERE COALESCE(t.recurrence_role,'')='parent'
		  AND COALESCE(t.status,'') NOT IN ('complete','cancelled')
		  AND COALESCE(r.archived,0)=0
		  AND TRIM(COALESCE(t.due_date,'')) != ''
		  AND t.due_date < ?`, today).Scan(&n)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") || strings.Contains(err.Error(), "no such column") {
			return 0, nil
		}
		return 0, err
	}
	return n, nil
}

func (r *WorkBoardRepo) ListBucket(bucket, mineUserID string, mineKeys []string, limit int) ([]model.WorkListItem, error) {
	return r.ListBucketOn(bucket, time.Now().Format("2006-01-02"), mineUserID, mineKeys, limit)
}

// ListBucketOn 대시보드와 같은 collect* 산식을 선택일 기준으로 조회한다. §7.8.1
func (r *WorkBoardRepo) ListBucketOn(bucket, date, mineUserID string, mineKeys []string, limit int) ([]model.WorkListItem, error) {
	start := time.Now()
	defer logSlowSQL("work.ListBucketOn."+bucket, start)
	date = model.NormalizeAppDate(date)
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	var items []model.WorkListItem
	var err error
	switch bucket {
	case model.WorkBucketToday:
		items, err = r.collectByDate(mineUserID, mineKeys, date, false)
	case model.WorkBucketDelayed:
		items, err = r.collectDelayed(mineUserID, mineKeys, date)
	case model.WorkBucketCompletedToday:
		items, err = r.collectCompletedOn(mineUserID, mineKeys, date)
	case model.WorkBucketInProgress:
		items, err = r.collectInProgress(mineUserID, mineKeys, date)
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

// collectInProgress 오늘 예정·지연 수집기를 재사용해 진행중만 남긴다. 새 SQL 산식을 만들지 않는다.
func (r *WorkBoardRepo) collectInProgress(mineUserID string, mineKeys []string, date string) ([]model.WorkListItem, error) {
	todayItems, err := r.collectByDate(mineUserID, mineKeys, date, false)
	if err != nil {
		return nil, err
	}
	delayed, err := r.collectDelayed(mineUserID, mineKeys, date)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	merged := appendUniqueWorkItems(nil, todayItems, seen)
	merged = appendUniqueWorkItems(merged, delayed, seen)
	var out []model.WorkListItem
	for _, it := range merged {
		if model.WBKanbanBucket(model.MapASStatusToWB(it.Status)) == model.WBTaskInProgress {
			out = append(out, it)
		}
	}
	return out, nil
}

// ListScheduledOn 기준일 예정 목록. §38.5 완료를 포함하고 schedule_confirmed 는 보지 않는다.
func (r *WorkBoardRepo) ListScheduledOn(date, mineUserID string, mineKeys []string, limit int) ([]model.WorkListItem, error) {
	start := time.Now()
	defer logSlowSQL("work.ListScheduledOn", start)
	items, err := r.collectByDate(mineUserID, mineKeys, date, true)
	if err != nil {
		return nil, err
	}
	return sortLimitWorkItems(items, limit), nil
}

// ListCompletedOn 기준일 완료(실적) 목록. AS는 부분완료 포함.
func (r *WorkBoardRepo) ListCompletedOn(date, mineUserID string, mineKeys []string, limit int) ([]model.WorkListItem, error) {
	start := time.Now()
	defer logSlowSQL("work.ListCompletedOn", start)
	items, err := r.collectCompletedOn(mineUserID, mineKeys, date)
	if err != nil {
		return nil, err
	}
	return sortLimitWorkItems(items, limit), nil
}

func workItemKey(it model.WorkListItem) string {
	return it.Prefix + ":" + it.RefID
}

func appendUniqueWorkItems(dst []model.WorkListItem, src []model.WorkListItem, seen map[string]bool) []model.WorkListItem {
	for _, it := range src {
		k := workItemKey(it)
		if seen[k] {
			continue
		}
		seen[k] = true
		dst = append(dst, it)
	}
	return dst
}

func maintMineWhere(extra, userID string, mineKeys []string) (string, []interface{}) {
	sql, args := assigneeMatchKeys(AssigneeKindMaintenance, "mv", mergeAssigneeKeys(userID, "", mineKeys...))
	if sql == "" {
		return extra, nil
	}
	return extra + sql, args
}

func withMaintAssignee(where string, args []interface{}, userID string, mineKeys []string) (string, []interface{}) {
	w, extra := maintMineWhere(where, userID, mineKeys)
	if len(extra) == 0 {
		return w, args
	}
	out := append([]interface{}{}, args...)
	return w, append(out, extra...)
}

// ListUnified 통합 목록. AS·정기점검·행정·지원. /work/all · 대시보드가 쓴다.
// 미완료는 날짜를 깎지 않는다(§37). 완료만 from~to. 「오늘 내 업무」는 ListWorkToday.
func (r *WorkBoardRepo) ListUnified(mineUserID string, mineKeys []string, includeAllSales bool, from, to string) ([]model.WorkListItem, error) {
	if to == "" {
		to = time.Now().Format("2006-01-02")
	}
	if from == "" {
		from = to
	}
	open, err := r.collectOpen(mineUserID, mineKeys)
	if err != nil {
		return nil, err
	}
	done, err := r.collectCompletedRange(mineUserID, mineKeys, from, to)
	if err != nil {
		return nil, err
	}
	tasks, err := r.queryWorkTasks(
		`t.work_type IN ('admin','support') AND COALESCE(t.status,'') NOT IN ('complete','cancelled')`,
		mineUserID, mineKeys, nil)
	if err != nil {
		return nil, err
	}
	overdueWhere, overdueArgs := maintMineWhere(
		`COALESCE(mv.completed,0)=0 AND mv.visit_date < ?`, mineUserID, mineKeys)
	overdueArgs = append([]interface{}{to}, overdueArgs...)
	overdueM, err := r.queryMaintenance(overdueWhere, overdueArgs, 500)
	if err != nil {
		return nil, err
	}
	partial, err := r.queryAS(`ar.status='partial_complete'`, mineUserID, mineKeys, nil)
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	var out []model.WorkListItem
	out = appendUniqueWorkItems(out, open, seen)
	out = appendUniqueWorkItems(out, done, seen)
	out = appendUniqueWorkItems(out, tasks, seen)
	out = appendUniqueWorkItems(out, overdueM, seen)
	out = appendUniqueWorkItems(out, partial, seen)

	if includeAllSales {
		salesWhere := `COALESCE(t.source_type,'')='sales_activity' AND COALESCE(t.status,'') NOT IN ('cancelled')
			 AND (COALESCE(t.status,'')!='complete' OR ` + adminTaskCompleteDateSQL + `=date(?))`
		salesArgs := []interface{}{to}
		if from != to {
			salesWhere = `COALESCE(t.source_type,'')='sales_activity' AND COALESCE(t.status,'') NOT IN ('cancelled')
			 AND (COALESCE(t.status,'')!='complete' OR (` + adminTaskCompleteDateSQL + `>=date(?) AND ` + adminTaskCompleteDateSQL + `<=date(?)))`
			salesArgs = []interface{}{from, to}
		}
		sales, err := r.queryWorkTasks(salesWhere, "", nil, salesArgs)
		if err != nil {
			return nil, err
		}
		out = appendUniqueWorkItems(out, sales, seen)
	}
	return out, nil
}

// ListWorkToday §38.3 「오늘 내 업무」전용. 오늘 예정·오늘 완료·진행중(오늘 이전 시작)·지연만 담는다.
// collectOpen·ListUnified 는 대시보드·미계획·/work/all 이 그대로 쓴다. 여기서 고치지 않는다.
func (r *WorkBoardRepo) ListWorkToday(mineUserID string, mineKeys []string, today string) ([]model.WorkListItem, error) {
	if today == "" {
		today = time.Now().Format("2006-01-02")
	}

	seen := map[string]bool{}
	var out []model.WorkListItem

	open, err := r.collectWorkTodayOpen(mineUserID, mineKeys, today)
	if err != nil {
		return nil, err
	}
	out = appendUniqueWorkItems(out, open, seen)

	done, err := r.collectCompletedOn(mineUserID, mineKeys, today)
	if err != nil {
		return nil, err
	}
	out = appendUniqueWorkItems(out, done, seen)

	// 회신 예정 경과·반복 발생 지연만. AS·점검 지연은 collectWorkTodayOpen 이 이미 담는다. §38.9.5
	extras, err := r.collectWorkTodayExtras(mineUserID, mineKeys, today)
	if err != nil {
		return nil, err
	}
	out = appendUniqueWorkItems(out, extras, seen)

	stampWorkTodayOverdue(out, today)
	r.fillBlockedReasons(out)
	return out, nil
}

// collectWorkTodayOpen §38.3 미완료 넷 중 오늘 예정·진행중·지연.
// 예정일이 내일 이후이거나 아예 없는 건은 넣지 않는다(미계획 업무함).
func (r *WorkBoardRepo) collectWorkTodayOpen(mineUserID string, mineKeys []string, today string) ([]model.WorkListItem, error) {
	var out []model.WorkListItem
	asItems, err := r.queryAS(
		`ar.status IN ('received','assigned','in_progress','hold','transfer')
		 AND TRIM(COALESCE(ar.visit_scheduled_date,'')) != ''
		 AND ar.visit_scheduled_date <= ?
		 AND NOT (ar.visit_scheduled_date < ? AND `+visitAlreadyDone("ar.")+`)`,
		mineUserID, mineKeys, []interface{}{today, today})
	if err != nil {
		return nil, err
	}
	out = append(out, asItems...)

	wItems, err := r.queryWorkItems(
		`w.status='open' AND ar.status NOT IN ('completed','closed','cancelled')
		 AND TRIM(COALESCE(w.scheduled_date,'')) != ''
		 AND w.scheduled_date <= ?`,
		mineUserID, mineKeys, []interface{}{today})
	if err != nil {
		return nil, err
	}
	out = append(out, wItems...)

	mWhere, mArgs := withMaintAssignee(
		`COALESCE(mv.completed,0)=0
		 AND TRIM(COALESCE(mv.visit_date,'')) != ''
		 AND mv.visit_date <= ?`,
		[]interface{}{today}, mineUserID, mineKeys)
	mItems, err := r.queryMaintenance(mWhere, mArgs, 500)
	if err != nil {
		return nil, err
	}
	out = append(out, mItems...)

	gItems, err := r.queryGeneral(
		`wo.phase != 'complete'
		 AND TRIM(COALESCE(wo.work_date,'')) != ''
		 AND wo.work_date <= ?`,
		[]interface{}{today})
	if err != nil {
		return nil, err
	}
	out = append(out, gItems...)

	tItems, err := r.queryWorkTasks(
		`t.work_type IN ('admin','support')
		 AND COALESCE(t.status,'') NOT IN ('complete','cancelled')
		 AND TRIM(COALESCE(NULLIF(TRIM(t.work_date),''), NULLIF(TRIM(t.due_date),''), '')) != ''
		 AND COALESCE(NULLIF(TRIM(t.work_date),''), NULLIF(TRIM(t.due_date),''), '') <= ?`,
		mineUserID, mineKeys, []interface{}{today})
	if err != nil {
		return nil, err
	}
	out = append(out, tItems...)
	return out, nil
}

// collectWorkTodayExtras collectWorkTodayOpen 에 없는 오늘 내 업무:
// 회신 예정 경과, 반복 발생 지연. §38.9.5
func (r *WorkBoardRepo) collectWorkTodayExtras(mineUserID string, mineKeys []string, today string) ([]model.WorkListItem, error) {
	var out []model.WorkListItem
	wa, err := r.collectOverdueWaitingActions(mineUserID, mineKeys, today)
	if err != nil {
		return nil, err
	}
	out = append(out, wa...)
	occ, err := r.collectOverdueOccurrences(mineUserID, mineKeys, today)
	if err != nil {
		return nil, err
	}
	out = append(out, occ...)
	return out, nil
}

func stampWorkTodayOverdue(items []model.WorkListItem, today string) {
	today = strings.TrimSpace(today)
	for i := range items {
		if items[i].DaysOverdue > 0 {
			continue
		}
		sched := model.WorkItemScheduledDate(items[i])
		if sched != "" && today != "" && sched < today {
			d := model.DaysBetweenDates(sched, today)
			if d < 1 {
				d = 1
			}
			items[i].DaysOverdue = d
		}
	}
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

	today := time.Now().Format("2006-01-02")
	mWhere, mArgs := withMaintAssignee(
		`mv.visit_date >= ? AND COALESCE(mv.completed,0)=0`, []interface{}{today}, mineUserID, mineKeys)
	mItems, err := r.queryMaintenance(mWhere, mArgs, 200)
	if err != nil {
		return nil, err
	}
	out = append(out, mItems...)

	gItems, err := r.queryGeneral(`wo.phase != 'complete'`, nil)
	if err != nil {
		return nil, err
	}
	out = append(out, gItems...)
	return out, nil
}

// collectByDate 그날 예정. §38.5 예정일이 있으면 예정이고 schedule_confirmed 는 보지 않는다.
// includeComplete=true 이면 완료 건도 담는다(회의 목록·통계와 같은 「예정」).
func (r *WorkBoardRepo) collectByDate(mineUserID string, mineKeys []string, date string, includeComplete bool) ([]model.WorkListItem, error) {
	var out []model.WorkListItem
	asStatus := `ar.status IN ('received','assigned','in_progress','hold','transfer')`
	if includeComplete {
		asStatus = `ar.status NOT IN ('cancelled')`
	}
	asItems, err := r.queryAS(
		asStatus+`
			 AND TRIM(COALESCE(ar.visit_scheduled_date,'')) != ''
			 AND ar.visit_scheduled_date=?`,
		mineUserID, mineKeys, []interface{}{date})
	if err != nil {
		return nil, err
	}
	out = append(out, asItems...)

	wStatus := `w.status='open' AND ar.status NOT IN ('completed','closed','cancelled')`
	if includeComplete {
		wStatus = `ar.status NOT IN ('cancelled')`
	}
	wItems, err := r.queryWorkItems(
		wStatus+`
			 AND TRIM(COALESCE(w.scheduled_date,'')) != ''
			 AND w.scheduled_date=?`,
		mineUserID, mineKeys, []interface{}{date})
	if err != nil {
		return nil, err
	}
	out = append(out, wItems...)

	mWhere := `mv.visit_date = ?`
	if !includeComplete {
		mWhere = `mv.visit_date = ? AND COALESCE(mv.completed,0)=0`
	}
	mWhere, mArgs := withMaintAssignee(mWhere, []interface{}{date}, mineUserID, mineKeys)
	mItems, err := r.queryMaintenance(mWhere, mArgs, 200)
	if err != nil {
		return nil, err
	}
	out = append(out, mItems...)

	gWhere := `wo.work_date = ? AND wo.phase != 'complete'`
	if includeComplete {
		gWhere = `wo.work_date = ?`
	}
	gItems, err := r.queryGeneral(gWhere, []interface{}{date})
	if err != nil {
		return nil, err
	}
	out = append(out, gItems...)

	tStatus := `AND t.status != 'complete'`
	if includeComplete {
		tStatus = ``
	}
	tItems, err := r.queryWorkTasks(
		`t.work_type IN ('admin','support')
			 `+tStatus+`
			 AND COALESCE(NULLIF(TRIM(t.work_date),''), NULLIF(TRIM(t.due_date),''), '')=?`,
		mineUserID, mineKeys, []interface{}{date})
	if err != nil {
		return nil, err
	}
	out = append(out, tItems...)
	return out, nil
}

func (r *WorkBoardRepo) collectDelayed(mineUserID string, mineKeys []string, today string) ([]model.WorkListItem, error) {
	var out []model.WorkListItem
	asItems, err := r.queryAS(
		`ar.status IN ('received','assigned','in_progress','hold','transfer')
		 AND COALESCE(ar.visit_scheduled_date,'') != ''
		 AND COALESCE(ar.schedule_confirmed,0)=1
		 AND ar.visit_scheduled_date < ?
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
		 AND w.scheduled_date < ?`,
		mineUserID, mineKeys, []interface{}{today})
	if err != nil {
		return nil, err
	}
	for i := range wItems {
		wItems[i].DaysOverdue = daysBetween(wItems[i].ScheduledDate, today)
	}
	out = append(out, wItems...)

	mWhere, mArgs := withMaintAssignee(
		`mv.visit_date < ? AND COALESCE(mv.completed,0)=0`, []interface{}{today}, mineUserID, mineKeys)
	mItems, err := r.queryMaintenance(mWhere, mArgs, 200)
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

	occItems, err := r.collectOverdueOccurrences(mineUserID, mineKeys, today)
	if err != nil {
		return nil, err
	}
	out = append(out, occItems...)
	return r.withoutWaiting(out), nil
}

func (r *WorkBoardRepo) collectOverdueOccurrences(mineUserID string, mineKeys []string, today string) ([]model.WorkListItem, error) {
	occItems, err := r.queryWorkTasks(
		`COALESCE(t.recurrence_role,'')='occurrence'
		 AND COALESCE(t.status,'') NOT IN ('complete','cancelled')
		 AND COALESCE(t.occurrence_status,'') NOT IN ('complete','skipped')
		 AND (
			COALESCE(t.occurrence_status,'')='overdue'
			OR (TRIM(COALESCE(t.work_date,'')) != '' AND t.work_date < ?
			    AND COALESCE(t.occurrence_status,'') IN ('scheduled','in_progress',''))
			OR (COALESCE(t.occurrence_status,'')='deferred'
			AND (TRIM(COALESCE(t.next_check_date,''))='' OR t.next_check_date < ?))
		 )`,
		mineUserID, mineKeys, []interface{}{today, today})
	if err != nil {
		return nil, err
	}
	for i := range occItems {
		occItems[i].DaysOverdue = daysBetween(occItems[i].ScheduledDate, today)
		occItems[i].SubLabel = "미완료"
	}
	return occItems, nil
}

func (r *WorkBoardRepo) collectCompletedOn(mineUserID string, mineKeys []string, date string) ([]model.WorkListItem, error) {
	return r.collectCompletedRange(mineUserID, mineKeys, date, date)
}

func sqlDateEqOrRange(expr, from, to string) (string, []interface{}) {
	if from == to {
		return `date(` + expr + `)=date(?)`, []interface{}{to}
	}
	return `date(` + expr + `)>=date(?) AND date(` + expr + `)<=date(?)`, []interface{}{from, to}
}

func (r *WorkBoardRepo) collectCompletedRange(mineUserID string, mineKeys []string, from, to string) ([]model.WorkListItem, error) {
	if to == "" {
		to = time.Now().Format("2006-01-02")
	}
	if from == "" {
		from = to
	}
	asPred, asArgs := sqlDateEqOrRange("NULLIF(TRIM(ar.complete_datetime),'')", from, to)
	asItems, err := r.queryAS(
		`ar.status IN `+model.SQLStatusStatsCompleted+` AND `+asPred,
		mineUserID, mineKeys, asArgs)
	if err != nil {
		return nil, err
	}
	var out []model.WorkListItem
	out = append(out, asItems...)

	wPred, wArgs := sqlDateEqOrRange("w.updated_at", from, to)
	wItems, err := r.queryWorkItems(
		`w.status='done' AND `+wPred,
		mineUserID, mineKeys, wArgs)
	if err != nil {
		return nil, err
	}
	out = append(out, wItems...)

	mLimit := 200
	mArgs := []interface{}{to}
	mWhere := `COALESCE(mv.completed,0)=1 AND COALESCE(NULLIF(mv.completed_date,''), mv.visit_date)=?`
	if from != to {
		mLimit = 2000
		mWhere = `COALESCE(mv.completed,0)=1 AND COALESCE(NULLIF(mv.completed_date,''), mv.visit_date)>=? AND COALESCE(NULLIF(mv.completed_date,''), mv.visit_date)<=?`
		mArgs = []interface{}{from, to}
	}
	mWhere, mArgs = withMaintAssignee(mWhere, mArgs, mineUserID, mineKeys)
	mItems, err := r.queryMaintenance(mWhere, mArgs, mLimit)
	if err != nil {
		return nil, err
	}
	out = append(out, mItems...)

	gWhere := `wo.phase='complete' AND wo.work_date=?`
	gArgs := []interface{}{to}
	if from != to {
		gWhere = `wo.phase='complete' AND wo.work_date>=? AND wo.work_date<=?`
		gArgs = []interface{}{from, to}
	}
	gItems, err := r.queryGeneral(gWhere, gArgs)
	if err != nil {
		return nil, err
	}
	out = append(out, gItems...)

	tPred := adminTaskCompleteDateSQL + `=date(?)`
	tArgs := []interface{}{to}
	if from != to {
		tPred = adminTaskCompleteDateSQL + `>=date(?) AND ` + adminTaskCompleteDateSQL + `<=date(?)`
		tArgs = []interface{}{from, to}
	}
	tItems, err := r.queryWorkTasks(
		`t.work_type IN ('admin','support')
		 AND t.status='complete'
		 AND `+tPred,
		mineUserID, mineKeys, tArgs)
	if err != nil {
		return nil, err
	}
	out = append(out, tItems...)
	if from == to {
		for i := range out {
			out[i].CompleteDate = to
		}
	}
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
		          AND ar.visit_scheduled_date < ?
		          AND `+visitAlreadyDone("ar.")+`))`,
		mineUserID, mineKeys, []interface{}{time.Now().Format("2006-01-02")})
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
		       COALESCE(ar.assigned_to,''), COALESCE(ar.visit_scheduled_date,''), ar.status,
		       COALESCE(ar.urgency,''), COALESCE(ar.receipt_group_id,''),
		       COALESCE(ar.customer_id,''), COALESCE(ar.project_id,''),
		       COALESCE(ar.schedule_confirmed,0)
		FROM as_receipts ar
		JOIN customers c ON c.customer_id = ar.customer_id
		WHERE ` + extraWhere
	args := append([]interface{}{}, extraArgs...)
	if mineCond, mineArgs := mineAssigneeCond("ar.", mineUserID, mineKeys); mineCond != "" {
		q += mineCond
		args = append(args, mineArgs...)
	}
	q += ` ORDER BY ar.visit_scheduled_date, ar.as_number LIMIT 5000`
	rows, err := queryTimed(r.db, "work.queryAS", q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []model.WorkListItem
	for rows.Next() {
		var it model.WorkListItem
		var status string
		var confirmed int
		if err := rows.Scan(&it.RefID, &it.RefNumber, &it.OrgName, &it.Title, &it.Assignee, &it.ScheduledDate, &status, &it.Urgency, &it.ReceiptGroupID, &it.CustomerID, &it.ProjectID, &confirmed); err != nil {
			return nil, err
		}
		it.Content = it.Title
		it.Prefix = model.WorkPrefixAS
		it.Status = status
		it.StatusLabel = asStatusLabel(status)
		it.WorkDate = it.ScheduledDate
		it.DueDate = it.ScheduledDate
		it.Href = "/as/" + it.RefID
		it.Tentative = confirmed == 0
		items = append(items, it)
	}
	return items, rows.Err()
}

func (r *WorkBoardRepo) queryWorkItems(extraWhere, mineUserID string, mineKeys []string, extraArgs []interface{}) ([]model.WorkListItem, error) {
	q := `
		SELECT w.work_id, w.work_number, w.work_kind, w.as_id, c.org_name,
		       COALESCE(NULLIF(TRIM(w.assigned_to),''), ar.assigned_to,''),
		       COALESCE(w.scheduled_date,''), w.status,
		       COALESCE(w.confirm_target,''), COALESCE(w.notes,''), ar.as_number,
		       COALESCE(ar.customer_id,''), COALESCE(ar.project_id,''),
		       COALESCE(w.schedule_confirmed,0)
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
	rows, err := queryTimed(r.db, "work.queryWorkItems", q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []model.WorkListItem
	for rows.Next() {
		var it model.WorkListItem
		var kind, asID, target, notes, asNum string
		var confirmed int
		if err := rows.Scan(&it.RefID, &it.RefNumber, &kind, &asID, &it.OrgName,
			&it.Assignee, &it.ScheduledDate, &it.Status, &target, &notes, &asNum, &it.CustomerID, &it.ProjectID, &confirmed); err != nil {
			return nil, err
		}
		it.Tentative = confirmed == 0
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
			it.Content = notes
		} else {
			// 재방문은 AS 계열로 표시
			it.Prefix = model.WorkPrefixAS
			it.SubLabel = "재방문"
			it.Title = notes
			if it.Title == "" {
				it.Title = "재방문 · " + asNum
			}
			it.Content = notes
		}
		it.StatusLabel = workItemStatusLabel(it.Status)
		it.WorkDate = it.ScheduledDate
		it.DueDate = it.ScheduledDate
		it.Href = "/as/work/" + it.RefID + "/action"
		items = append(items, it)
	}
	return items, rows.Err()
}

// mineAssigneeCondWork 하부업무 담당자(없으면 원 접수 담당자). §38.4.
func mineAssigneeCondWork(mineUserID string, mineKeys []string) (string, []interface{}) {
	keys := mergeAssigneeKeys(mineUserID, "", mineKeys...)
	if len(keys) == 0 {
		return "", nil
	}
	to := `TRIM(COALESCE(NULLIF(TRIM(w.assigned_to),''), ar.assigned_to,''))`
	uid := `TRIM(COALESCE(NULLIF(TRIM(w.assigned_user_id),''), ar.assigned_user_id,''))`
	expr, args := asColsExpr(to, uid, keys)
	if expr == "" {
		return "", nil
	}
	return " AND (" + expr + ")", args
}

func (r *WorkBoardRepo) queryMaintenance(extraWhere string, args []interface{}, limit int) ([]model.WorkListItem, error) {
	q := fmt.Sprintf(`
		SELECT mv.visit_id, mv.visit_date, COALESCE(cfg.short_name, c.org_name), c.org_name,
		       COALESCE(mv.entry_category,''), mv.plan_id,
		       COALESCE(mv.completed,0), COALESCE(mv.assignee,''), COALESCE(mv.product_type,''),
		       mv.customer_id, COALESCE(mv.project_id,'')
		FROM maintenance_visits mv
		JOIN customers c ON c.customer_id = mv.customer_id
		LEFT JOIN maintenance_site_config cfg ON cfg.customer_id = mv.customer_id
		WHERE %s
		ORDER BY mv.visit_date, c.org_name
		LIMIT %d`, extraWhere, limit)
	rows, err := queryTimed(r.db, "work.queryMaintenance", q, args...)
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
		var planID, cat, product string
		var completed int
		if err := rows.Scan(&it.RefID, &it.ScheduledDate, &it.Title, &it.OrgName, &cat, &planID,
			&completed, &it.Assignee, &product, &it.CustomerID, &it.ProjectID); err != nil {
			return nil, err
		}
		it.Prefix = model.WorkPrefixMaintenance
		it.RefNumber = it.ScheduledDate
		it.WorkDate = it.ScheduledDate
		it.DueDate = it.ScheduledDate
		if completed == 1 {
			it.Status = "done"
			it.StatusLabel = "방문 완료"
		} else {
			it.Status = "planned"
			it.StatusLabel = "예정"
		}
		it.Href = "/maintenance/" + planID
		it.ProductType = product
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
	rows, err := queryTimed(r.db, "work.queryGeneral", q, args...)
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
		it.WorkDate = it.ScheduledDate
		it.DueDate = it.ScheduledDate
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
		SELECT t.task_id, COALESCE(NULLIF(TRIM(t.work_date),''), ''),
		       COALESCE(NULLIF(TRIM(t.due_date),''), ''),
		       t.title, COALESCE(t.assignee,''), t.status, t.work_type,
		       COALESCE(NULLIF(TRIM(c.org_name),''), NULLIF(TRIM(t.customer_name),''), COALESCE(p.short_name, p.name, '')),
		       COALESCE(t.source_type,''), COALESCE(t.customer_id,''), COALESCE(t.project_id,''), COALESCE(t.description,'')
		FROM work_tasks t
		LEFT JOIN work_projects p ON p.project_id = t.project_id
		LEFT JOIN customers c ON c.customer_id = t.customer_id
		WHERE ` + extraWhere + SQLRecurrenceWorkUnit
	args := append([]interface{}{}, extraArgs...)
	if mineCond, mineArgs := mineTaskAssigneeCond(mineUserID, mineKeys); mineCond != "" {
		q += mineCond
		args = append(args, mineArgs...)
	}
	q += ` ORDER BY 2, t.task_id LIMIT 5000`
	rows, err := queryTimed(r.db, "work.queryWorkTasks", q, args...)
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
		var workType, sourceType string
		if err := rows.Scan(&it.RefID, &it.WorkDate, &it.DueDate, &it.Title, &it.Assignee, &it.Status, &workType, &it.OrgName, &sourceType, &it.CustomerID, &it.ProjectID, &it.Content); err != nil {
			return nil, err
		}
		it.ScheduledDate = it.WorkDate
		if it.ScheduledDate == "" {
			it.ScheduledDate = it.DueDate
		}
		it.Prefix = model.WorkPrefixGeneral
		if sourceType == model.WBSourceSalesActivity {
			it.Prefix = model.WorkPrefixSales
		}
		it.RefNumber = it.RefID
		if workType == model.WBWorkSupport {
			it.SubLabel = "지원"
		} else if it.Prefix == model.WorkPrefixSales {
			it.SubLabel = "영업"
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
		  AND a.reply_due_date < ?
		  AND COALESCE(t.status,'') NOT IN ('complete','cancelled')
		  AND t.work_type IN ('admin','support')
		  AND TRIM(COALESCE(t.source_type,'')) = ''`
	args := []interface{}{today}
	if mineCond, mineArgs := mineTaskAssigneeCond(mineUserID, mineKeys); mineCond != "" {
		q += mineCond
		args = append(args, mineArgs...)
	}
	q += ` ORDER BY a.reply_due_date, a.action_id LIMIT 500`
	rows, err := queryTimed(r.db, "work.queryWaitingActions", q, args...)
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

// mineTaskAssigneeCond work_tasks + work_task_members. §38.4 · §7.7
func mineTaskAssigneeCond(mineUserID string, mineKeys []string) (string, []interface{}) {
	return assigneeMatchKeys(AssigneeKindTask, "t", mergeAssigneeKeys(mineUserID, "", mineKeys...))
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
	return model.DaysBetweenDates(from, to)
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

// CountMissingCompleteDates 완료인데 완료일이 없는 건. 대시보드 배너용. §4.13
func (r *WorkBoardRepo) CountMissingCompleteDates() (MissingCompleteDates, error) {
	if r == nil || r.db == nil {
		return MissingCompleteDates{}, nil
	}
	return NewWBRepo(r.db).CountMissingCompleteDates()
}

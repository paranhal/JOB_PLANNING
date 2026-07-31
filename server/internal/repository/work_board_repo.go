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
	return s, nil
}

func (r *WorkBoardRepo) ListBucket(bucket, mineUserID string, mineKeys []string, limit int) ([]model.WorkListItem, error) {
	today := time.Now().Format("2006-01-02")
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
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].ScheduledDate != items[j].ScheduledDate {
			return items[i].ScheduledDate < items[j].ScheduledDate
		}
		return items[i].RefNumber < items[j].RefNumber
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (r *WorkBoardRepo) collectOpen(mineUserID string, mineKeys []string) ([]model.WorkListItem, error) {
	var out []model.WorkListItem
	asItems, err := r.queryAS(`ar.status IN ('received','assigned','in_progress','hold','transfer')`, mineUserID, mineKeys, nil)
	if err != nil {
		return nil, err
	}
	out = append(out, asItems...)

	wItems, err := r.queryWorkItems(`w.status='open'`, mineUserID, mineKeys, nil)
	if err != nil {
		return nil, err
	}
	out = append(out, wItems...)

	// 정기점검: 오늘 이후(포함) 미도래·오늘 — 진행중으로 간주
	today := time.Now().Format("2006-01-02")
	mItems, err := r.queryMaintenance(`mv.visit_date >= ?`, []interface{}{today}, 200)
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
			`w.status='open' AND COALESCE(w.schedule_confirmed,0)=1 AND date(w.scheduled_date)=date(?)`,
			mineUserID, mineKeys, []interface{}{date})
		if err != nil {
			return nil, err
		}
		out = append(out, wItems...)

		mItems, err := r.queryMaintenance(`mv.visit_date = ?`, []interface{}{date}, 200)
		if err != nil {
			return nil, err
		}
		out = append(out, mItems...)

		gItems, err := r.queryGeneral(`wo.work_date = ? AND wo.phase != 'complete'`, []interface{}{date})
		if err != nil {
			return nil, err
		}
		out = append(out, gItems...)
	}
	return out, nil
}

func (r *WorkBoardRepo) collectDelayed(mineUserID string, mineKeys []string, today string) ([]model.WorkListItem, error) {
	var out []model.WorkListItem
	asItems, err := r.queryAS(
		`ar.status IN ('received','assigned','in_progress','hold','transfer')
		 AND COALESCE(ar.visit_scheduled_date,'') != ''
		 AND COALESCE(ar.schedule_confirmed,0)=1
		 AND date(ar.visit_scheduled_date) < date(?)`,
		mineUserID, mineKeys, []interface{}{today})
	if err != nil {
		return nil, err
	}
	for i := range asItems {
		asItems[i].DaysOverdue = daysBetween(asItems[i].ScheduledDate, today)
	}
	out = append(out, asItems...)

	wItems, err := r.queryWorkItems(
		`w.status='open' AND COALESCE(w.scheduled_date,'') != ''
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

	mItems, err := r.queryMaintenance(`mv.visit_date < ?`, []interface{}{today}, 200)
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
	return out, nil
}

func (r *WorkBoardRepo) collectCompletedOn(mineUserID string, mineKeys []string, date string) ([]model.WorkListItem, error) {
	var out []model.WorkListItem
	asItems, err := r.queryAS(
		`ar.status IN ('completed','closed')
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

	gItems, err := r.queryGeneral(`wo.phase='complete' AND wo.work_date=?`, []interface{}{date})
	if err != nil {
		return nil, err
	}
	out = append(out, gItems...)
	return out, nil
}

func (r *WorkBoardRepo) collectSchedulePending(mineUserID string, mineKeys []string) ([]model.WorkListItem, error) {
	return r.queryAS(
		`ar.status IN ('received','assigned','in_progress')
		 AND (COALESCE(ar.assigned_to,'') != '' OR COALESCE(ar.assigned_user_id,'') != '')
		 AND COALESCE(ar.schedule_confirmed,0)=0`,
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
	q += ` ORDER BY ar.visit_scheduled_date, ar.as_number LIMIT 500`
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
		       COALESCE(ar.assigned_to,''), COALESCE(w.scheduled_date,''), w.status,
		       COALESCE(w.confirm_target,''), COALESCE(w.notes,''), ar.as_number
		FROM as_work_items w
		JOIN as_receipts ar ON ar.as_id = w.as_id
		JOIN customers c ON c.customer_id = ar.customer_id
		WHERE ` + extraWhere
	args := append([]interface{}{}, extraArgs...)
	if mineCond, mineArgs := mineAssigneeCond("ar.", mineUserID, mineKeys); mineCond != "" {
		q += mineCond
		args = append(args, mineArgs...)
	}
	q += ` ORDER BY w.scheduled_date, w.work_number LIMIT 500`
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
		it.Href = "/as/" + asID
		items = append(items, it)
	}
	return items, rows.Err()
}

func (r *WorkBoardRepo) queryMaintenance(extraWhere string, args []interface{}, limit int) ([]model.WorkListItem, error) {
	q := fmt.Sprintf(`
		SELECT mv.visit_id, mv.visit_date, COALESCE(cfg.short_name, c.org_name), c.org_name,
		       COALESCE(mv.entry_category,''), mv.plan_id
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
		var planID, cat string
		if err := rows.Scan(&it.RefID, &it.ScheduledDate, &it.Title, &it.OrgName, &cat, &planID); err != nil {
			return nil, err
		}
		it.Prefix = model.WorkPrefixMaintenance
		it.RefNumber = it.ScheduledDate
		it.Status = "planned"
		it.StatusLabel = "예정"
		it.Href = "/maintenance/" + planID
		if cat != "" {
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

func asStatusLabel(s string) string {
	m := map[string]string{
		"received": "접수", "assigned": "담당자 배정", "in_progress": "진행중", "hold": "보류",
		"transfer": "이관", "cancelled": "접수취소", "completed": "완료", "closed": "종료",
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

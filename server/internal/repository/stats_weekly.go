package repository

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"customer-support/internal/model"
)

// BuildWeeklyReport §16.1 주간업무보고서. 선택 주(월~일) 한 주, 팀+담당자 행, 이벤트 리스트.
func (r *StatsRepo) BuildWeeklyReport(anchor time.Time) (model.WeeklyReport, error) {
	cols := BuildStatsPeriodColumns(model.StatsViewWeek, anchor)
	if len(cols) < 2 {
		return model.WeeklyReport{}, fmt.Errorf("주간 기간 열 없음")
	}
	cur := cols[1]
	from, toEx := cur.From, cur.ToExclusive
	weekTo := exclusiveToInclusive(toEx)
	friday := mondayToFriday(from)
	return r.buildWeeklyReportCore(from, weekTo, toEx, friday)
}

// BuildWeeklyReportRange §16.1 구조를 임의 기간에 적용. 보고서 화면의 월·기간 선택과 공유한다.
func (r *StatsRepo) BuildWeeklyReportRange(from, toEx string) (model.WeeklyReport, error) {
	weekTo := exclusiveToInclusive(toEx)
	friday := weekTo
	if t, err := time.ParseInLocation("2006-01-02", from, time.Local); err == nil {
		end, err2 := time.ParseInLocation("2006-01-02", weekTo, time.Local)
		if err2 == nil && t.Weekday() == time.Monday && end.Sub(t) >= 6*24*time.Hour && end.Sub(t) < 8*24*time.Hour {
			friday = t.AddDate(0, 0, 4).Format("2006-01-02")
		}
	}
	return r.buildWeeklyReportCore(from, weekTo, toEx, friday)
}

func exclusiveToInclusive(toEx string) string {
	if t, err := time.ParseInLocation("2006-01-02", toEx, time.Local); err == nil {
		return t.AddDate(0, 0, -1).Format("2006-01-02")
	}
	return toEx
}

func mondayToFriday(from string) string {
	if t, err := time.ParseInLocation("2006-01-02", from, time.Local); err == nil {
		return t.AddDate(0, 0, 4).Format("2006-01-02")
	}
	return from
}

func (r *StatsRepo) buildWeeklyReportCore(from, weekTo, toEx, friday string) (model.WeeklyReport, error) {
	wd, yearMissing, err := r.CountWorkingDays(from, toEx)
	if err != nil {
		return model.WeeklyReport{}, err
	}
	out := model.WeeklyReport{
		WeekFrom:           from,
		WeekTo:             weekTo,
		WeekToEx:           toEx,
		Friday:             friday,
		WorkingDays:        wd,
		HolidayYearMissing: yearMissing,
	}

	unplanned, err := r.unplannedCountsByAssignee()
	if err != nil {
		return out, err
	}

	team, err := r.buildWeeklyPersonRow(from, toEx, wd, "", true, unplanned)
	if err != nil {
		return out, err
	}
	out.PersonRows = append(out.PersonRows, team)

	names, err := r.weeklyActiveAssignees(from, toEx)
	if err != nil {
		return out, err
	}
	var people []model.WeeklyPersonRow
	for _, name := range names {
		row, err := r.buildWeeklyPersonRow(from, toEx, wd, name, false, unplanned)
		if err != nil {
			return out, err
		}
		if row.Receipt == 0 && row.Completed == 0 && row.CarryOut == 0 {
			continue
		}
		people = append(people, row)
	}
	sort.SliceStable(people, func(i, j int) bool {
		if people[i].Unassigned != people[j].Unassigned {
			return !people[i].Unassigned
		}
		if people[i].Completed != people[j].Completed {
			return people[i].Completed > people[j].Completed
		}
		return people[i].Label < people[j].Label
	})
	out.PersonRows = append(out.PersonRows, people...)

	if pc, e := NewWorkBoardRepo(r.db).CountPlanning(); e == nil {
		out.PlanningOpen = pc.Open
		out.PlanningPlanned = pc.Planned
		out.HasPlanning = pc.HasPlanningRate()
		out.PlanningRate = mathRound1(pc.Rate())
		out.PlanDisplay = model.StatsReliability(pc.Open, !pc.HasPlanningRate(), out.PlanningRate)
	}

	teamF := ParseMeetingFilter(model.StatsScopeTeam, "", "")
	b, err := r.countBucket(from, toEx, teamF)
	if err != nil {
		return out, err
	}
	out.MntQuota = b.Mnt.Quota
	out.MntReceipt = b.Mnt.Receipt
	out.MntDone = b.Mnt.Process
	out.MntCumulative = b.Mnt.Cumulative
	out.MntRemaining = b.Mnt.Remaining
	out.MntMonthLabel = b.Mnt.MonthLabel

	lead, err := r.loadAdminLeadBreakdown(from, toEx, teamF)
	if err != nil {
		return out, err
	}
	out.AdminLead = lead

	out.Events, err = r.ListWeeklyEventRows(from, toEx)
	if err != nil {
		return out, err
	}
	out.Events = r.collapseWeeklyReportEvents(out.Events)
	return out, nil
}

func weeklyFilter(assignee string) model.StatsMeetingFilter {
	if assignee == "" {
		return ParseMeetingFilter(model.StatsScopeTeam, "", "")
	}
	return ParseMeetingFilter(model.StatsScopeAssignee, assignee, "")
}

func (r *StatsRepo) buildWeeklyPersonRow(from, toEx string, workingDays int, assignee string, isTeam bool, unplanned map[string]int) (model.WeeklyPersonRow, error) {
	label := model.StatsTeamLabel
	if !isTeam {
		label = assignee
	}
	row := model.WeeklyPersonRow{
		Label:      label,
		IsTeam:     isTeam,
		Unassigned: assignee == model.StatsUnassignedLabel,
	}
	f := weeklyFilter(assignee)

	b, err := r.countBucket(from, toEx, f)
	if err != nil {
		return row, err
	}
	an, err := r.LoadStatsWorkAnalysis(from, toEx, f)
	if err != nil {
		return row, err
	}
	kpi, err := r.avgKPIForFilter(from, toEx, f)
	if err != nil {
		return row, err
	}

	row.Receipt = an.Receipt
	row.Completed = an.Completed
	row.CarryOut = an.CarryOut
	exec := mathRound1(b.ExecutionRatePct())
	row.ExecDisplay = model.StatsReliability(b.ExecPlanned(), !b.HasExecutionRate(), exec)
	if isTeam {
		if pc, e := NewWorkBoardRepo(r.db).CountPlanning(); e == nil {
			row.ExecDisplay = row.ExecDisplay.CapIfLowPlanning(pc.HasPlanningRate(), pc.Rate())
		}
	}

	row.VisitDisplay = model.StatsReliability(kpi.nVisitAS, kpi.nVisitAS == 0, mathRound1(kpi.visitAS)).
		WithReason("해당 기간 방문 데이터 없음")
	row.CompleteDisplay = model.StatsReliability(kpi.nCompleteAS, kpi.nCompleteAS == 0, mathRound1(kpi.completeAS)).
		WithReason("해당 기간 완료 데이터 없음")

	if avg, ok := model.DailyAvgCompleted(row.Completed, workingDays); ok {
		row.HasDayAvg = true
		row.DayAvg = mathRound1(avg)
	}
	row.DayMax, row.DayMaxDate, err = r.weeklyDayMaxCompleted(from, toEx, f)
	if err != nil {
		return row, err
	}
	row.InProgress, err = r.countWeeklyInProgress(toEx, f)
	if err != nil {
		return row, err
	}
	if isTeam {
		for _, n := range unplanned {
			row.Unplanned += n
		}
	} else {
		row.Unplanned = unplanned[assignee]
	}
	return row, nil
}

func (r *StatsRepo) weeklyDayMaxCompleted(from, toEx string, f model.StatsMeetingFilter) (int, string, error) {
	asDone, err := r.mapDayCountsAS(from, toEx, f, "completed")
	if err != nil {
		return 0, "", err
	}
	mntDone, err := r.mapDayCountsMnt(from, toEx, f, true)
	if err != nil {
		return 0, "", err
	}
	adminDone, err := r.mapDayCountsAdmin(from, toEx, f, true)
	if err != nil {
		return 0, "", err
	}
	addMaps(asDone, mntDone)
	addMaps(asDone, adminDone)
	maxN, maxDate := 0, ""
	for d, n := range asDone {
		if n > maxN || (n == maxN && n > 0 && (maxDate == "" || d < maxDate)) {
			maxN = n
			maxDate = d
		}
	}
	if maxN == 0 {
		return 0, "", nil
	}
	return maxN, maxDate, nil
}

func (r *StatsRepo) countWeeklyInProgress(toEx string, f model.StatsMeetingFilter) (int, error) {
	asSQL, asArgs := r.filterAS(f)
	n, err := r.countSQL(`
		SELECT COUNT(*) FROM as_receipts ar
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		WHERE ar.status IN `+model.SQLStatusOpsInProgress+`
		  AND date(ar.receipt_datetime) < date(?)
		  AND (TRIM(COALESCE(ar.complete_datetime,'')) = ''
		       OR date(ar.complete_datetime) >= date(?))`+asSQL,
		append([]interface{}{toEx, toEx}, asArgs...)...)
	if err != nil {
		return 0, err
	}
	w, err := r.countSQL(`
		SELECT COUNT(*) FROM as_work_items w
		JOIN as_receipts ar ON ar.as_id = w.as_id
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		WHERE ar.status = 'partial_complete'
		  AND COALESCE(w.status,'open') = 'open'
		  AND date(w.created_at) < date(?)`+asSQL, append([]interface{}{toEx}, asArgs...)...)
	if err != nil {
		return 0, err
	}
	n += w

	mntSQL, mntArgs := r.filterMnt(f)
	m, err := r.countSQL(`
		SELECT COUNT(*) FROM maintenance_visits v
		WHERE COALESCE(v.completed,0)=0
		  AND v.visit_date < ?
		  AND TRIM(COALESCE(v.assignee,'')) != ''`+mntSQL, append([]interface{}{toEx}, mntArgs...)...)
	if err != nil {
		return 0, err
	}
	n += m

	adminSQL, adminArgs := r.filterAdmin(f)
	dateExpr := `COALESCE(NULLIF(TRIM(t.work_date),''), NULLIF(TRIM(t.due_date),''), date(t.created_at))`
	a, err := r.countSQL(`
		SELECT COUNT(*) FROM work_tasks t
		WHERE t.work_type IN ('admin','support')
		  AND t.status = ?
		  AND `+dateExpr+` < ?`+adminSQL,
		append([]interface{}{model.WBTaskInProgress, toEx}, adminArgs...)...)
	if err != nil {
		return 0, err
	}
	return n + a, nil
}

func (r *StatsRepo) unplannedCountsByAssignee() (map[string]int, error) {
	out := map[string]int{}
	items, _, err := NewWorkBoardRepo(r.db).ListUnplanned("", nil, "")
	if err != nil {
		return out, err
	}
	for _, it := range items {
		keep := false
		for _, k := range it.Kinds {
			if k == model.UnplannedNoDate || k == model.UnplannedUnassigned {
				keep = true
				break
			}
		}
		if !keep {
			continue
		}
		name := strings.TrimSpace(it.Assignee)
		if name == "" {
			name = model.StatsUnassignedLabel
		}
		out[name]++
	}
	return out, nil
}

func (r *StatsRepo) weeklyActiveAssignees(from, toEx string) ([]string, error) {
	base := r.MetricsPolicy().BaseDate
	asBase, mntBase, adminBase := "", "", ""
	var asB, mntB, adminB []interface{}
	if base != "" {
		asBase = ` AND date(ar.receipt_datetime) >= date(?)`
		mntBase = ` AND date(v.visit_date) >= date(?)`
		adminBase = ` AND date(` + adminTaskReceiptDateSQL + `) >= date(?)`
		asB, mntB, adminB = []interface{}{base}, []interface{}{base}, []interface{}{base}
	}
	q := `
		SELECT name FROM (
			SELECT TRIM(COALESCE(ar.assigned_to,'')) AS name
			FROM as_receipts ar
			WHERE ar.status != 'cancelled'
			  AND COALESCE(ar.data_origin,'app') != 'import'` + asBase + `
			  AND (
			    (date(ar.receipt_datetime) >= date(?) AND date(ar.receipt_datetime) < date(?))
			    OR (TRIM(COALESCE(ar.complete_datetime,'')) != ''
			        AND date(ar.complete_datetime) >= date(?) AND date(ar.complete_datetime) < date(?))
			    OR (ar.status IN ` + model.SQLStatusStatsOpen + `
			        AND date(ar.receipt_datetime) < date(?)
			        AND (TRIM(COALESCE(ar.complete_datetime,'')) = ''
			             OR date(ar.complete_datetime) >= date(?)))
			  )
			UNION
			SELECT TRIM(COALESCE(v.assignee,''))
			FROM maintenance_visits v
			WHERE COALESCE(v.data_origin,'app') != 'import'` + mntBase + `
			  AND (
			    (v.visit_date >= ? AND v.visit_date < ?)
			    OR (COALESCE(v.completed,0)=1
			        AND COALESCE(NULLIF(TRIM(v.completed_date),''), v.visit_date) >= ?
			        AND COALESCE(NULLIF(TRIM(v.completed_date),''), v.visit_date) < ?)
			    OR (COALESCE(v.completed,0)=0 AND v.visit_date < ?)
			  )
			UNION
			SELECT TRIM(COALESCE(t.assignee,''))
			FROM work_tasks t
			WHERE t.work_type IN ('admin','support')
			  AND TRIM(COALESCE(t.source_type,'')) NOT IN ('as','maintenance')` + SQLRecurrenceWorkUnit + adminBase + `
			  AND (
			    (` + adminTaskReceiptDateSQL + ` >= ? AND ` + adminTaskReceiptDateSQL + ` < ?)
			    OR (t.status='complete' AND ` + adminTaskCompleteDateSQL + ` >= ? AND ` + adminTaskCompleteDateSQL + ` < ?)
			    OR (COALESCE(t.status,'') != 'complete'
			        AND COALESCE(NULLIF(TRIM(t.work_date),''), NULLIF(TRIM(t.due_date),''), date(t.created_at)) < ?)
			  )
		) WHERE 1=1`
	args := []interface{}{}
	args = append(args, asB...)
	args = append(args, from, toEx, from, toEx, toEx, toEx)
	args = append(args, mntB...)
	args = append(args, from, toEx, from, toEx, toEx)
	args = append(args, adminB...)
	args = append(args, from, toEx, from, toEx, toEx)
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	seen := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		n = strings.TrimSpace(n)
		if n == "" {
			n = model.StatsUnassignedLabel
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		names = append(names, n)
	}
	return names, rows.Err()
}

// CountWorkingDays 기간 [from, toEx)의 워킹데이(평일 − holidays). 해당 연도 행이 없으면 주말만 제외.
func (r *StatsRepo) CountWorkingDays(from, toEx string) (int, bool, error) {
	start, err := time.ParseInLocation("2006-01-02", from, time.Local)
	if err != nil {
		return 0, false, err
	}
	endEx, err := time.ParseInLocation("2006-01-02", toEx, time.Local)
	if err != nil {
		return 0, false, err
	}
	years := map[int]bool{}
	for d := start; d.Before(endEx); d = d.AddDate(0, 0, 1) {
		years[d.Year()] = true
	}
	yearMissing := false
	for y := range years {
		var n int
		if err := r.db.QueryRow(`SELECT COUNT(*) FROM holidays WHERE holiday_year=?`, y).Scan(&n); err != nil {
			return 0, false, err
		}
		if n == 0 {
			yearMissing = true
		}
	}
	holi := map[string]bool{}
	hrows, err := r.db.Query(`SELECT holiday_date FROM holidays WHERE holiday_date >= ? AND holiday_date < ?`, from, toEx)
	if err != nil {
		return 0, false, err
	}
	defer hrows.Close()
	for hrows.Next() {
		var d string
		if err := hrows.Scan(&d); err != nil {
			return 0, false, err
		}
		d = strings.TrimSpace(d)
		if len(d) >= 10 {
			d = d[:10]
		}
		if d != "" {
			holi[d] = true
		}
	}
	if err := hrows.Err(); err != nil {
		return 0, false, err
	}
	days := 0
	for d := start; d.Before(endEx); d = d.AddDate(0, 0, 1) {
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			continue
		}
		if holi[d.Format("2006-01-02")] {
			continue
		}
		days++
	}
	return days, yearMissing, nil
}

type weeklyKPI struct {
	visitAS, completeAS, completeMnt, completeAdmin, completeAll      float64
	nVisitAS, nCompleteAS, nCompleteMnt, nCompleteAdmin, nCompleteAll int
}

func (r *StatsRepo) avgKPIForFilter(from, toEx string, f model.StatsMeetingFilter) (weeklyKPI, error) {
	var k weeklyKPI
	var err error
	k.visitAS, k.nVisitAS, err = r.avgASDays(from, toEx, f, "visit")
	if err != nil {
		return k, err
	}
	k.completeAS, k.nCompleteAS, err = r.avgASDays(from, toEx, f, "complete")
	if err != nil {
		return k, err
	}
	k.completeMnt, k.nCompleteMnt, err = r.avgMntCompleteDays(from, toEx, f)
	if err != nil {
		return k, err
	}
	k.completeAdmin, k.nCompleteAdmin, err = r.avgAdminCompleteDays(from, toEx, f)
	if err != nil {
		return k, err
	}
	sum, n := 0.0, 0
	add := func(avg float64, cnt int) {
		if cnt > 0 {
			sum += avg * float64(cnt)
			n += cnt
		}
	}
	add(k.completeAS, k.nCompleteAS)
	add(k.completeMnt, k.nCompleteMnt)
	add(k.completeAdmin, k.nCompleteAdmin)
	k.nCompleteAll = n
	if n > 0 {
		k.completeAll = sum / float64(n)
	}
	return k, nil
}

func (r *StatsRepo) avgMntCompleteDays(from, toEx string, f model.StatsMeetingFilter) (float64, int, error) {
	mntSQL, args := r.filterMnt(f)
	q := `
		SELECT AVG(julianday(COALESCE(NULLIF(TRIM(v.completed_date),''), v.visit_date)) - julianday(v.visit_date)),
		       COUNT(*)
		FROM maintenance_visits v
		WHERE COALESCE(v.completed,0)=1
		  AND COALESCE(NULLIF(TRIM(v.completed_date),''), v.visit_date) >= ?
		  AND COALESCE(NULLIF(TRIM(v.completed_date),''), v.visit_date) < ?` + mntSQL
	return r.scanAvgCount(q, append([]interface{}{from, toEx}, args...)...)
}

func (r *StatsRepo) avgAdminCompleteDays(from, toEx string, f model.StatsMeetingFilter) (float64, int, error) {
	adminSQL, args := r.filterAdmin(f)
	q := `
		SELECT AVG(julianday(` + adminTaskCompleteDateSQL + `) - julianday(` + adminTaskReceiptDateSQL + `)),
		       COUNT(*)
		FROM work_tasks t
		WHERE t.work_type IN ('admin','support') AND t.status='complete'
		  AND ` + adminTaskCompleteDateSQL + ` >= ? AND ` + adminTaskCompleteDateSQL + ` < ?` + adminSQL
	return r.scanAvgCount(q, append([]interface{}{from, toEx}, args...)...)
}

func (r *StatsRepo) scanAvgCount(q string, args ...interface{}) (avg float64, n int, err error) {
	var avgNull interface{}
	err = r.db.QueryRow(q, args...).Scan(&avgNull, &n)
	if err != nil {
		return 0, 0, err
	}
	if n == 0 || avgNull == nil {
		return 0, 0, nil
	}
	switch v := avgNull.(type) {
	case float64:
		avg = v
	case []byte:
		fmt.Sscanf(string(v), "%f", &avg)
	case string:
		fmt.Sscanf(v, "%f", &avg)
	}
	if avg < 0 {
		avg = 0
	}
	return avg, n, nil
}

func mathRound1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}

func statsResultLabel(resultCode, status string) string {
	switch strings.TrimSpace(resultCode) {
	case "done":
		return "완료"
	case "partial":
		return "부분완료"
	case "temporary":
		return "임시조치"
	case "transfer":
		return "타사이관"
	case "revisit_needed":
		return "재방문필요"
	case "escalation":
		return "제조사에스컬레이션"
	}
	if status != "" {
		return statsStatusLabel(status)
	}
	return ""
}

func weeklyAssigneeLabel(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return model.StatsUnassignedLabel
	}
	return s
}

func weeklyProduct(category, name, modelName string) string {
	parts := []string{}
	for _, p := range []string{strings.TrimSpace(category), strings.TrimSpace(name), strings.TrimSpace(modelName)} {
		if p != "" && (len(parts) == 0 || parts[len(parts)-1] != p) {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " · ")
}

// ListWeeklyEventRows 그 주에 접수·조치·완료·방문이 있는 건. 이관 제외. 발생일·업무번호 순.
func (r *StatsRepo) ListWeeklyEventRows(from, toEx string) ([]model.WeeklyEventRow, error) {
	var out []model.WeeklyEventRow
	as, err := r.listWeeklyASEvents(from, toEx)
	if err != nil {
		return nil, err
	}
	out = append(out, as...)
	mnt, err := r.listWeeklyMntEvents(from, toEx)
	if err != nil {
		return nil, err
	}
	out = append(out, mnt...)
	admin, err := r.listWeeklyAdminEvents(from, toEx)
	if err != nil {
		return nil, err
	}
	out = append(out, admin...)
	waiting, err := r.listWeeklyWaitingForEvents(from, toEx)
	if err != nil {
		return nil, err
	}
	seenAdmin := map[string]bool{}
	for _, e := range out {
		if e.Kind == model.WeeklyEventAdmin && strings.TrimSpace(e.WorkNo) != "" {
			seenAdmin[e.WorkNo] = true
		}
	}
	for _, e := range waiting {
		if seenAdmin[e.WorkNo] {
			continue
		}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].OccurDate != out[j].OccurDate {
			return out[i].OccurDate < out[j].OccurDate
		}
		if out[i].WorkNo != out[j].WorkNo {
			return out[i].WorkNo < out[j].WorkNo
		}
		return weeklyEventKindOrder(out[i].Kind) < weeklyEventKindOrder(out[j].Kind)
	})
	return out, nil
}

func weeklyEventKindOrder(kind string) int {
	switch kind {
	case model.WeeklyEventReceipt:
		return 0
	case model.WeeklyEventAction:
		return 1
	case model.WeeklyEventComplete:
		return 2
	case model.WeeklyEventMnt:
		return 3
	default:
		return 4
	}
}

func (r *StatsRepo) listWeeklyASEvents(from, toEx string) ([]model.WeeklyEventRow, error) {
	var out []model.WeeklyEventRow
	asSQL, asArgs := r.filterAS(model.StatsMeetingFilter{})
	base := `
		SELECT COALESCE(ar.as_number,''), COALESCE(c.org_name,''),
		       COALESCE(a.product_category,''), COALESCE(a.product_name,''), COALESCE(a.model_name,''),
		       COALESCE(NULLIF(TRIM(ar.assigned_to),''), ''),
		       COALESCE(ar.symptom,''), COALESCE(ar.action_taken,''),
		       COALESCE(ar.result_code,''), COALESCE(ar.status,''),
		       COALESCE(date(%s),'')
		FROM as_receipts ar
		JOIN customers c ON c.customer_id = ar.customer_id
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		WHERE ar.status != 'cancelled'` + asSQL + `
		  AND date(%s) >= date(?) AND date(%s) < date(?)`

	add := func(kind, dateExpr, contentExpr string, args ...interface{}) error {
		q := fmt.Sprintf(base, dateExpr, dateExpr, dateExpr)
		rows, err := r.db.Query(q, append(append([]interface{}{}, asArgs...), args...)...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var it model.WeeklyEventRow
			var cat, pname, mname, symptom, action, resultCode, status string
			if err := rows.Scan(&it.WorkNo, &it.Customer, &cat, &pname, &mname, &it.Assignee,
				&symptom, &action, &resultCode, &status, &it.OccurDate); err != nil {
				return err
			}
			it.Kind = kind
			it.WorkType = "AS"
			it.Assignee = weeklyAssigneeLabel(it.Assignee)
			it.Product = weeklyProduct(cat, pname, mname)
			it.Result = statsResultLabel(resultCode, status)
			if contentExpr == "action" {
				it.Content = strings.TrimSpace(action)
				if it.Content == "" {
					it.Content = symptom
				}
			} else {
				it.Content = symptom
			}
			out = append(out, it)
		}
		return rows.Err()
	}
	if err := add(model.WeeklyEventReceipt, "ar.receipt_datetime", "symptom", from, toEx); err != nil {
		return nil, err
	}
	if err := add(model.WeeklyEventComplete, "ar.complete_datetime", "action", from, toEx); err != nil {
		return nil, err
	}

	prows, err := r.db.Query(`
		SELECT COALESCE(ar.as_number,''), COALESCE(c.org_name,''),
		       COALESCE(a.product_category,''), COALESCE(a.product_name,''), COALESCE(a.model_name,''),
		       COALESCE(NULLIF(TRIM(p.worker),''), NULLIF(TRIM(ar.assigned_to),''), ''),
		       COALESCE(NULLIF(TRIM(p.work_content),''), ar.action_taken, ar.symptom, ''),
		       COALESCE(ar.result_code,''), COALESCE(ar.status,''),
		       COALESCE(date(p.process_datetime),''), COALESCE(p.time_spent,0)
		FROM as_processes p
		JOIN as_receipts ar ON ar.as_id = p.as_id
		JOIN customers c ON c.customer_id = ar.customer_id
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		WHERE ar.status != 'cancelled'` + asSQL + `
		  AND p.process_datetime IS NOT NULL
		  AND date(p.process_datetime) >= date(?) AND date(p.process_datetime) < date(?)`, append(append([]interface{}{}, asArgs...), from, toEx)...)
	if err != nil {
		return nil, err
	}
	defer prows.Close()
	for prows.Next() {
		var it model.WeeklyEventRow
		var cat, pname, mname, resultCode, status string
		if err := prows.Scan(&it.WorkNo, &it.Customer, &cat, &pname, &mname, &it.Assignee,
			&it.Content, &resultCode, &status, &it.OccurDate, &it.Minutes); err != nil {
			return nil, err
		}
		it.Kind = model.WeeklyEventAction
		it.WorkType = "AS"
		it.Assignee = weeklyAssigneeLabel(it.Assignee)
		it.Product = weeklyProduct(cat, pname, mname)
		it.Result = statsResultLabel(resultCode, status)
		out = append(out, it)
	}
	return out, prows.Err()
}

func (r *StatsRepo) listWeeklyMntEvents(from, toEx string) ([]model.WeeklyEventRow, error) {
	doneDate := `COALESCE(NULLIF(TRIM(v.completed_date),''), CASE WHEN COALESCE(v.completed,0)=1 THEN v.visit_date ELSE '' END)`
	mntSQL, mntArgs := r.filterMnt(model.StatsMeetingFilter{})
	q := `
		SELECT COALESCE(v.visit_id,''), COALESCE(c.org_name,''), COALESCE(v.product_type,''),
		       COALESCE(NULLIF(TRIM(v.assignee),''), ''),
		       COALESCE(v.notes,''),
		       CASE WHEN COALESCE(v.completed,0)=1 THEN 'completed' ELSE 'in_progress' END,
		       CASE
		         WHEN TRIM(COALESCE(` + doneDate + `,'')) != ''
		              AND ` + doneDate + ` >= ? AND ` + doneDate + ` < ? THEN ` + doneDate + `
		         ELSE v.visit_date
		       END,
		       CASE WHEN COALESCE(v.completed,0)=1 THEN 1 ELSE 0 END
		FROM maintenance_visits v
		JOIN customers c ON c.customer_id = v.customer_id
		WHERE (` + `
		    (v.visit_date >= ? AND v.visit_date < ?)
		    OR (TRIM(COALESCE(v.completed_date,'')) != '' AND v.completed_date >= ? AND v.completed_date < ?)
		  )` + mntSQL
	rows, err := r.db.Query(q, append([]interface{}{from, toEx, from, toEx, from, toEx}, mntArgs...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []model.WeeklyEventRow
	for rows.Next() {
		var it model.WeeklyEventRow
		var status string
		var done int
		if err := rows.Scan(&it.WorkNo, &it.Customer, &it.Product, &it.Assignee, &it.Content, &status, &it.OccurDate, &done); err != nil {
			return nil, err
		}
		it.Kind = model.WeeklyEventMnt
		it.WorkType = "정기점검"
		it.Assignee = weeklyAssigneeLabel(it.Assignee)
		it.Result = statsStatusLabel(status)
		if strings.TrimSpace(it.Content) == "" {
			it.Content = strings.TrimSpace(it.Product) + " 정기점검"
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

func (r *StatsRepo) listWeeklyAdminEvents(from, toEx string) ([]model.WeeklyEventRow, error) {
	adminSQL, adminArgs := r.filterAdmin(model.StatsMeetingFilter{})
	q := `
		SELECT COALESCE(t.task_id,''),
		       COALESCE(NULLIF(TRIM(c.org_name),''), NULLIF(TRIM(t.customer_name),''), ''),
		       COALESCE(NULLIF(TRIM(t.assignee),''), ''),
		       COALESCE(t.title,''), COALESCE(t.description,''), COALESCE(t.status,''), COALESCE(t.work_type,''),
		       CASE
		         WHEN t.status='complete' AND ` + adminTaskCompleteDateSQL + ` >= ? AND ` + adminTaskCompleteDateSQL + ` < ?
		           THEN ` + adminTaskCompleteDateSQL + `
		         WHEN ` + adminTaskReceiptDateSQL + ` >= ? AND ` + adminTaskReceiptDateSQL + ` < ?
		           THEN ` + adminTaskReceiptDateSQL + `
		         ELSE COALESCE(NULLIF(TRIM(t.work_date),''), NULLIF(TRIM(t.due_date),''), date(t.created_at))
		       END,
		       COALESCE((SELECT SUM(COALESCE(a.spent_minutes,0)) FROM work_activities a WHERE a.task_id=t.task_id), t.duration_min, 0),
		       COALESCE(t.wait_party,''), COALESCE(t.reply_due_date,''),
		       COALESCE(t.parent_task_id,''), COALESCE(t.recurrence_role,'')
		FROM work_tasks t
		LEFT JOIN customers c ON c.customer_id = t.customer_id
		WHERE t.work_type IN ('admin','support')
		  AND TRIM(COALESCE(t.source_type,'')) NOT IN ('as','maintenance')` + SQLRecurrenceWorkUnit + adminSQL + `
		  AND (
		    (` + adminTaskReceiptDateSQL + ` >= ? AND ` + adminTaskReceiptDateSQL + ` < ?)
		    OR (t.status='complete' AND ` + adminTaskCompleteDateSQL + ` >= ? AND ` + adminTaskCompleteDateSQL + ` < ?)
		    OR (COALESCE(NULLIF(TRIM(t.work_date),''), '') >= ? AND COALESCE(NULLIF(TRIM(t.work_date),''), '') < ?)
		  )`
	args := []interface{}{from, toEx, from, toEx}
	args = append(args, adminArgs...)
	args = append(args, from, toEx, from, toEx, from, toEx)
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []model.WeeklyEventRow
	for rows.Next() {
		var it model.WeeklyEventRow
		var desc, status, wtype, waitParty, replyDue string
		if err := rows.Scan(&it.WorkNo, &it.Customer, &it.Assignee, &it.Content, &desc, &status, &wtype, &it.OccurDate, &it.Minutes, &waitParty, &replyDue, &it.ParentTaskID, &it.RecurrenceRole); err != nil {
			return nil, err
		}
		it.Kind = model.WeeklyEventAdmin
		it.WorkType = "행정"
		if wtype == "support" {
			it.WorkType = "지원"
		}
		it.Assignee = weeklyAssigneeLabel(it.Assignee)
		it.Result = model.WBTaskStatusLabel(status)
		if status == model.WBTaskWaitingFor {
			it.Content = weeklyWaitingContent(it.Content, waitParty, replyDue)
		}
		if strings.TrimSpace(desc) != "" && !strings.Contains(it.Content, desc) {
			// 내용은 업무명, 설명은 보조 — 시트 「내용」은 업무명
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// listWeeklyWaitingForEvents 「회신 대기」 건. v2.4는 미계획건 시트가 없으므로 전체 리스트에 대기 대상·회신 예정일을 싣는다(§13.10).
func (r *StatsRepo) listWeeklyWaitingForEvents(from, toEx string) ([]model.WeeklyEventRow, error) {
	q := `
		SELECT COALESCE(t.task_id,''),
		       COALESCE(NULLIF(TRIM(c.org_name),''), NULLIF(TRIM(t.customer_name),''), ''),
		       COALESCE(NULLIF(TRIM(t.assignee),''), ''),
		       COALESCE(t.title,''), COALESCE(t.wait_party,''), COALESCE(t.reply_due_date,''),
		       COALESCE(t.status,''), COALESCE(t.work_type,''),
		       COALESCE(NULLIF(TRIM(t.reply_due_date),''), NULLIF(TRIM(t.next_check_date),''), ` + adminTaskReceiptDateSQL + `)
		FROM work_tasks t
		LEFT JOIN customers c ON c.customer_id = t.customer_id
		WHERE t.work_type IN ('admin','support')
		  AND t.status = ?
		  AND TRIM(COALESCE(t.source_type,'')) NOT IN ('as','maintenance')` + SQLRecurrenceWorkUnit + `
		  AND (
		    (TRIM(COALESCE(t.reply_due_date,'')) != '' AND t.reply_due_date >= ? AND t.reply_due_date < ?)
		    OR (TRIM(COALESCE(t.next_check_date,'')) != '' AND t.next_check_date >= ? AND t.next_check_date < ?)
		  )`
	rows, err := r.db.Query(q, model.WBTaskWaitingFor, from, toEx, from, toEx)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") || strings.Contains(err.Error(), "no such column") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var items []model.WeeklyEventRow
	seen := map[string]bool{}
	for rows.Next() {
		var it model.WeeklyEventRow
		var waitParty, replyDue, status, wtype string
		if err := rows.Scan(&it.WorkNo, &it.Customer, &it.Assignee, &it.Content, &waitParty, &replyDue, &status, &wtype, &it.OccurDate); err != nil {
			return nil, err
		}
		seen[it.WorkNo] = true
		it.Kind = model.WeeklyEventAdmin
		it.WorkType = "행정"
		if wtype == "support" {
			it.WorkType = "지원"
		}
		it.Assignee = weeklyAssigneeLabel(it.Assignee)
		it.Result = model.WBTaskStatusLabel(status)
		it.Content = weeklyWaitingContent(it.Content, waitParty, replyDue)
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return items, err
	}

	aq := `
		SELECT COALESCE(t.task_id,''),
		       COALESCE(NULLIF(TRIM(c.org_name),''), NULLIF(TRIM(t.customer_name),''), ''),
		       COALESCE(NULLIF(TRIM(a.assignee),''), NULLIF(TRIM(t.assignee),''), ''),
		       COALESCE(NULLIF(TRIM(a.title),''), COALESCE(t.title,'')),
		       COALESCE(a.wait_party,''), COALESCE(a.reply_due_date,''),
		       COALESCE(t.work_type,''),
		       COALESCE(NULLIF(TRIM(a.reply_due_date),''), NULLIF(TRIM(a.next_check_date),''), '')
		FROM work_actions a
		JOIN work_tasks t ON t.task_id = a.task_id
		LEFT JOIN customers c ON c.customer_id = t.customer_id
		WHERE t.work_type IN ('admin','support')
		  AND TRIM(COALESCE(t.source_type,'')) NOT IN ('as','maintenance')
		  AND t.status != ?
		  AND a.status = ?
		  AND COALESCE(a.confirmed,0)=0
		  AND (
		    (TRIM(COALESCE(a.reply_due_date,'')) != '' AND a.reply_due_date >= ? AND a.reply_due_date < ?)
		    OR (TRIM(COALESCE(a.next_check_date,'')) != '' AND a.next_check_date >= ? AND a.next_check_date < ?)
		  )`
	arows, err := r.db.Query(aq, model.WBTaskWaitingFor, model.WBActionWaiting, from, toEx, from, toEx)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") || strings.Contains(err.Error(), "no such column") {
			return items, nil
		}
		return items, err
	}
	defer arows.Close()
	for arows.Next() {
		var it model.WeeklyEventRow
		var waitParty, replyDue, wtype string
		if err := arows.Scan(&it.WorkNo, &it.Customer, &it.Assignee, &it.Content, &waitParty, &replyDue, &wtype, &it.OccurDate); err != nil {
			return items, err
		}
		if seen[it.WorkNo] {
			continue
		}
		it.Kind = model.WeeklyEventAdmin
		it.WorkType = "행정"
		if wtype == "support" {
			it.WorkType = "지원"
		}
		it.Assignee = weeklyAssigneeLabel(it.Assignee)
		it.Result = model.WBTaskStatusLabel(model.WBTaskWaitingFor)
		it.Content = weeklyWaitingContent(it.Content, waitParty, replyDue)
		items = append(items, it)
	}
	return items, arows.Err()
}

func weeklyWaitingContent(title, waitParty, replyDue string) string {
	title = strings.TrimSpace(title)
	var extra []string
	if p := strings.TrimSpace(waitParty); p != "" {
		extra = append(extra, "대기 대상 "+p)
	}
	if d := strings.TrimSpace(replyDue); d != "" {
		extra = append(extra, "회신예정 "+d)
	}
	if len(extra) == 0 {
		return title
	}
	joined := strings.Join(extra, " · ")
	if title == "" {
		return joined
	}
	return title + " · " + joined
}

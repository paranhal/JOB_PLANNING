package repository

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"customer-support/internal/model"
)

const mntDoneDateSQL = `COALESCE(NULLIF(TRIM(v.completed_date),''), v.visit_date)`

// CompanyWeeklyPeriod 기준일(월요일) → 전주·금주·시트 이름 기본값. §16.6.3
func CompanyWeeklyPeriod(anchor time.Time) model.CompanyWeeklyPeriod {
	loc := anchor.Location()
	y, m, d := anchor.Date()
	day := time.Date(y, m, d, 0, 0, 0, 0, loc)
	wd := int(day.Weekday())
	if wd == 0 {
		wd = 7
	}
	thisMon := day.AddDate(0, 0, -(wd - 1))
	prevMon := thisMon.AddDate(0, 0, -7)
	prevSun := prevMon.AddDate(0, 0, 6)
	thisSun := thisMon.AddDate(0, 0, 6)
	weekN := nthMondayOfMonth(prevMon)
	year := prevMon.Year()
	month := int(prevMon.Month())
	return model.CompanyWeeklyPeriod{
		Anchor:           thisMon.Format("2006-01-02"),
		PrevFrom:         prevMon.Format("2006-01-02"),
		PrevTo:           prevSun.Format("2006-01-02"),
		PrevToEx:         thisMon.Format("2006-01-02"),
		ThisFrom:         thisMon.Format("2006-01-02"),
		ThisTo:           thisSun.Format("2006-01-02"),
		ThisToEx:         thisMon.AddDate(0, 0, 7).Format("2006-01-02"),
		SheetNameDefault: fmt.Sprintf("%d.%d월%d주(업무)", year%100, month, weekN),
		TitleE1:          fmt.Sprintf("'%d년 %d월 %d주차 주간업무보고", year%100, month, weekN),
		PrevRangeLabel:   companyWeekRangeLabel(prevMon, prevSun),
		ThisRangeLabel:   companyWeekRangeLabel(thisMon, thisSun),
		Year:             year,
		Month:            month,
		WeekN:            weekN,
	}
}

func companyWeekRangeLabel(mon, sun time.Time) string {
	return fmt.Sprintf("%d.%d(월)~%d.%d(일)", int(mon.Month()), mon.Day(), int(sun.Month()), sun.Day())
}

// WeekInfoOf 그 주 월요일 기준 목요일 달·주차. 화면·새 엑셀이 이 함수만 쓴다.
func WeekInfoOf(monday time.Time) model.WeekInfo {
	loc := monday.Location()
	y, m, d := monday.Date()
	mon := time.Date(y, m, d, 0, 0, 0, 0, loc)
	wd := int(mon.Weekday())
	if wd == 0 {
		wd = 7
	}
	mon = mon.AddDate(0, 0, -(wd - 1))
	thu := mon.AddDate(0, 0, 3)
	n := (thu.Day() + 6) / 7
	return model.WeekInfo{
		Monday: mon,
		Sunday: mon.AddDate(0, 0, 6),
		Year:   thu.Year(),
		Month:  int(thu.Month()),
		WeekN:  n,
	}
}

// WeeklyPlanBounds 보고 주 W(월요일) 기준 이월·차주 계획 구간.
type WeeklyPlanBounds struct {
	WeekFrom string // W 월요일
	WeekToEx string // W+7
	PlanFrom string // W+7 (차주)
	PlanToEx string // W+14
}

func WeeklyPlanWindow(contentMonday time.Time) WeeklyPlanBounds {
	p := CompanyWeeklyPeriod(contentMonday.AddDate(0, 0, 7))
	return WeeklyPlanBounds{
		WeekFrom: p.PrevFrom, WeekToEx: p.PrevToEx,
		PlanFrom: p.ThisFrom, PlanToEx: p.ThisToEx,
	}
}

func nthMondayOfMonth(mon time.Time) int {
	n := 0
	for d := 1; d <= mon.Day(); d++ {
		t := time.Date(mon.Year(), mon.Month(), d, 0, 0, 0, 0, mon.Location())
		if t.Weekday() == time.Monday {
			n++
		}
	}
	return n
}

// BuildCompanyWeeklyDraft 전사 주간업무보고 20~25행 F·G 초안. §16.6.5~7
func (r *StatsRepo) BuildCompanyWeeklyDraft(anchor time.Time) (model.CompanyWeeklyDraft, error) {
	return r.BuildCompanyWeeklyDraftOrg(anchor, OrgAll)
}

func (r *StatsRepo) BuildCompanyWeeklyDraftOrg(anchor time.Time, orgID string) (model.CompanyWeeklyDraft, error) {
	orgID, err := reportOrgID(orgID)
	if err != nil {
		return model.CompanyWeeklyDraft{}, err
	}
	p := CompanyWeeklyPeriod(anchor)
	mapRows, err := r.ListWeeklyReportRowsOrg(orgID, true)
	if err != nil {
		return model.CompanyWeeklyDraft{}, err
	}
	mapped := map[string]bool{}
	for _, row := range mapRows {
		if id := strings.TrimSpace(row.ProjectID); id != "" {
			mapped[id] = true
		}
	}

	win := WeeklyPlanWindow(mustParseDay(p.PrevFrom, anchor.Location()))
	incomplete, err := r.listIncompleteMntVisits(win.WeekFrom, win.WeekToEx, orgID)
	if err != nil {
		return model.CompanyWeeklyDraft{}, err
	}

	out := model.CompanyWeeklyDraft{Period: p, IncompleteMnt: incomplete}
	for i, row := range mapRows {
		item := model.CompanyWeeklyRow{
			RowKey:       row.Key,
			SheetRow:     20 + i,
			Division:     row.Division,
			Team:         row.Team,
			NoLabel:      row.NoLabel,
			DisplayName:  row.DisplayName,
			ProjectID:    row.ProjectID,
			RowKind:      row.Kind,
			ActivityKind: row.ActivityKind,
			Highlight:    row.Highlight,
			IsActive:     row.Active,
		}
		if row.Kind == "project" && strings.TrimSpace(row.ProjectID) == "" {
			item.MissingProject = true
		}
		if row.Kind == "activity" {
			item.PrevText, item.PlanText, item.AutoDone, item.AutoPlan, err = r.composeActivityTexts(row, p, mapped, orgID)
			if err != nil {
				return out, err
			}
		} else if pid := strings.TrimSpace(row.ProjectID); pid != "" {
			item.MntDoneSites, err = r.countCompanyMntSitesDone(pid, win.WeekFrom, win.WeekToEx, orgID)
			if err != nil {
				return out, err
			}
			item.ASDoneCount, err = r.countCompanyASDone(pid, win.WeekFrom, win.WeekToEx, orgID)
			if err != nil {
				return out, err
			}
			item.MntPlanSites, err = r.countCompanyMntSitesPlanned(pid, win.PlanFrom, win.PlanToEx, orgID)
			if err != nil {
				return out, err
			}
			doneNames, doneCust, err := r.listCompanyMntSiteNames(pid, win.WeekFrom, win.WeekToEx, orgID, false)
			if err != nil {
				return out, err
			}
			planNames, planCust, err := r.listCompanyMntSiteNames(pid, win.PlanFrom, win.PlanToEx, orgID, true)
			if err != nil {
				return out, err
			}
			adminDone, err := r.listCompanyAdminDone(pid, win.WeekFrom, win.WeekToEx, orgID)
			if err != nil {
				return out, err
			}
			adminPlan, err := r.listCompanyAdminPlan(pid, win.PlanFrom, win.PlanToEx, orgID)
			if err != nil {
				return out, err
			}
			asFollow, err := r.listCompanyASNextActionPlan(pid, win.PlanFrom, win.PlanToEx, orgID)
			if err != nil {
				return out, err
			}
			adminPlan = append(adminPlan, asFollow...)
			carry, err := r.listCompanyAdminCarry(pid, win.WeekFrom, win.PlanFrom, orgID)
			if err != nil {
				return out, err
			}
			openAS, err := r.listCompanyASOpen(pid, orgID)
			if err != nil {
				return out, err
			}
			mntCarry := incompleteMntCarry(incomplete, pid)
			mntCust := mergeCustSet(doneCust, planCust)
			item.PrevText, item.AutoDone = composePrevBundle(item.MntDoneSites, doneNames, item.ASDoneCount, openAS, dropDupMntAdmin(adminDone, doneCust), pid)
			item.PlanText, item.AutoPlan = composePlanBundle(item.MntPlanSites, planNames, dropDupMntAdmin(adminPlan, mntCust), dropDupMntAdmin(carry, mntCust), mntCarry, pid)
		}
		out.Rows = append(out.Rows, item)
	}

	unassigned, err := r.listCompanyUnassigned(p, mapped, orgID)
	if err != nil {
		return out, err
	}
	if hasMntAsActivity(mapRows) {
		unassigned = dropUnassignedKinds(unassigned, model.ScopeWorkMaintenance, model.ScopeWorkAS)
	}
	out.Unassigned = unassigned
	return out, nil
}

func mustParseDay(iso string, loc *time.Location) time.Time {
	t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(iso), loc)
	if err != nil {
		return time.Now().In(loc)
	}
	return t
}

func incompleteMntCarry(visits []model.IncompleteMntVisit, projectID string) []adminLine {
	var out []adminLine
	for _, v := range visits {
		if strings.TrimSpace(v.ProjectID) != projectID {
			continue
		}
		title := strings.TrimSpace(v.OrgName)
		if title == "" {
			title = v.VisitID
		}
		out = append(out, adminLine{ID: v.VisitID, Title: "정기점검 " + title, Date: v.VisitDate, ProjectID: projectID, CustomerID: "", WorkType: "mnt"})
	}
	return out
}

func hasMntAsActivity(rows []weeklyReportRow) bool {
	for _, row := range rows {
		if row.Kind == "activity" && row.ActivityKind == "mnt_as" {
			return true
		}
	}
	return false
}

func dropUnassignedKinds(in []model.CompanyWeeklyUnassigned, kinds ...string) []model.CompanyWeeklyUnassigned {
	skip := map[string]bool{}
	for _, k := range kinds {
		skip[k] = true
	}
	var out []model.CompanyWeeklyUnassigned
	for _, row := range in {
		if skip[row.Kind] {
			continue
		}
		out = append(out, row)
	}
	return out
}

func composePrevText(mntSites int, names []string, asCount int, admin []adminLine) string {
	text, _ := composePrevBundle(mntSites, names, asCount, asOpenSummary{}, admin, "")
	return text
}

func composePlanText(mntSites int, names []string, plan, carry []adminLine) string {
	text, _ := composePlanBundle(mntSites, names, plan, carry, nil, "")
	return text
}

func composePrevBundle(mntSites int, names []string, asCount int, open asOpenSummary, admin []adminLine, projectID string) (string, []model.CompanyWeeklyAutoLine) {
	var auto []model.CompanyWeeklyAutoLine
	core := formatMntSitesCore(mntSites, names)
	if core != "" && asCount > 0 {
		auto = append(auto, model.CompanyWeeklyAutoLine{Key: "agg:done:" + projectID, Text: core + fmt.Sprintf(", AS %d건 처리완료", asCount)})
	} else if core != "" {
		auto = append(auto, model.CompanyWeeklyAutoLine{Key: "agg:done:" + projectID, Text: core})
	} else if asCount > 0 {
		auto = append(auto, model.CompanyWeeklyAutoLine{Key: "agg:done:" + projectID, Text: fmt.Sprintf("AS %d건 처리완료", asCount)})
	}
	if open.N > 0 {
		auto = append(auto, model.CompanyWeeklyAutoLine{Key: "as:open:" + projectID, Text: formatASOpenLine(open)})
	}
	for _, a := range admin {
		text := a.Title
		if strings.TrimSpace(a.Date) != "" {
			text = a.Title + "(" + slashMD(a.Date) + ")"
		}
		auto = append(auto, model.CompanyWeeklyAutoLine{Key: lineKey("task", a.ID), Text: text})
	}
	return joinAutoLines(auto), auto
}

func composePlanBundle(mntSites int, names []string, plan, carry, mntCarry []adminLine, projectID string) (string, []model.CompanyWeeklyAutoLine) {
	var auto []model.CompanyWeeklyAutoLine
	if core := formatMntSitesCore(mntSites, names); core != "" {
		auto = append(auto, model.CompanyWeeklyAutoLine{Key: "agg:plan:" + projectID, Text: core})
	}
	for _, a := range plan {
		kind := "task"
		if a.WorkType == "as" {
			kind = "as"
		}
		auto = append(auto, model.CompanyWeeklyAutoLine{Key: lineKey(kind, a.ID), Text: a.Title})
	}
	for _, a := range carry {
		auto = append(auto, model.CompanyWeeklyAutoLine{Key: lineKey("task", a.ID), Text: carryTitle(a.Title)})
	}
	for _, a := range mntCarry {
		auto = append(auto, model.CompanyWeeklyAutoLine{Key: lineKey("mnt", a.ID), Text: carryTitle(a.Title)})
	}
	return joinAutoLines(auto), auto
}

func joinAutoLines(auto []model.CompanyWeeklyAutoLine) string {
	var lines []string
	for _, a := range auto {
		t := strings.TrimSpace(a.Text)
		if t == "" {
			continue
		}
		if !strings.HasPrefix(t, "·") {
			t = "·" + t
		}
		lines = append(lines, t)
	}
	return strings.Join(lines, "\n")
}

func carryTitle(title string) string {
	title = strings.TrimSpace(title)
	if strings.Contains(title, "이월") || strings.Contains(title, "지연") {
		return title
	}
	return title + "(이월)"
}

func lineKey(kind, id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		id = "_"
	}
	return kind + ":" + id
}

func formatASOpenLine(open asOpenSummary) string {
	core := fmt.Sprintf("AS 진행 %d건", open.N)
	if strings.TrimSpace(open.Detail) == "" {
		return core
	}
	return core + "(" + open.Detail + ")"
}

func formatMntSitesCore(n int, names []string) string {
	if n <= 0 {
		return ""
	}
	names = uniqSortedNames(names)
	core := fmt.Sprintf("정기점검 %d사이트", n)
	if len(names) == 0 {
		return core
	}
	shown := names
	extra := 0
	if len(shown) > 5 {
		extra = len(shown) - 5
		shown = shown[:5]
	}
	core += "(" + strings.Join(shown, "·")
	if extra > 0 {
		core += fmt.Sprintf(" 외 %d곳", extra)
	}
	core += ")"
	return core
}

func uniqSortedNames(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func dropDupMntAdmin(lines []adminLine, mntCust map[string]bool) []adminLine {
	if len(mntCust) == 0 {
		return lines
	}
	var out []adminLine
	for _, a := range lines {
		if strings.Contains(a.Title, "정기점검") && mntCust[strings.TrimSpace(a.CustomerID)] {
			continue
		}
		out = append(out, a)
	}
	return out
}

func mergeCustSet(a, b map[string]bool) map[string]bool {
	out := map[string]bool{}
	for k, v := range a {
		if v {
			out[k] = true
		}
	}
	for k, v := range b {
		if v {
			out[k] = true
		}
	}
	return out
}

func slashMD(iso string) string {
	t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(iso), time.Local)
	if err != nil {
		return iso
	}
	return fmt.Sprintf("%d/%d", int(t.Month()), t.Day())
}

type adminLine struct {
	ID, Title, Date, ProjectID, ParentID, Role, CustomerID, WorkType string
}

type asOpenSummary struct {
	N      int
	Detail string
}

const companyWeeklyAdminTypesSQL = `t.work_type IN ('admin','support','project_work','work')`

func collapseCompanyAdminLines(lines []adminLine) []adminLine {
	type grp struct {
		first adminLine
		n     int
	}
	order := []string{}
	bag := map[string]*grp{}
	var rest []adminLine
	for _, a := range lines {
		if a.Role != model.RecurrenceRoleOccurrence || strings.TrimSpace(a.ParentID) == "" {
			rest = append(rest, a)
			continue
		}
		key := a.ParentID
		g, ok := bag[key]
		if !ok {
			g = &grp{first: a}
			bag[key] = g
			order = append(order, key)
		}
		g.n++
	}
	out := append([]adminLine{}, rest...)
	for _, key := range order {
		g := bag[key]
		a := g.first
		a.ID = key
		a.Title = strings.TrimPrefix(model.FormatRecurrenceTimes(a.Title, g.n), "·")
		a.Date = ""
		out = append(out, a)
	}
	return out
}

func (r *StatsRepo) countCompanyMntSitesDone(projectID, from, toEx, orgID string) (int, error) {
	mntSQL, mntArgs := r.filterMnt(model.StatsMeetingFilter{OrgID: orgID, DateBasis: model.BasisComplete})
	var n int
	args := append([]interface{}{from, toEx, projectID}, mntArgs...)
	err := r.db.QueryRow(`
		SELECT COUNT(DISTINCT v.customer_id) FROM maintenance_visits v
		WHERE COALESCE(v.completed,0)=1
		  AND `+mntDoneDateSQL+` >= ? AND `+mntDoneDateSQL+` < ?
		  AND TRIM(COALESCE(v.project_id,'')) = ?`+mntSQL, args...).Scan(&n)
	return n, err
}

func (r *StatsRepo) countCompanyMntSitesPlanned(projectID, from, toEx, orgID string) (int, error) {
	mntSQL, mntArgs := r.filterMnt(model.StatsMeetingFilter{OrgID: orgID, DateBasis: model.BasisVisit})
	var n int
	args := append([]interface{}{from, toEx, projectID}, mntArgs...)
	err := r.db.QueryRow(`
		SELECT COUNT(DISTINCT v.customer_id) FROM maintenance_visits v
		WHERE v.visit_date >= ? AND v.visit_date < ?
		  AND TRIM(COALESCE(v.project_id,'')) = ?`+mntSQL, args...).Scan(&n)
	return n, err
}

func (r *StatsRepo) listCompanyMntSiteNames(projectID, from, toEx, orgID string, planned bool) ([]string, map[string]bool, error) {
	cust := map[string]bool{}
	var names []string
	var rows *sql.Rows
	var err error
	if planned {
		mntSQL, mntArgs := r.filterMnt(model.StatsMeetingFilter{OrgID: orgID, DateBasis: model.BasisVisit})
		args := append([]interface{}{from, toEx, projectID}, mntArgs...)
		rows, err = r.db.Query(`
			SELECT DISTINCT v.customer_id, COALESCE(NULLIF(TRIM(c.org_name),''), v.customer_id)
			FROM maintenance_visits v
			LEFT JOIN customers c ON c.customer_id = v.customer_id
			WHERE v.visit_date >= ? AND v.visit_date < ?
			  AND TRIM(COALESCE(v.project_id,'')) = ?`+mntSQL+`
			ORDER BY 2`, args...)
	} else {
		mntSQL, mntArgs := r.filterMnt(model.StatsMeetingFilter{OrgID: orgID, DateBasis: model.BasisComplete})
		args := append([]interface{}{from, toEx, projectID}, mntArgs...)
		rows, err = r.db.Query(`
			SELECT DISTINCT v.customer_id, COALESCE(NULLIF(TRIM(c.org_name),''), v.customer_id)
			FROM maintenance_visits v
			LEFT JOIN customers c ON c.customer_id = v.customer_id
			WHERE COALESCE(v.completed,0)=1
			  AND `+mntDoneDateSQL+` >= ? AND `+mntDoneDateSQL+` < ?
			  AND TRIM(COALESCE(v.project_id,'')) = ?`+mntSQL+`
			ORDER BY 2`, args...)
	}
	if err != nil {
		return nil, cust, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, cust, err
		}
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		cust[id] = true
		names = append(names, name)
	}
	return names, cust, rows.Err()
}

func (r *StatsRepo) listIncompleteMntVisits(from, toEx, orgID string) ([]model.IncompleteMntVisit, error) {
	mntSQL, mntArgs := r.filterMnt(model.StatsMeetingFilter{OrgID: orgID, DateBasis: model.BasisVisit})
	args := append([]interface{}{from, toEx}, mntArgs...)
	rows, err := r.db.Query(`
		SELECT v.visit_id, COALESCE(c.org_name,''), v.visit_date,
		       COALESCE(v.plan_id,''), COALESCE(v.project_id,'')
		FROM maintenance_visits v
		LEFT JOIN customers c ON c.customer_id = v.customer_id
		WHERE COALESCE(NULLIF(TRIM(v.completed_date),''), v.visit_date) >= ?
		  AND COALESCE(NULLIF(TRIM(v.completed_date),''), v.visit_date) < ?
		  AND COALESCE(v.completed,0)=0`+statsProjectKindSQL("v.project_id")+mntSQL+`
		ORDER BY v.visit_date, c.org_name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.IncompleteMntVisit
	for rows.Next() {
		var v model.IncompleteMntVisit
		if err := rows.Scan(&v.VisitID, &v.OrgName, &v.VisitDate, &v.PlanID, &v.ProjectID); err != nil {
			return nil, err
		}
		v.Href = "/maintenance"
		if v.PlanID != "" {
			v.Href = "/maintenance/" + v.PlanID
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *StatsRepo) countCompanyASDone(projectID, from, toEx, orgID string) (int, error) {
	asSQL, asArgs := r.filterAS(model.StatsMeetingFilter{OrgID: orgID, DateBasis: model.BasisComplete})
	var n int
	args := append([]interface{}{from, toEx, projectID}, asArgs...)
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM as_receipts ar
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		WHERE TRIM(COALESCE(ar.complete_datetime,'')) != ''
		  AND ar.complete_datetime >= ? AND ar.complete_datetime < ?
		  AND TRIM(COALESCE(ar.project_id,'')) = ?`+asSQL, args...).Scan(&n)
	return n, err
}

func (r *StatsRepo) listCompanyAdminDone(projectID, from, toEx, orgID string) ([]adminLine, error) {
	adminSQL, adminArgs := r.filterAdmin(model.StatsMeetingFilter{OrgID: orgID, DateBasis: model.BasisComplete, ExcludeSalesActivity: true})
	lines, err := r.queryAdminLines(`
		SELECT t.task_id, t.title, `+adminTaskCompleteDateSQL+`, COALESCE(t.project_id,''),
		       COALESCE(t.parent_task_id,''), COALESCE(t.recurrence_role,''), COALESCE(t.customer_id,''),
		       COALESCE(t.work_type,'')
		FROM work_tasks t
		WHERE `+companyWeeklyAdminTypesSQL+SQLRecurrenceWorkUnit+adminSQL+`
		  AND t.status = 'complete'
		  AND `+adminTaskCompleteDateSQL+` >= date(?) AND `+adminTaskCompleteDateSQL+` < date(?)
		  AND TRIM(COALESCE(t.project_id,'')) = ?
		ORDER BY `+adminTaskCompleteDateSQL+`, t.title`, append(append([]interface{}{}, adminArgs...), from, toEx, projectID)...)
	if err != nil {
		return nil, err
	}
	return collapseCompanyAdminLines(lines), nil
}

func companyAdminPlanDateSQL() string {
	return `CASE WHEN COALESCE(t.recurrence_role,'')='occurrence'
		THEN NULLIF(TRIM(t.work_date),'')
		ELSE COALESCE(NULLIF(TRIM(t.work_date),''), NULLIF(TRIM(t.due_date),'')) END`
}

func (r *StatsRepo) listCompanyAdminPlan(projectID, from, toEx, orgID string) ([]adminLine, error) {
	d := companyAdminPlanDateSQL()
	adminSQL, adminArgs := r.filterAdmin(model.StatsMeetingFilter{OrgID: orgID, DateBasis: model.BasisVisit, ExcludeSalesActivity: true})
	lines, err := r.queryAdminLines(`
		SELECT t.task_id, t.title, COALESCE(`+d+`,''), COALESCE(t.project_id,''),
		       COALESCE(t.parent_task_id,''), COALESCE(t.recurrence_role,''), COALESCE(t.customer_id,''),
		       COALESCE(t.work_type,'')
		FROM work_tasks t
		WHERE `+companyWeeklyAdminTypesSQL+SQLRecurrenceWorkUnit+adminSQL+`
		  AND t.status NOT IN ('complete','cancelled')
		  AND TRIM(COALESCE(`+d+`,'')) != ''
		  AND `+d+` >= ? AND `+d+` < ?
		  AND TRIM(COALESCE(t.project_id,'')) = ?
		ORDER BY `+d+`, t.title`, append(append([]interface{}{}, adminArgs...), from, toEx, projectID)...)
	if err != nil {
		return nil, err
	}
	return collapseCompanyAdminLines(lines), nil
}

func (r *StatsRepo) listCompanyAdminCarry(projectID, weekFrom, planFrom, orgID string) ([]adminLine, error) {
	d := companyAdminPlanDateSQL()
	f := r.attachMetrics(model.StatsMeetingFilter{OrgID: orgID, ExcludeSalesActivity: true})
	orgSQL, orgArgs := mustOrgSQL("t", f.OrgID)
	adminSQL := ` AND TRIM(COALESCE(t.source_type,'')) NOT IN ('as','maintenance','sales_activity')` +
		statsProjectKindSQL("t.project_id") + orgSQL + statsTestSQL("t", f)
	lines, err := r.queryAdminLines(`
		SELECT t.task_id, t.title, COALESCE(`+d+`,''), COALESCE(t.project_id,''),
		       COALESCE(t.parent_task_id,''), COALESCE(t.recurrence_role,''), COALESCE(t.customer_id,''),
		       COALESCE(t.work_type,'')
		FROM work_tasks t
		WHERE `+companyWeeklyAdminTypesSQL+SQLRecurrenceWorkUnit+adminSQL+`
		  AND t.status NOT IN ('complete','cancelled')
		  AND TRIM(COALESCE(t.project_id,'')) = ?
		  AND (
		        TRIM(COALESCE(`+d+`,'')) = ''
		     OR date(`+d+`) < date(?)
		  )
		ORDER BY `+d+`, t.title`, append(append([]interface{}{}, orgArgs...), projectID, planFrom)...)
	if err != nil {
		return nil, err
	}
	for i := range lines {
		if n := delayWeeks(lines[i].Date, weekFrom); n >= 1 {
			lines[i].Title = strings.TrimSpace(lines[i].Title) + fmt.Sprintf("(%d주 지연)", n)
		}
	}
	return collapseCompanyAdminLines(lines), nil
}

func delayWeeks(date, weekFrom string) int {
	d, err1 := time.ParseInLocation("2006-01-02", firstISODay(date), time.Local)
	w, err2 := time.ParseInLocation("2006-01-02", firstISODay(weekFrom), time.Local)
	if err1 != nil || err2 != nil || !d.Before(w) {
		return 0
	}
	return int(w.Sub(d).Hours()/24) / 7
}

func firstISODay(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

func (r *StatsRepo) listCompanyASOpen(projectID, orgID string) (asOpenSummary, error) {
	asSQL, asArgs := r.filterAS(model.StatsMeetingFilter{OrgID: orgID, DateBasis: model.BasisReceipt})
	rows, err := r.db.Query(`
		SELECT COALESCE(NULLIF(TRIM(c.org_name),''), COALESCE(ar.as_number,''), ar.as_id),
		       COALESCE(ar.symptom,'')
		FROM as_receipts ar
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		LEFT JOIN customers c ON c.customer_id = ar.customer_id
		WHERE TRIM(COALESCE(ar.complete_datetime,'')) = ''
		  AND LOWER(TRIM(COALESCE(ar.status,''))) NOT IN ('completed','complete','cancelled','canceled','cancel')
		  AND TRIM(COALESCE(ar.project_id,'')) = ?`+asSQL+`
		ORDER BY ar.receipt_datetime DESC`, append([]interface{}{projectID}, asArgs...)...)
	if err != nil {
		return asOpenSummary{}, err
	}
	defer rows.Close()
	var parts []string
	n := 0
	for rows.Next() {
		var cust, symptom string
		if err := rows.Scan(&cust, &symptom); err != nil {
			return asOpenSummary{}, err
		}
		n++
		if len(parts) >= 6 {
			continue
		}
		cust = strings.TrimSpace(cust)
		symptom = truncateRunes(strings.TrimSpace(strings.ReplaceAll(symptom, "\n", " ")), 20)
		if cust != "" {
			parts = append(parts, cust)
		}
		if symptom != "" {
			parts = append(parts, symptom)
		}
	}
	return asOpenSummary{N: n, Detail: strings.Join(parts, " · ")}, rows.Err()
}

func (r *StatsRepo) listCompanyASNextActionPlan(projectID, from, toEx, orgID string) ([]adminLine, error) {
	asSQL, asArgs := r.filterAS(model.StatsMeetingFilter{OrgID: orgID, DateBasis: model.BasisReceipt})
	rows, err := r.db.Query(`
		SELECT ar.as_id, COALESCE(NULLIF(TRIM(c.org_name),''), COALESCE(ar.as_number,''), ar.as_id),
		       MAX(ap.next_action_date)
		FROM as_receipts ar
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		LEFT JOIN customers c ON c.customer_id = ar.customer_id
		INNER JOIN as_processes ap ON ap.as_id = ar.as_id
		WHERE TRIM(COALESCE(ar.complete_datetime,'')) = ''
		  AND TRIM(COALESCE(ap.next_action_date,'')) != ''
		  AND ap.next_action_date >= ? AND ap.next_action_date < ?
		  AND TRIM(COALESCE(ar.project_id,'')) = ?`+asSQL+`
		GROUP BY ar.as_id
		ORDER BY 3`, append([]interface{}{from, toEx, projectID}, asArgs...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []adminLine
	for rows.Next() {
		var id, name, date string
		if err := rows.Scan(&id, &name, &date); err != nil {
			return nil, err
		}
		out = append(out, adminLine{ID: id, Title: "AS 후속 " + name, Date: date, ProjectID: projectID, WorkType: "as"})
	}
	return out, rows.Err()
}

func (r *StatsRepo) queryAdminLines(q string, args ...interface{}) ([]adminLine, error) {
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []adminLine
	for rows.Next() {
		var a adminLine
		if err := rows.Scan(&a.ID, &a.Title, &a.Date, &a.ProjectID, &a.ParentID, &a.Role, &a.CustomerID, &a.WorkType); err != nil {
			return nil, err
		}
		if a.WorkType == model.WBWorkProjectWork {
			a.Title = formatProjectWorkLine(a.Title, r.taskSiteNames(a.ID), a.Date)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *StatsRepo) listCompanyUnassigned(p model.CompanyWeeklyPeriod, mapped map[string]bool, orgID string) ([]model.CompanyWeeklyUnassigned, error) {
	var out []model.CompanyWeeklyUnassigned
	addIf := func(row model.CompanyWeeklyUnassigned) {
		pid := strings.TrimSpace(row.ProjectID)
		if pid != "" && mapped[pid] {
			return
		}
		out = append(out, row)
	}

	mntDoneSQL, mntDoneArgs := r.filterMnt(model.StatsMeetingFilter{OrgID: orgID, DateBasis: model.BasisComplete})
	asDoneSQL, asDoneArgs := r.filterAS(model.StatsMeetingFilter{OrgID: orgID, DateBasis: model.BasisComplete})
	adminDoneSQL, adminDoneArgs := r.filterAdmin(model.StatsMeetingFilter{OrgID: orgID, DateBasis: model.BasisComplete, ExcludeSalesActivity: true})
	mntPlanSQL, mntPlanArgs := r.filterMnt(model.StatsMeetingFilter{OrgID: orgID, DateBasis: model.BasisVisit})
	adminPlanSQL, adminPlanArgs := r.filterAdmin(model.StatsMeetingFilter{OrgID: orgID, DateBasis: model.BasisVisit, ExcludeSalesActivity: true})

	vrows, err := r.db.Query(`
		SELECT v.visit_id, COALESCE(c.org_name,''), `+mntDoneDateSQL+`, COALESCE(v.project_id,''), COALESCE(p.name,''), COALESCE(v.plan_id,'')
		FROM maintenance_visits v
		LEFT JOIN customers c ON c.customer_id = v.customer_id
		LEFT JOIN work_projects p ON p.project_id = v.project_id
		WHERE COALESCE(v.completed,0)=1
		  AND `+mntDoneDateSQL+` >= ? AND `+mntDoneDateSQL+` < ?`+statsProjectKindSQL("v.project_id")+mntDoneSQL,
		append([]interface{}{p.PrevFrom, p.PrevToEx}, mntDoneArgs...)...)
	if err != nil {
		return nil, err
	}
	for vrows.Next() {
		var id, org, date, pid, pname, planID string
		if err := vrows.Scan(&id, &org, &date, &pid, &pname, &planID); err != nil {
			vrows.Close()
			return nil, err
		}
		href := "/maintenance"
		if planID != "" {
			href = "/maintenance/" + planID
		}
		addIf(model.CompanyWeeklyUnassigned{
			Kind: model.ScopeWorkMaintenance, KindLabel: "정기점검",
			ID: id, Title: firstNonEmpty(org, id), Date: date, ProjectID: pid, ProjectName: pname,
			Href: href, Bucket: "prev", BucketLabel: "전주 완료",
		})
	}
	vrows.Close()

	arows, err := r.db.Query(`
		SELECT ar.as_id, COALESCE(ar.as_number,''), COALESCE(c.org_name,''), date(ar.complete_datetime),
		       COALESCE(ar.project_id,''), COALESCE(p.name,'')
		FROM as_receipts ar
		LEFT JOIN customers c ON c.customer_id = ar.customer_id
		LEFT JOIN work_projects p ON p.project_id = ar.project_id
		WHERE TRIM(COALESCE(ar.complete_datetime,'')) != ''
		  AND ar.complete_datetime >= ? AND ar.complete_datetime < ?`+statsProjectKindSQL("ar.project_id")+asDoneSQL,
		append([]interface{}{p.PrevFrom, p.PrevToEx}, asDoneArgs...)...)
	if err != nil {
		return nil, err
	}
	for arows.Next() {
		var id, num, org, date, pid, pname string
		if err := arows.Scan(&id, &num, &org, &date, &pid, &pname); err != nil {
			arows.Close()
			return nil, err
		}
		addIf(model.CompanyWeeklyUnassigned{
			Kind: model.ScopeWorkAS, KindLabel: "AS",
			ID: id, Title: firstNonEmpty(org, num), Date: date, ProjectID: pid, ProjectName: pname,
			Href: "/as/" + id, Bucket: "prev", BucketLabel: "전주 완료",
		})
	}
	arows.Close()

	trows, err := r.db.Query(`
		SELECT t.task_id, t.title, `+adminTaskCompleteDateSQL+`, COALESCE(t.project_id,''), COALESCE(p.name,''), COALESCE(t.work_type,'')
		FROM work_tasks t
		LEFT JOIN work_projects p ON p.project_id = t.project_id
		WHERE `+companyWeeklyAdminTypesSQL+SQLRecurrenceWorkUnit+adminDoneSQL+`
		  AND t.status = 'complete'
		  AND `+adminTaskCompleteDateSQL+` >= date(?) AND `+adminTaskCompleteDateSQL+` < date(?)`+statsProjectKindSQL("t.project_id"),
		append(append([]interface{}{}, adminDoneArgs...), p.PrevFrom, p.PrevToEx)...)
	if err != nil {
		return nil, err
	}
	for trows.Next() {
		var id, title, date, pid, pname, wtype string
		if err := trows.Scan(&id, &title, &date, &pid, &pname, &wtype); err != nil {
			trows.Close()
			return nil, err
		}
		label := "행정/지원"
		if wtype == model.WBWorkProjectWork {
			label = "작업"
			title = formatProjectWorkLine(title, r.taskSiteNames(id), date)
		}
		addIf(model.CompanyWeeklyUnassigned{
			Kind: model.ScopeWorkAdmin, KindLabel: label,
			ID: id, Title: title, Date: date, ProjectID: pid, ProjectName: pname,
			Href: model.WorkTaskOpenHref(id, wtype), Bucket: "prev", BucketLabel: "전주 완료",
		})
	}
	trows.Close()

	prows, err := r.db.Query(`
		SELECT v.visit_id, COALESCE(c.org_name,''), v.visit_date, COALESCE(v.project_id,''), COALESCE(p.name,''), COALESCE(v.plan_id,'')
		FROM maintenance_visits v
		LEFT JOIN customers c ON c.customer_id = v.customer_id
		LEFT JOIN work_projects p ON p.project_id = v.project_id
		WHERE v.visit_date >= ? AND v.visit_date < ?`+statsProjectKindSQL("v.project_id")+mntPlanSQL,
		append([]interface{}{p.ThisFrom, p.ThisToEx}, mntPlanArgs...)...)
	if err != nil {
		return nil, err
	}
	for prows.Next() {
		var id, org, date, pid, pname, planID string
		if err := prows.Scan(&id, &org, &date, &pid, &pname, &planID); err != nil {
			prows.Close()
			return nil, err
		}
		href := "/maintenance"
		if planID != "" {
			href = "/maintenance/" + planID
		}
		addIf(model.CompanyWeeklyUnassigned{
			Kind: model.ScopeWorkMaintenance, KindLabel: "정기점검",
			ID: id, Title: firstNonEmpty(org, id), Date: date, ProjectID: pid, ProjectName: pname,
			Href: href, Bucket: "plan", BucketLabel: "금주 계획",
		})
	}
	prows.Close()

	crows, err := r.db.Query(`
		SELECT t.task_id, t.title, COALESCE(`+companyAdminPlanDateSQL()+`,''), COALESCE(t.project_id,''), COALESCE(p.name,''),
		       CASE WHEN TRIM(COALESCE(`+companyAdminPlanDateSQL()+`,'')) != '' AND date(`+companyAdminPlanDateSQL()+`) >= date(?) AND date(`+companyAdminPlanDateSQL()+`) < date(?) THEN 'plan' ELSE 'carry' END,
		       COALESCE(t.work_type,'')
		FROM work_tasks t
		LEFT JOIN work_projects p ON p.project_id = t.project_id
		WHERE `+companyWeeklyAdminTypesSQL+SQLRecurrenceWorkUnit+adminPlanSQL+`
		  AND t.status NOT IN ('complete','cancelled')
		  AND (
		        TRIM(COALESCE(`+companyAdminPlanDateSQL()+`,'')) = ''
		     OR date(`+companyAdminPlanDateSQL()+`) < date(?)
		     OR (date(`+companyAdminPlanDateSQL()+`) >= date(?) AND date(`+companyAdminPlanDateSQL()+`) < date(?))
		  )`+statsProjectKindSQL("t.project_id"),
		append(append([]interface{}{}, adminPlanArgs...), p.ThisFrom, p.ThisToEx, p.ThisFrom, p.ThisFrom, p.ThisToEx)...)
	if err != nil {
		return nil, err
	}
	for crows.Next() {
		var id, title, date, pid, pname, bucket, wtype string
		if err := crows.Scan(&id, &title, &date, &pid, &pname, &bucket, &wtype); err != nil {
			crows.Close()
			return nil, err
		}
		label := "금주 계획"
		if bucket == "carry" {
			label = "이월"
		}
		kindLabel := "행정/지원"
		if wtype == model.WBWorkProjectWork {
			kindLabel = "작업"
			title = formatProjectWorkLine(title, r.taskSiteNames(id), date)
		}
		addIf(model.CompanyWeeklyUnassigned{
			Kind: model.ScopeWorkAdmin, KindLabel: kindLabel,
			ID: id, Title: title, Date: date, ProjectID: pid, ProjectName: pname,
			Href: model.WorkTaskOpenHref(id, wtype), Bucket: bucket, BucketLabel: label,
		})
	}
	crows.Close()
	return out, nil
}

func (r *StatsRepo) taskSiteNames(taskID string) []string {
	rows, err := r.db.Query(`
		SELECT COALESCE(NULLIF(TRIM(c.org_name),''), wts.customer_id)
		FROM work_task_sites wts
		LEFT JOIN customers c ON c.customer_id = wts.customer_id
		WHERE wts.task_id=?
		ORDER BY 1`, taskID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return names
		}
		names = append(names, n)
	}
	return names
}

func formatProjectWorkLine(title string, names []string, _ string) string {
	title = strings.TrimSpace(title)
	names = uniqSortedNames(names)
	if len(names) == 0 {
		return title
	}
	shown := names
	extra := 0
	if len(shown) > 2 {
		extra = len(shown) - 2
		shown = shown[:2]
	}
	core := title + "(" + strings.Join(shown, "·")
	if extra > 0 {
		core += fmt.Sprintf(" 외 %d곳", extra)
	}
	return core + ")"
}

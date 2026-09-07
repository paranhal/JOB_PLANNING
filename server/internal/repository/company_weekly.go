package repository

import (
	"fmt"
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
	p := CompanyWeeklyPeriod(anchor)
	mapRows, err := r.listWeeklyReportRows()
	if err != nil {
		return model.CompanyWeeklyDraft{}, err
	}
	mapped := map[string]bool{}
	for _, row := range mapRows {
		if id := strings.TrimSpace(row.ProjectID); id != "" {
			mapped[id] = true
		}
	}

	out := model.CompanyWeeklyDraft{Period: p}
	for _, row := range mapRows {
		item := model.CompanyWeeklyRow{
			RowKey:      row.Key,
			SheetRow:    row.SheetRow,
			Division:    row.Division,
			Team:        row.Team,
			NoLabel:     row.NoLabel,
			DisplayName: row.DisplayName,
			ProjectID:   row.ProjectID,
			RowKind:     row.Kind,
			Highlight:   row.Highlight,
			IsActive:    row.Active,
		}
		if row.Kind == "project" && strings.TrimSpace(row.ProjectID) == "" {
			item.MissingProject = true
		}
		if pid := strings.TrimSpace(row.ProjectID); pid != "" {
			item.MntDoneSites, err = r.countCompanyMntSitesDone(pid, p.PrevFrom, p.PrevToEx)
			if err != nil {
				return out, err
			}
			item.ASDoneCount, err = r.countCompanyASDone(pid, p.PrevFrom, p.PrevToEx)
			if err != nil {
				return out, err
			}
			item.MntPlanSites, err = r.countCompanyMntSitesPlanned(pid, p.ThisFrom, p.ThisToEx)
			if err != nil {
				return out, err
			}
			adminDone, err := r.listCompanyAdminDone(pid, p.PrevFrom, p.PrevToEx)
			if err != nil {
				return out, err
			}
			adminPlan, err := r.listCompanyAdminPlan(pid, p.ThisFrom, p.ThisToEx)
			if err != nil {
				return out, err
			}
			carry, err := r.listCompanyAdminCarry(pid, p.PrevFrom, p.PrevToEx)
			if err != nil {
				return out, err
			}
			item.PrevText = composePrevText(item.MntDoneSites, item.ASDoneCount, adminDone)
			item.PlanText = composePlanText(item.MntPlanSites, adminPlan, carry)
		}
		out.Rows = append(out.Rows, item)
	}

	unassigned, err := r.listCompanyUnassigned(p, mapped)
	if err != nil {
		return out, err
	}
	out.Unassigned = unassigned
	return out, nil
}

func composePrevText(mntSites, asCount int, admin []adminLine) string {
	var lines []string
	if mntSites > 0 && asCount > 0 {
		lines = append(lines, fmt.Sprintf("·정기점검 %d사이트, AS %d건 처리완료", mntSites, asCount))
	} else if mntSites > 0 {
		lines = append(lines, fmt.Sprintf("·정기점검 %d사이트", mntSites))
	} else if asCount > 0 {
		lines = append(lines, fmt.Sprintf("·AS %d건 처리완료", asCount))
	}
	for _, a := range admin {
		if strings.TrimSpace(a.Date) == "" {
			lines = append(lines, "·"+a.Title)
			continue
		}
		lines = append(lines, "·"+a.Title+"("+slashMD(a.Date)+")")
	}
	return strings.Join(lines, "\n")
}

func composePlanText(mntSites int, plan, carry []adminLine) string {
	var lines []string
	if mntSites > 0 {
		lines = append(lines, fmt.Sprintf("·정기점검 %d사이트", mntSites))
	}
	for _, a := range plan {
		lines = append(lines, "·"+a.Title)
	}
	for _, a := range carry {
		lines = append(lines, "·"+a.Title+"(이월)")
	}
	return strings.Join(lines, "\n")
}

func slashMD(iso string) string {
	t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(iso), time.Local)
	if err != nil {
		return iso
	}
	return fmt.Sprintf("%d/%d", int(t.Month()), t.Day())
}

type adminLine struct {
	ID, Title, Date, ProjectID, ParentID, Role string
}

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

func (r *StatsRepo) countCompanyMntSitesDone(projectID, from, toEx string) (int, error) {
	mntSQL, mntArgs := r.filterMnt(model.StatsMeetingFilter{})
	var n int
	args := append([]interface{}{from, toEx, projectID}, mntArgs...)
	err := r.db.QueryRow(`
		SELECT COUNT(DISTINCT v.customer_id) FROM maintenance_visits v
		WHERE COALESCE(v.completed,0)=1
		  AND `+mntDoneDateSQL+` >= ? AND `+mntDoneDateSQL+` < ?
		  AND TRIM(COALESCE(v.project_id,'')) = ?`+mntSQL, args...).Scan(&n)
	return n, err
}

func (r *StatsRepo) countCompanyMntSitesPlanned(projectID, from, toEx string) (int, error) {
	mntSQL, mntArgs := r.filterMnt(model.StatsMeetingFilter{})
	var n int
	args := append([]interface{}{from, toEx, projectID}, mntArgs...)
	err := r.db.QueryRow(`
		SELECT COUNT(DISTINCT v.customer_id) FROM maintenance_visits v
		WHERE v.visit_date >= ? AND v.visit_date < ?
		  AND TRIM(COALESCE(v.project_id,'')) = ?`+mntSQL, args...).Scan(&n)
	return n, err
}

func (r *StatsRepo) countCompanyASDone(projectID, from, toEx string) (int, error) {
	asSQL, asArgs := r.filterAS(model.StatsMeetingFilter{})
	var n int
	args := append([]interface{}{from, toEx, projectID}, asArgs...)
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM as_receipts ar
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		WHERE TRIM(COALESCE(ar.complete_datetime,'')) != ''
		  AND date(ar.complete_datetime) >= date(?) AND date(ar.complete_datetime) < date(?)
		  AND TRIM(COALESCE(ar.project_id,'')) = ?`+asSQL, args...).Scan(&n)
	return n, err
}

func (r *StatsRepo) listCompanyAdminDone(projectID, from, toEx string) ([]adminLine, error) {
	adminSQL, adminArgs := r.filterAdmin(model.StatsMeetingFilter{})
	lines, err := r.queryAdminLines(`
		SELECT t.task_id, t.title, `+adminTaskCompleteDateSQL+`, COALESCE(t.project_id,''),
		       COALESCE(t.parent_task_id,''), COALESCE(t.recurrence_role,'')
		FROM work_tasks t
		WHERE t.work_type IN ('admin','support')
		  AND COALESCE(t.source_type,'') IN ('', 'sales_activity')`+SQLRecurrenceWorkUnit+adminSQL+`
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
		ELSE NULLIF(TRIM(t.due_date),'') END`
}

func (r *StatsRepo) listCompanyAdminPlan(projectID, from, toEx string) ([]adminLine, error) {
	d := companyAdminPlanDateSQL()
	adminSQL, adminArgs := r.filterAdmin(model.StatsMeetingFilter{})
	lines, err := r.queryAdminLines(`
		SELECT t.task_id, t.title, COALESCE(`+d+`,''), COALESCE(t.project_id,''),
		       COALESCE(t.parent_task_id,''), COALESCE(t.recurrence_role,'')
		FROM work_tasks t
		WHERE t.work_type IN ('admin','support')
		  AND COALESCE(t.source_type,'') IN ('', 'sales_activity')`+SQLRecurrenceWorkUnit+adminSQL+`
		  AND t.status NOT IN ('complete','cancelled')
		  AND TRIM(COALESCE(`+d+`,'')) != ''
		  AND date(`+d+`) >= date(?) AND date(`+d+`) < date(?)
		  AND TRIM(COALESCE(t.project_id,'')) = ?
		ORDER BY date(`+d+`), t.title`, append(append([]interface{}{}, adminArgs...), from, toEx, projectID)...)
	if err != nil {
		return nil, err
	}
	return collapseCompanyAdminLines(lines), nil
}

func (r *StatsRepo) listCompanyAdminCarry(projectID, from, toEx string) ([]adminLine, error) {
	return r.listCompanyAdminPlan(projectID, from, toEx)
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
		if err := rows.Scan(&a.ID, &a.Title, &a.Date, &a.ProjectID, &a.ParentID, &a.Role); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *StatsRepo) listCompanyUnassigned(p model.CompanyWeeklyPeriod, mapped map[string]bool) ([]model.CompanyWeeklyUnassigned, error) {
	var out []model.CompanyWeeklyUnassigned
	addIf := func(row model.CompanyWeeklyUnassigned) {
		pid := strings.TrimSpace(row.ProjectID)
		if pid != "" && mapped[pid] {
			return
		}
		out = append(out, row)
	}

	vrows, err := r.db.Query(`
		SELECT v.visit_id, COALESCE(c.org_name,''), `+mntDoneDateSQL+`, COALESCE(v.project_id,''), COALESCE(p.name,''), COALESCE(v.plan_id,'')
		FROM maintenance_visits v
		LEFT JOIN customers c ON c.customer_id = v.customer_id
		LEFT JOIN work_projects p ON p.project_id = v.project_id
		WHERE COALESCE(v.completed,0)=1
		  AND `+mntDoneDateSQL+` >= ? AND `+mntDoneDateSQL+` < ?`, p.PrevFrom, p.PrevToEx)
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
		  AND date(ar.complete_datetime) >= date(?) AND date(ar.complete_datetime) < date(?)`, p.PrevFrom, p.PrevToEx)
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
		SELECT t.task_id, t.title, `+adminTaskCompleteDateSQL+`, COALESCE(t.project_id,''), COALESCE(p.name,'')
		FROM work_tasks t
		LEFT JOIN work_projects p ON p.project_id = t.project_id
		WHERE t.work_type IN ('admin','support')
		  AND COALESCE(t.source_type,'') IN ('', 'sales_activity')`+SQLRecurrenceWorkUnit+`
		  AND t.status = 'complete'
		  AND `+adminTaskCompleteDateSQL+` >= date(?) AND `+adminTaskCompleteDateSQL+` < date(?)`, p.PrevFrom, p.PrevToEx)
	if err != nil {
		return nil, err
	}
	for trows.Next() {
		var id, title, date, pid, pname string
		if err := trows.Scan(&id, &title, &date, &pid, &pname); err != nil {
			trows.Close()
			return nil, err
		}
		addIf(model.CompanyWeeklyUnassigned{
			Kind: model.ScopeWorkAdmin, KindLabel: "행정/지원",
			ID: id, Title: title, Date: date, ProjectID: pid, ProjectName: pname,
			Href: "/workboard/tasks/" + id, Bucket: "prev", BucketLabel: "전주 완료",
		})
	}
	trows.Close()

	prows, err := r.db.Query(`
		SELECT v.visit_id, COALESCE(c.org_name,''), v.visit_date, COALESCE(v.project_id,''), COALESCE(p.name,''), COALESCE(v.plan_id,'')
		FROM maintenance_visits v
		LEFT JOIN customers c ON c.customer_id = v.customer_id
		LEFT JOIN work_projects p ON p.project_id = v.project_id
		WHERE v.visit_date >= ? AND v.visit_date < ?`, p.ThisFrom, p.ThisToEx)
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
		       CASE WHEN date(`+companyAdminPlanDateSQL()+`) >= date(?) AND date(`+companyAdminPlanDateSQL()+`) < date(?) THEN 'plan' ELSE 'carry' END
		FROM work_tasks t
		LEFT JOIN work_projects p ON p.project_id = t.project_id
		WHERE t.work_type IN ('admin','support')
		  AND COALESCE(t.source_type,'') IN ('', 'sales_activity')`+SQLRecurrenceWorkUnit+`
		  AND t.status NOT IN ('complete','cancelled')
		  AND TRIM(COALESCE(`+companyAdminPlanDateSQL()+`,'')) != ''
		  AND (
		        (date(`+companyAdminPlanDateSQL()+`) >= date(?) AND date(`+companyAdminPlanDateSQL()+`) < date(?))
		     OR (date(`+companyAdminPlanDateSQL()+`) >= date(?) AND date(`+companyAdminPlanDateSQL()+`) < date(?))
		  )`, p.ThisFrom, p.ThisToEx, p.ThisFrom, p.ThisToEx, p.PrevFrom, p.PrevToEx)
	if err != nil {
		return nil, err
	}
	for crows.Next() {
		var id, title, date, pid, pname, bucket string
		if err := crows.Scan(&id, &title, &date, &pid, &pname, &bucket); err != nil {
			crows.Close()
			return nil, err
		}
		label := "금주 계획"
		if bucket == "carry" {
			label = "이월"
		}
		addIf(model.CompanyWeeklyUnassigned{
			Kind: model.ScopeWorkAdmin, KindLabel: "행정/지원",
			ID: id, Title: title, Date: date, ProjectID: pid, ProjectName: pname,
			Href: "/workboard/tasks/" + id, Bucket: bucket, BucketLabel: label,
		})
	}
	crows.Close()
	return out, nil
}

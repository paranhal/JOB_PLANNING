package repository

import (
	"strings"

	"customer-support/internal/model"
)

func (r *StatsRepo) composeActivityTexts(row weeklyReportRow, p model.CompanyWeeklyPeriod, mapped map[string]bool, orgID string) (prev, plan string, err error) {
	kind := activityKindOf(row)
	exclProjects, exclSales := mappedExcludeSQL(mapped)
	var prevLines, planLines []string
	switch kind {
	case "asset":
		prevLines, err = r.listAssetActivityLines(p.PrevFrom, p.PrevToEx, true, exclProjects, orgID)
		if err != nil {
			return "", "", err
		}
		planLines, err = r.listAssetActivityLines(p.ThisFrom, p.ThisToEx, false, exclProjects, orgID)
	case "tech":
		prevLines, err = r.listTechSalesActivityLines(p.PrevFrom, p.PrevToEx, exclSales, orgID)
		if err != nil {
			return "", "", err
		}
		planLines, err = r.listTechSalesActivityLines(p.ThisFrom, p.ThisToEx, exclSales, orgID)
	case "mnt_as":
		prev, plan, err = r.composeUnmappedMntASTexts(p, mapped, orgID)
		return prev, plan, err
	default:
		return "", "", nil
	}
	if err != nil {
		return "", "", err
	}
	return strings.Join(bulletLines(prevLines), "\n"), strings.Join(bulletLines(planLines), "\n"), nil
}

func activityKindOf(row weeklyReportRow) string {
	if row.Kind != "activity" {
		return ""
	}
	switch strings.TrimSpace(row.ActivityKind) {
	case "asset", "tech", "mnt_as":
		return row.ActivityKind
	}
	return ""
}

func mappedExcludeSQL(mapped map[string]bool) (projects, sales string) {
	var pids []string
	for id := range mapped {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		pids = append(pids, strings.ReplaceAll(id, "'", "''"))
	}
	if len(pids) == 0 {
		return "'__none__'", "'__none__'"
	}
	in := "'" + strings.Join(pids, "','") + "'"
	return in, in
}

func bulletLines(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		if !strings.HasPrefix(s, "·") {
			s = "·" + s
		}
		out = append(out, s)
	}
	return out
}

func (r *StatsRepo) listAssetActivityLines(from, toEx string, done bool, exclProjects, orgID string) ([]string, error) {
	dateSQL := `COALESCE(NULLIF(TRIM(t.complete_date),''), NULLIF(TRIM(t.work_date),''), NULLIF(TRIM(t.due_date),''))`
	statusSQL := `t.status='complete'`
	if !done {
		statusSQL = `t.status NOT IN ('complete','cancelled')`
		dateSQL = `COALESCE(NULLIF(TRIM(t.due_date),''), NULLIF(TRIM(t.work_date),''))`
	}
	orgSQL, orgArgs, err := AppendOrgSQL("t", orgIDOrAll(orgID))
	if err != nil {
		return nil, err
	}
	q := `
		SELECT t.task_id, t.title
		FROM work_tasks t
		WHERE ` + statusSQL + `
		  AND TRIM(COALESCE(` + dateSQL + `,'')) != ''
		  AND ` + dateSQL + ` >= ? AND ` + dateSQL + ` < ?
		  AND TRIM(COALESCE(t.project_id,'')) NOT IN (` + exclProjects + `)
		  AND (
		        t.title LIKE '%설치%' OR t.title LIKE '%회수%' OR t.title LIKE '%이동%'
		     OR EXISTS (SELECT 1 FROM work_task_assets wta WHERE wta.task_id=t.task_id)
		  )` + orgSQL + `
		ORDER BY ` + dateSQL + `, t.title`
	args := append([]interface{}{from, toEx}, orgArgs...)
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id, title string
		if err := rows.Scan(&id, &title); err != nil {
			return nil, err
		}
		out = append(out, title)
	}
	return out, rows.Err()
}

func (r *StatsRepo) listTechSalesActivityLines(from, toEx, exclProjects, orgID string) ([]string, error) {
	orgSQL, orgArgs, err := AppendOrgSQL("sp", orgIDOrAll(orgID))
	if err != nil {
		return nil, err
	}
	qorgSQL, qorgArgs, err := AppendOrgSQL("q", orgIDOrAll(orgID))
	if err != nil {
		return nil, err
	}
	var out []string
	arows, err := r.db.Query(`
		SELECT a.activity_id, COALESCE(NULLIF(TRIM(a.title),''), a.activity_type)
		FROM sales_activities a
		LEFT JOIN work_projects wp ON wp.sales_project_id = a.sales_id
		LEFT JOIN sales_projects sp ON sp.sales_id = a.sales_id
		WHERE a.activity_date >= ? AND a.activity_date < ?
		  AND a.activity_type IN ('research','proposal','quote','rfp','bid','requirement','material','rfp_received')
		  AND TRIM(COALESCE(wp.project_id,'')) NOT IN (`+exclProjects+`)`+orgSQL+`
		ORDER BY a.activity_date, a.title`, append([]interface{}{from, toEx}, orgArgs...)...)
	if err != nil {
		return nil, err
	}
	for arows.Next() {
		var id, title string
		if err := arows.Scan(&id, &title); err != nil {
			arows.Close()
			return nil, err
		}
		out = append(out, title)
	}
	arows.Close()

	qrows, err := r.db.Query(`
		SELECT q.quote_id, COALESCE(NULLIF(TRIM(q.title),''), q.quote_no)
		FROM sales_quotes q
		LEFT JOIN work_projects wp ON wp.sales_project_id = q.sales_id
		WHERE q.quote_date >= ? AND q.quote_date < ?
		  AND TRIM(COALESCE(wp.project_id,'')) NOT IN (`+exclProjects+`)`+qorgSQL+`
		ORDER BY q.quote_date, q.quote_no`, append([]interface{}{from, toEx}, qorgArgs...)...)
	if err != nil {
		return nil, err
	}
	defer qrows.Close()
	for qrows.Next() {
		var id, title string
		if err := qrows.Scan(&id, &title); err != nil {
			return nil, err
		}
		out = append(out, "견적 "+title)
	}
	return out, qrows.Err()
}

func (r *StatsRepo) composeUnmappedMntASTexts(p model.CompanyWeeklyPeriod, mapped map[string]bool, orgID string) (prev, plan string, err error) {
	excl, _ := mappedExcludeSQL(mapped)
	mntDone, doneNames, err := r.countUnmappedMnt(p.PrevFrom, p.PrevToEx, orgID, excl, false)
	if err != nil {
		return "", "", err
	}
	asDone, err := r.countUnmappedAS(p.PrevFrom, p.PrevToEx, orgID, excl)
	if err != nil {
		return "", "", err
	}
	mntPlan, planNames, err := r.countUnmappedMnt(p.ThisFrom, p.ThisToEx, orgID, excl, true)
	if err != nil {
		return "", "", err
	}
	return composePrevText(mntDone, doneNames, asDone, nil), composePlanText(mntPlan, planNames, nil, nil), nil
}

func (r *StatsRepo) countUnmappedMnt(from, toEx, orgID, excl string, planned bool) (int, []string, error) {
	var n int
	var names []string
	if planned {
		mntSQL, mntArgs := r.filterMnt(model.StatsMeetingFilter{OrgID: orgIDOrAll(orgID), DateBasis: model.BasisVisit})
		args := append([]interface{}{from, toEx}, mntArgs...)
		err := r.db.QueryRow(`
			SELECT COUNT(DISTINCT v.customer_id) FROM maintenance_visits v
			WHERE v.visit_date >= ? AND v.visit_date < ?
			  AND TRIM(COALESCE(v.project_id,'')) NOT IN (`+excl+`)`+mntSQL, args...).Scan(&n)
		if err != nil {
			return 0, nil, err
		}
		rows, err := r.db.Query(`
			SELECT DISTINCT COALESCE(NULLIF(TRIM(c.org_name),''), v.customer_id)
			FROM maintenance_visits v
			LEFT JOIN customers c ON c.customer_id = v.customer_id
			WHERE v.visit_date >= ? AND v.visit_date < ?
			  AND TRIM(COALESCE(v.project_id,'')) NOT IN (`+excl+`)`+mntSQL+`
			ORDER BY 1`, args...)
		if err != nil {
			return n, nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return n, names, err
			}
			names = append(names, name)
		}
		return n, names, rows.Err()
	}
	mntSQL, mntArgs := r.filterMnt(model.StatsMeetingFilter{OrgID: orgIDOrAll(orgID), DateBasis: model.BasisComplete})
	args := append([]interface{}{from, toEx}, mntArgs...)
	err := r.db.QueryRow(`
		SELECT COUNT(DISTINCT v.customer_id) FROM maintenance_visits v
		WHERE COALESCE(v.completed,0)=1
		  AND `+mntDoneDateSQL+` >= ? AND `+mntDoneDateSQL+` < ?
		  AND TRIM(COALESCE(v.project_id,'')) NOT IN (`+excl+`)`+mntSQL, args...).Scan(&n)
	if err != nil {
		return 0, nil, err
	}
	rows, err := r.db.Query(`
		SELECT DISTINCT COALESCE(NULLIF(TRIM(c.org_name),''), v.customer_id)
		FROM maintenance_visits v
		LEFT JOIN customers c ON c.customer_id = v.customer_id
		WHERE COALESCE(v.completed,0)=1
		  AND `+mntDoneDateSQL+` >= ? AND `+mntDoneDateSQL+` < ?
		  AND TRIM(COALESCE(v.project_id,'')) NOT IN (`+excl+`)`+mntSQL+`
		ORDER BY 1`, args...)
	if err != nil {
		return n, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return n, names, err
		}
		names = append(names, name)
	}
	return n, names, rows.Err()
}

func (r *StatsRepo) countUnmappedAS(from, toEx, orgID, excl string) (int, error) {
	asSQL, asArgs := r.filterAS(model.StatsMeetingFilter{OrgID: orgIDOrAll(orgID), DateBasis: model.BasisComplete})
	var n int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM as_receipts ar
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		WHERE TRIM(COALESCE(ar.complete_datetime,'')) != ''
		  AND ar.complete_datetime >= ? AND ar.complete_datetime < ?
		  AND TRIM(COALESCE(ar.project_id,'')) NOT IN (`+excl+`)`+asSQL, append([]interface{}{from, toEx}, asArgs...)...).Scan(&n)
	if err != nil {
		return 0, err
	}
	return n, nil
}

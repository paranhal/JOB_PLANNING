package repository

import (
	"strings"

	"customer-support/internal/model"
)

func (r *StatsRepo) composeActivityTexts(row weeklyReportRow, p model.CompanyWeeklyPeriod, mapped map[string]bool) (prev, plan string, err error) {
	kind := activityKindOf(row)
	if kind == "" {
		return "", "", nil
	}
	exclProjects, exclSales := mappedExcludeSQL(mapped)
	var prevLines, planLines []string
	switch kind {
	case "asset":
		prevLines, err = r.listAssetActivityLines(p.PrevFrom, p.PrevToEx, true, exclProjects)
		if err != nil {
			return "", "", err
		}
		planLines, err = r.listAssetActivityLines(p.ThisFrom, p.ThisToEx, false, exclProjects)
	case "tech":
		prevLines, err = r.listTechSalesActivityLines(p.PrevFrom, p.PrevToEx, exclSales)
		if err != nil {
			return "", "", err
		}
		planLines, err = r.listTechSalesActivityLines(p.ThisFrom, p.ThisToEx, exclSales)
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
	if row.Key == "X02" || strings.Contains(row.DisplayName, "자산관리") {
		return "asset"
	}
	if row.Key == "X03" || strings.Contains(row.DisplayName, "기술영업") {
		return "tech"
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

func (r *StatsRepo) listAssetActivityLines(from, toEx string, done bool, exclProjects string) ([]string, error) {
	dateSQL := `COALESCE(NULLIF(TRIM(t.complete_date),''), NULLIF(TRIM(t.work_date),''), NULLIF(TRIM(t.due_date),''))`
	statusSQL := `t.status='complete'`
	if !done {
		statusSQL = `t.status NOT IN ('complete','cancelled')`
		dateSQL = `COALESCE(NULLIF(TRIM(t.due_date),''), NULLIF(TRIM(t.work_date),''))`
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
		  )
		ORDER BY ` + dateSQL + `, t.title`
	rows, err := r.db.Query(q, from, toEx)
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

func (r *StatsRepo) listTechSalesActivityLines(from, toEx, exclProjects string) ([]string, error) {
	var out []string
	arows, err := r.db.Query(`
		SELECT a.activity_id, COALESCE(NULLIF(TRIM(a.title),''), a.activity_type)
		FROM sales_activities a
		LEFT JOIN work_projects wp ON wp.sales_project_id = a.sales_id
		WHERE a.activity_date >= ? AND a.activity_date < ?
		  AND a.activity_type IN ('research','proposal','quote','rfp','bid','requirement','material')
		  AND TRIM(COALESCE(wp.project_id,'')) NOT IN (`+exclProjects+`)
		ORDER BY a.activity_date, a.title`, from, toEx)
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
		  AND TRIM(COALESCE(wp.project_id,'')) NOT IN (`+exclProjects+`)
		ORDER BY q.quote_date, q.quote_no`, from, toEx)
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

package repository

import (
	"database/sql"
	"sort"
	"strings"

	"customer-support/internal/model"
)

const (
	weeklyStatBoxMnt   = "mnt"
	weeklyStatBoxAS    = "as"
	weeklyStatBoxWork  = "work"
	weeklyStatBoxAdmin = "admin"
	weeklyStatBoxSales = "sales"
)

var companyWeeklyStatBoxMeta = []struct {
	Key, Label string
}{
	{weeklyStatBoxMnt, "정기점검 방문"},
	{weeklyStatBoxAS, "AS 방문/완료"},
	{weeklyStatBoxWork, "작업 방문"},
	{weeklyStatBoxAdmin, "행정/지원 업무"},
	{weeklyStatBoxSales, "영업"},
}

func emptyCompanyWeeklyStatBoxes() []model.CompanyWeeklyStatBox {
	out := make([]model.CompanyWeeklyStatBox, len(companyWeeklyStatBoxMeta))
	for i, m := range companyWeeklyStatBoxMeta {
		out[i] = model.CompanyWeeklyStatBox{Key: m.Key, Label: m.Label, Rate: model.SiteCheckRateLabel(0, 0)}
	}
	return out
}

func companyWeeklyStatBoxKey(kind, wtype string) string {
	switch kind {
	case model.ScopeWorkMaintenance:
		return weeklyStatBoxMnt
	case model.ScopeWorkAS:
		return weeklyStatBoxAS
	default:
		switch wtype {
		case model.WBWorkProjectWork:
			return weeklyStatBoxWork
		case model.WBWorkSales:
			return weeklyStatBoxSales
		default:
			return weeklyStatBoxAdmin
		}
	}
}

func weeklyStatStatus(done bool) string {
	if done {
		return "완료"
	}
	return "예정"
}

func (r *StatsRepo) weeklyStatQuery(n *int, query string, args ...interface{}) (*sql.Rows, error) {
	if n != nil {
		*n++
	}
	return r.store().Query(query, args...)
}

// ListCompanyWeeklyStatItems 실적 주 항목을 한 번에 읽는다. 카드 수와 쿼리 수가 무관하다. 95-146
func (r *StatsRepo) ListCompanyWeeklyStatItems(p model.CompanyWeeklyPeriod, orgID string) ([]model.CompanyWeeklyStatItem, int, error) {
	orgID, err := reportOrgID(orgID)
	if err != nil {
		return nil, 0, err
	}
	var qn int
	seen := map[string]bool{}
	var out []model.CompanyWeeklyStatItem
	add := func(it model.CompanyWeeklyStatItem) {
		it.BoxKey = companyWeeklyStatBoxKey(it.BoxKey, it.WorkType)
		key := it.BoxKey + ":" + it.ID
		if seen[key] {
			return
		}
		seen[key] = true
		it.Status = weeklyStatStatus(it.Done)
		out = append(out, it)
	}

	if err := r.scanWeeklyStatMntDone(p, orgID, &qn, add); err != nil {
		return nil, qn, err
	}
	if err := r.scanWeeklyStatMntOpen(p, orgID, &qn, add); err != nil {
		return nil, qn, err
	}
	if err := r.scanWeeklyStatASDone(p, orgID, &qn, add); err != nil {
		return nil, qn, err
	}
	if err := r.scanWeeklyStatASOpen(p, orgID, &qn, add); err != nil {
		return nil, qn, err
	}
	taskIDs, err := r.scanWeeklyStatTasksDone(p, orgID, &qn, add)
	if err != nil {
		return nil, qn, err
	}
	openIDs, err := r.scanWeeklyStatTasksOpen(p, orgID, &qn, add)
	if err != nil {
		return nil, qn, err
	}
	taskIDs = append(taskIDs, openIDs...)
	if err := r.fillWeeklyStatTaskSites(taskIDs, &qn, out); err != nil {
		return nil, qn, err
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date < out[j].Date
		}
		return out[i].Title < out[j].Title
	})
	return out, qn, nil
}

func (r *StatsRepo) scanWeeklyStatMntDone(p model.CompanyWeeklyPeriod, orgID string, qn *int, add func(model.CompanyWeeklyStatItem)) error {
	mntDoneF := companyWeeklyRowFilter(orgID, model.BasisComplete)
	mntDoneF.SkipOrg = true
	mntDoneSQL, mntDoneArgs := r.filterMnt(mntDoneF)
	actSQL, actArgs := companyWeeklyMntActorSQL(orgID)
	mntDoneSQL += actSQL + companyWeeklyProjectTestSQL("v.project_id")
	mntDoneArgs = append(mntDoneArgs, actArgs...)
	rows, err := r.weeklyStatQuery(qn, `
		SELECT v.visit_id, COALESCE(c.org_name,''), `+mntDoneDateSQL+`, COALESCE(v.project_id,''), COALESCE(v.assignee,'')
		FROM maintenance_visits v
		LEFT JOIN customers c ON c.customer_id = v.customer_id
		WHERE COALESCE(v.completed,0)=1
		  AND `+mntDoneDateSQL+` >= ? AND `+mntDoneDateSQL+` < ?`+mntDoneSQL,
		append([]interface{}{p.PrevFrom, p.PrevToEx}, mntDoneArgs...)...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var it model.CompanyWeeklyStatItem
		if err := rows.Scan(&it.ID, &it.Site, &it.Date, &it.ProjectID, &it.Assignee); err != nil {
			return err
		}
		it.BoxKey = model.ScopeWorkMaintenance
		it.Title = "정기점검 방문"
		it.Href = "/maintenance/visits/" + it.ID + "/action"
		it.Done = true
		add(it)
	}
	return rows.Err()
}

func (r *StatsRepo) scanWeeklyStatMntOpen(p model.CompanyWeeklyPeriod, orgID string, qn *int, add func(model.CompanyWeeklyStatItem)) error {
	mntPlanF := companyWeeklyRowFilter(orgID, model.BasisVisit)
	mntPlanF.SkipOrg = true
	mntSQL, mntArgs := r.filterMnt(mntPlanF)
	actSQL, actArgs := companyWeeklyMntActorSQL(orgID)
	mntSQL += actSQL + companyWeeklyProjectTestSQL("v.project_id")
	mntArgs = append(mntArgs, actArgs...)
	rows, err := r.weeklyStatQuery(qn, `
		SELECT v.visit_id, COALESCE(c.org_name,''), v.visit_date, COALESCE(v.project_id,''), COALESCE(v.assignee,'')
		FROM maintenance_visits v
		LEFT JOIN customers c ON c.customer_id = v.customer_id
		WHERE COALESCE(v.completed,0)=0
		  AND v.visit_date >= ? AND v.visit_date < ?`+mntSQL,
		append([]interface{}{p.PrevFrom, p.PrevToEx}, mntArgs...)...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var it model.CompanyWeeklyStatItem
		if err := rows.Scan(&it.ID, &it.Site, &it.Date, &it.ProjectID, &it.Assignee); err != nil {
			return err
		}
		it.BoxKey = model.ScopeWorkMaintenance
		it.Title = "정기점검 방문"
		it.Href = "/maintenance/visits/" + it.ID + "/action"
		add(it)
	}
	return rows.Err()
}

const weeklyStatASCancelledSQL = ` AND LOWER(TRIM(COALESCE(ar.status,''))) NOT IN ('cancelled','canceled','cancel')`

func (r *StatsRepo) scanWeeklyStatASDone(p model.CompanyWeeklyPeriod, orgID string, qn *int, add func(model.CompanyWeeklyStatItem)) error {
	asSQL, asArgs := r.filterAS(companyWeeklyRowFilter(orgID, model.BasisComplete))
	asSQL += companyWeeklyProjectTestSQL("ar.project_id")
	rows, err := r.weeklyStatQuery(qn, `
		SELECT ar.as_id, COALESCE(c.org_name,''), COALESCE(ar.as_number,''), date(ar.complete_datetime),
		       COALESCE(ar.project_id,''), COALESCE(ar.assigned_to,'')
		FROM as_receipts ar
		LEFT JOIN customers c ON c.customer_id = ar.customer_id
		WHERE TRIM(COALESCE(ar.complete_datetime,'')) != ''
		  AND ar.complete_datetime >= ? AND ar.complete_datetime < ?`+weeklyStatASCancelledSQL+asSQL,
		append([]interface{}{p.PrevFrom, p.PrevToEx}, asArgs...)...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var it model.CompanyWeeklyStatItem
		var num string
		if err := rows.Scan(&it.ID, &it.Site, &num, &it.Date, &it.ProjectID, &it.Assignee); err != nil {
			return err
		}
		it.BoxKey = model.ScopeWorkAS
		it.Title = firstNonEmpty(num, it.Site, it.ID)
		it.Href = model.ASOpenHref(it.ID)
		it.Done = true
		add(it)
	}
	return rows.Err()
}

func (r *StatsRepo) scanWeeklyStatASOpen(p model.CompanyWeeklyPeriod, orgID string, qn *int, add func(model.CompanyWeeklyStatItem)) error {
	asSQL, asArgs := r.filterAS(companyWeeklyRowFilter(orgID, model.BasisReceipt))
	asSQL += companyWeeklyProjectTestSQL("ar.project_id")
	rows, err := r.weeklyStatQuery(qn, `
		SELECT ar.as_id, COALESCE(c.org_name,''), COALESCE(ar.as_number,''), date(ar.receipt_datetime),
		       COALESCE(ar.project_id,''), COALESCE(ar.assigned_to,'')
		FROM as_receipts ar
		LEFT JOIN customers c ON c.customer_id = ar.customer_id
		WHERE TRIM(COALESCE(ar.complete_datetime,'')) = ''
		  AND ar.receipt_datetime >= ? AND ar.receipt_datetime < ?`+weeklyStatASCancelledSQL+asSQL,
		append([]interface{}{p.PrevFrom, p.PrevToEx}, asArgs...)...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var it model.CompanyWeeklyStatItem
		var num string
		if err := rows.Scan(&it.ID, &it.Site, &num, &it.Date, &it.ProjectID, &it.Assignee); err != nil {
			return err
		}
		it.BoxKey = model.ScopeWorkAS
		it.Title = firstNonEmpty(num, it.Site, it.ID)
		it.Href = model.ASOpenHref(it.ID)
		add(it)
	}
	return rows.Err()
}

func (r *StatsRepo) scanWeeklyStatTaskRows(qn *int, done bool, query string, args []interface{}, add func(model.CompanyWeeklyStatItem)) ([]string, error) {
	rows, err := r.weeklyStatQuery(qn, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var it model.CompanyWeeklyStatItem
		if err := rows.Scan(&it.ID, &it.Title, &it.Date, &it.ProjectID, &it.WorkType, &it.Assignee); err != nil {
			return nil, err
		}
		it.BoxKey = model.ScopeWorkAdmin
		it.Href = model.WorkTaskOpenHref(it.ID, it.WorkType)
		it.Done = done
		ids = append(ids, it.ID)
		add(it)
	}
	return ids, rows.Err()
}

func (r *StatsRepo) scanWeeklyStatTasksDone(p model.CompanyWeeklyPeriod, orgID string, qn *int, add func(model.CompanyWeeklyStatItem)) ([]string, error) {
	adminSQL, adminArgs := r.filterAdmin(companyWeeklyAdminFilter(orgID, model.BasisComplete))
	adminSQL += companyWeeklyProjectTestSQL("t.project_id")
	q := `
		SELECT t.task_id, t.title, ` + adminTaskCompleteDateSQL + `, COALESCE(t.project_id,''), COALESCE(t.work_type,''), COALESCE(t.assignee,'')
		FROM work_tasks t
		WHERE ` + companyWeeklyAdminTypesSQL + SQLRecurrenceWorkUnit + adminSQL + `
		  AND t.status = 'complete'
		  AND ` + adminTaskCompleteDateSQL + ` >= date(?) AND ` + adminTaskCompleteDateSQL + ` < date(?)`
	return r.scanWeeklyStatTaskRows(qn, true, q, append(append([]interface{}{}, adminArgs...), p.PrevFrom, p.PrevToEx), add)
}

func (r *StatsRepo) scanWeeklyStatTasksOpen(p model.CompanyWeeklyPeriod, orgID string, qn *int, add func(model.CompanyWeeklyStatItem)) ([]string, error) {
	adminSQL, adminArgs := r.filterAdmin(companyWeeklyAdminFilter(orgID, model.BasisVisit))
	adminSQL += companyWeeklyProjectTestSQL("t.project_id")
	d := companyAdminPlanDateSQL()
	q := `
		SELECT t.task_id, t.title, COALESCE(` + d + `,''), COALESCE(t.project_id,''), COALESCE(t.work_type,''), COALESCE(t.assignee,'')
		FROM work_tasks t
		WHERE ` + companyWeeklyAdminTypesSQL + SQLRecurrenceWorkUnit + adminSQL + `
		  AND t.status NOT IN ('complete','cancelled')
		  AND TRIM(COALESCE(` + d + `,'')) != ''
		  AND ` + d + ` >= ? AND ` + d + ` < ?`
	return r.scanWeeklyStatTaskRows(qn, false, q, append(append([]interface{}{}, adminArgs...), p.PrevFrom, p.PrevToEx), add)
}

func (r *StatsRepo) fillWeeklyStatTaskSites(taskIDs []string, qn *int, items []model.CompanyWeeklyStatItem) error {
	if len(taskIDs) == 0 {
		return nil
	}
	ph := strings.Repeat("?,", len(taskIDs))
	ph = ph[:len(ph)-1]
	args := make([]interface{}, len(taskIDs))
	for i, id := range taskIDs {
		args[i] = id
	}
	rows, err := r.weeklyStatQuery(qn, `
		SELECT wts.task_id, COALESCE(NULLIF(TRIM(c.org_name),''), wts.customer_id)
		FROM work_task_sites wts
		LEFT JOIN customers c ON c.customer_id = wts.customer_id
		WHERE wts.task_id IN (`+ph+`)
		ORDER BY 2`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	by := map[string][]string{}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return err
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		by[id] = append(by[id], name)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range items {
		if items[i].BoxKey != weeklyStatBoxWork && items[i].BoxKey != weeklyStatBoxAdmin && items[i].BoxKey != weeklyStatBoxSales {
			continue
		}
		names := by[items[i].ID]
		if len(names) == 0 {
			continue
		}
		items[i].Site = strings.Join(names, ", ")
	}
	return nil
}

// GroupCompanyWeeklyStatBoxes 저장된 project_id(95-145 현행 귀속)로 카드에 붙인다. 통칭 행은 미연결.
func GroupCompanyWeeklyStatBoxes(rows []model.CompanyWeeklyRow, items []model.CompanyWeeklyStatItem) map[string][]model.CompanyWeeklyStatBox {
	byPID := map[string][]model.CompanyWeeklyStatItem{}
	var unlinked []model.CompanyWeeklyStatItem
	for _, it := range items {
		pid := strings.TrimSpace(it.ProjectID)
		if pid == "" {
			unlinked = append(unlinked, it)
			continue
		}
		byPID[pid] = append(byPID[pid], it)
	}
	out := map[string][]model.CompanyWeeklyStatBox{}
	for _, row := range rows {
		if !weeklyStatCardRow(row) {
			continue
		}
		var bag []model.CompanyWeeklyStatItem
		if activityKindOf(weeklyReportRow{Kind: row.RowKind, ActivityKind: row.ActivityKind}) == "unlinked" {
			bag = unlinked
		} else if pid := strings.TrimSpace(row.ProjectID); pid != "" {
			bag = byPID[pid]
		}
		out[row.RowKey] = buildCompanyWeeklyStatBoxes(bag)
	}
	return out
}

func weeklyStatCardRow(row model.CompanyWeeklyRow) bool {
	if row.RowKind == "project" {
		return true
	}
	return activityKindOf(weeklyReportRow{Kind: row.RowKind, ActivityKind: row.ActivityKind}) == "unlinked"
}

func buildCompanyWeeklyStatBoxes(items []model.CompanyWeeklyStatItem) []model.CompanyWeeklyStatBox {
	out := emptyCompanyWeeklyStatBoxes()
	idx := map[string]int{}
	for i, b := range out {
		idx[b.Key] = i
	}
	for _, it := range items {
		i, ok := idx[it.BoxKey]
		if !ok {
			continue
		}
		out[i].Items = append(out[i].Items, it)
		out[i].Total++
		if it.Done {
			out[i].Done++
		}
	}
	for i := range out {
		out[i].Rate = model.SiteCheckRateLabel(out[i].Done, out[i].Total)
	}
	return out
}

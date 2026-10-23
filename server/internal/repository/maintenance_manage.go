package repository

import (
	"fmt"
	"strings"

	"customer-support/internal/model"
)

// PlanListItem 관리 화면용 연도 계획 행 (§23.10).
type PlanListItem struct {
	model.MaintenancePlan
	VisitCount int
	DoneCount  int
}

func (r *MaintenanceRepo) ListPlansWithCounts() ([]PlanListItem, error) {
	return r.ListPlansWithCountsInOrg(OrgAll)
}

func (r *MaintenanceRepo) ListPlansWithCountsInOrg(orgID string) ([]PlanListItem, error) {
	q := `
		SELECT p.plan_id, p.plan_year, COALESCE(p.title,''), p.status,
		       IFNULL(p.created_at,''), IFNULL(p.updated_at,''), COALESCE(p.org_id,''),
		       COUNT(CASE WHEN length(trim(COALESCE(v.visit_date,''))) >= 10 THEN 1 END),
		       COUNT(CASE WHEN COALESCE(v.completed,0)=1 AND length(trim(COALESCE(v.visit_date,''))) >= 10 THEN 1 END)
		FROM maintenance_plans p
		LEFT JOIN maintenance_visits v ON v.plan_id = p.plan_id
		WHERE 1=1`
	q, args := appendOrg(q, nil, "p", orgIDForFilter(orgID))
	q += `
		GROUP BY p.plan_id
		ORDER BY p.plan_year DESC`
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlanListItem
	for rows.Next() {
		var it PlanListItem
		if err := rows.Scan(&it.PlanID, &it.PlanYear, &it.Title, &it.Status,
			&it.CreatedAt, &it.UpdatedAt, &it.OrgID, &it.VisitCount, &it.DoneCount); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// CopyPlan 새 연도 계획을 만들고 지난해 방문의 담당자·점검대상을 날짜 없이 넘긴다 (§23.10).
func (r *MaintenanceRepo) CopyPlan(sourcePlanID string, destYear int, title string) (*model.MaintenancePlan, error) {
	src, err := r.GetPlan(sourcePlanID)
	if err != nil || src == nil {
		return nil, fmt.Errorf("원본 계획을 찾을 수 없습니다")
	}
	if destYear < 2000 || destYear > 2100 {
		return nil, fmt.Errorf("연도를 입력하세요")
	}
	if destYear == src.PlanYear {
		return nil, fmt.Errorf("같은 연도로는 복사할 수 없습니다")
	}
	if strings.TrimSpace(src.OrgID) == "" {
		return nil, fmt.Errorf("조직이 없는 계획은 복사할 수 없습니다")
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = fmt.Sprintf("%d년 정기점검", destYear)
	}
	dest, err := r.CreatePlanForOrg(src.OrgID, destYear, title)
	if err != nil {
		return nil, err
	}
	visits, err := r.ListVisits(src.PlanID)
	if err != nil {
		return dest, err
	}
	type key struct{ cust, product string }
	best := map[key]model.MaintenanceVisit{}
	for _, v := range visits {
		if len(strings.TrimSpace(v.VisitDate)) < 10 {
			continue
		}
		k := key{v.CustomerID, strings.TrimSpace(v.ProductType)}
		cur, ok := best[k]
		if !ok || v.VisitDate > cur.VisitDate {
			best[k] = v
		}
	}
	for _, v := range best {
		tpl := model.MaintenanceVisit{
			PlanID: dest.PlanID, VisitDate: "", CustomerID: v.CustomerID,
			ProductType: v.ProductType, Assignee: v.Assignee,
			EntryCategory: v.EntryCategory, ProjectID: v.ProjectID, Notes: "복사(날짜 미정)",
		}
		if err := r.InsertVisitFull(tpl); err != nil {
			return dest, err
		}
	}
	r.TouchPlanUpdated(dest.PlanID)
	return dest, nil
}

// VisitFilter 일괄 담당자·삭제 조건 (§23.10).
type VisitFilter struct {
	Month   int
	Region  string
	Product string
}

func (r *MaintenanceRepo) ListVisitsFiltered(planID string, year int, f VisitFilter) ([]model.MaintenanceVisit, error) {
	visits, err := r.ListVisits(planID)
	if err != nil {
		return nil, err
	}
	cfgs, _ := r.ListSiteConfigs()
	regionOf := map[string]string{}
	for _, c := range cfgs {
		regionOf[c.CustomerID] = strings.TrimSpace(c.Region)
	}
	cfgBy := map[string]model.MaintenanceSiteConfig{}
	for _, c := range cfgs {
		cfgBy[c.CustomerID] = c
	}
	wantRegion := strings.TrimSpace(f.Region)
	wantProd := strings.TrimSpace(f.Product)
	var out []model.MaintenanceVisit
	for _, v := range visits {
		date := strings.TrimSpace(v.VisitDate)
		if len(date) < 10 {
			continue
		}
		if f.Month >= 1 && f.Month <= 12 {
			prefix := model.MonthPrefix(year, f.Month)
			if len(date) < len(prefix) || date[:len(prefix)] != prefix {
				continue
			}
		}
		if wantRegion != "" && regionOf[v.CustomerID] != wantRegion {
			continue
		}
		if wantProd != "" {
			cat := cfgBy[v.CustomerID].Products
			if model.VisitProductSlotKeyFor(v.ProductType, cat) != model.VisitProductSlotKeyFor(wantProd, cat) {
				continue
			}
		}
		out = append(out, v)
	}
	return out, nil
}

func (r *MaintenanceRepo) BulkUpdateAssignees(planID string, year int, f VisitFilter, assignee string) (int, error) {
	assignee = strings.TrimSpace(assignee)
	if assignee == "" {
		return 0, fmt.Errorf("담당자를 선택하세요")
	}
	list, err := r.ListVisitsFiltered(planID, year, f)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, v := range list {
		if strings.TrimSpace(v.Assignee) == assignee {
			continue
		}
		v.Assignee = assignee
		if err := r.UpdateVisit(v); err != nil {
			return n, err
		}
		n++
	}
	if n > 0 {
		r.TouchPlanUpdated(planID)
	}
	return n, nil
}

// FindVisitBySlot 같은 계획·고객·제품·날짜의 방문 한 건. 방금 넣은 알림 대상. §42.5
func (r *MaintenanceRepo) FindVisitBySlot(planID, customerID, product, date string) *model.MaintenanceVisit {
	if r == nil {
		return nil
	}
	visits, err := r.ListVisits(planID)
	if err != nil {
		return nil
	}
	wantP := r.visitSlotKey(customerID, product)
	wantD := strings.TrimSpace(date)
	var found *model.MaintenanceVisit
	for i := range visits {
		v := visits[i]
		if v.CustomerID != customerID {
			continue
		}
		if strings.TrimSpace(v.VisitDate) != wantD {
			continue
		}
		if r.visitSlotKey(customerID, v.ProductType) != wantP {
			continue
		}
		found = &visits[i]
	}
	return found
}

// AssignSlot 미배정 슬롯에 날짜를 넣는다. 계획 복사로 날짜만 비운 행이 있으면 그 행을 쓴다.
func (r *MaintenanceRepo) AssignSlot(v model.MaintenanceVisit) error {
	_, err := r.AssignSlotID(v)
	return err
}

// AssignSlotID AssignSlot 과 같고 방문 ID를 돌려준다.
func (r *MaintenanceRepo) AssignSlotID(v model.MaintenanceVisit) (string, error) {
	v.VisitDate = strings.TrimSpace(v.VisitDate)
	v.CustomerID = strings.TrimSpace(v.CustomerID)
	if v.PlanID == "" || v.VisitDate == "" || v.CustomerID == "" {
		return "", fmt.Errorf("날짜와 고객을 선택하세요")
	}
	if v.EntryCategory == "" {
		v.EntryCategory = "normal"
	}
	if v.People == nil && strings.TrimSpace(v.Assignee) == "" {
		v.Assignee = r.AssigneeForVisit(v.PlanID, v.CustomerID, v.ProductType, v.VisitDate)
	}
	v.DupReason = strings.TrimSpace(v.DupReason)
	exceptID := ""
	if tpl := r.findUndatedVisit(v.PlanID, v.CustomerID, v.ProductType); tpl != nil {
		exceptID = tpl.VisitID
	}
	autoRound := v.RoundNo <= 0
	if autoRound {
		v.RoundNo = r.nextRoundNo(v.PlanID, v.CustomerID, v.ProductType, v.VisitDate, exceptID)
	}
	if err := r.guardVisitRound(v, exceptID, autoRound, false); err != nil {
		return "", err
	}
	if tpl := r.findUndatedVisit(v.PlanID, v.CustomerID, v.ProductType); tpl != nil {
		tpl.VisitDate = v.VisitDate
		if strings.TrimSpace(v.Assignee) != "" {
			tpl.Assignee = v.Assignee
		}
		if strings.TrimSpace(v.ProjectID) != "" {
			tpl.ProjectID = v.ProjectID
		}
		if strings.TrimSpace(v.WorkGroupID) != "" {
			tpl.WorkGroupID = v.WorkGroupID
		}
		if v.People != nil {
			tpl.People = v.People
		}
		if strings.TrimSpace(tpl.Notes) == "복사(날짜 미정)" {
			tpl.Notes = ""
		}
		tpl.DupReason = v.DupReason
		if err := r.UpdateVisit(*tpl); err != nil {
			return "", err
		}
		return tpl.VisitID, nil
	}
	return r.InsertVisitFullID(v)
}

func (r *MaintenanceRepo) findUndatedVisit(planID, customerID, product string) *model.MaintenanceVisit {
	visits, err := r.ListVisits(planID)
	if err != nil {
		return nil
	}
	want := model.VisitProductSlotKey(product)
	for i := range visits {
		v := visits[i]
		if v.CustomerID != customerID {
			continue
		}
		if len(strings.TrimSpace(v.VisitDate)) >= 10 {
			continue
		}
		if model.VisitProductSlotKey(v.ProductType) != want {
			continue
		}
		return &v
	}
	return nil
}

type visitAssigneeRow struct {
	CustomerID  string
	ProductType string
	VisitDate   string
	Assignee    string
}

func assigneeIndexKey(customerID, product string) string {
	return strings.TrimSpace(customerID) + "|" + model.VisitProductSlotKey(product)
}

func fillAssigneePicks(rows []visitAssigneeRow, dst map[string]string) {
	bestDate := map[string]string{}
	for _, v := range rows {
		a := strings.TrimSpace(v.Assignee)
		if a == "" {
			continue
		}
		k := assigneeIndexKey(v.CustomerID, v.ProductType)
		d := strings.TrimSpace(v.VisitDate)
		if d >= bestDate[k] {
			bestDate[k] = d
			dst[k] = a
		}
	}
}

func (r *MaintenanceRepo) listVisitAssigneeRows(planID string) ([]visitAssigneeRow, error) {
	planID = strings.TrimSpace(planID)
	if planID == "" {
		return nil, nil
	}
	rows, err := r.db.Query(`
		SELECT v.customer_id, COALESCE(v.product_type,''), COALESCE(v.visit_date,''), COALESCE(v.assignee,'')
		FROM maintenance_visits v
		LEFT JOIN maintenance_site_config cfg ON cfg.customer_id = v.customer_id
		WHERE v.plan_id = ?
		ORDER BY v.visit_date, v.sort_order, cfg.short_name`, planID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []visitAssigneeRow
	for rows.Next() {
		var row visitAssigneeRow
		if err := rows.Scan(&row.CustomerID, &row.ProductType, &row.VisitDate, &row.Assignee); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func visitAssigneeRowsFrom(list []model.MaintenanceVisit) []visitAssigneeRow {
	out := make([]visitAssigneeRow, len(list))
	for i, v := range list {
		out[i] = visitAssigneeRow{CustomerID: v.CustomerID, ProductType: v.ProductType, VisitDate: v.VisitDate, Assignee: v.Assignee}
	}
	return out
}

func (r *MaintenanceRepo) assigneeIndex(planID string) (func(customerID, product string) string, error) {
	here, err := r.listVisitAssigneeRows(planID)
	if err != nil {
		return func(string, string) string { return "" }, err
	}
	return r.assigneeIndexFromRows(planID, here)
}

func (r *MaintenanceRepo) assigneeIndexFromVisits(planID string, here []model.MaintenanceVisit) (func(customerID, product string) string, error) {
	return r.assigneeIndexFromRows(planID, visitAssigneeRowsFrom(here))
}

func (r *MaintenanceRepo) assigneeIndexFromRows(planID string, here []visitAssigneeRow) (func(customerID, product string) string, error) {
	idx := map[string]string{}
	empty := func(string, string) string { return "" }
	plan, err := r.GetPlan(planID)
	if err != nil || plan == nil {
		return empty, err
	}
	if prev, err := r.GetPlanByYear(plan.PlanYear - 1); err == nil && prev != nil {
		if rows, err := r.listVisitAssigneeRows(prev.PlanID); err == nil {
			fillAssigneePicks(rows, idx)
		}
	}
	fillAssigneePicks(here, idx)
	return func(customerID, product string) string {
		return idx[assigneeIndexKey(customerID, product)]
	}, nil
}

func pickAssigneeFromVisits(list []model.MaintenanceVisit, customerID, product string) string {
	want := model.VisitProductSlotKey(product)
	bestDate, best := "", ""
	for _, v := range list {
		if v.CustomerID != customerID {
			continue
		}
		if model.VisitProductSlotKey(v.ProductType) != want {
			continue
		}
		a := strings.TrimSpace(v.Assignee)
		if a == "" {
			continue
		}
		d := strings.TrimSpace(v.VisitDate)
		if d >= bestDate {
			bestDate, best = d, a
		}
	}
	return best
}

// lookupAssigneeSlow 슬롯마다 올해·전년 방문을 다시 읽는 수정 전 구현. 동등성 테스트용.
func (r *MaintenanceRepo) lookupAssigneeSlow(planID, customerID, product string) string {
	plan, err := r.GetPlan(planID)
	if err != nil || plan == nil {
		return ""
	}
	if here, err := r.ListVisits(planID); err == nil {
		if a := pickAssigneeFromVisits(here, customerID, product); a != "" {
			return a
		}
	}
	prev, err := r.GetPlanByYear(plan.PlanYear - 1)
	if err != nil || prev == nil {
		return ""
	}
	list, err := r.ListVisits(prev.PlanID)
	if err != nil {
		return ""
	}
	return pickAssigneeFromVisits(list, customerID, product)
}

func (r *MaintenanceRepo) lookupAssignee(planID, customerID, product string) string {
	lookup, err := r.assigneeIndex(planID)
	if err != nil || lookup == nil {
		return ""
	}
	return lookup(customerID, product)
}

// AssigneeForVisit 이전 담당자를 가져오되, 그날 연차인 사람은 비운다. §23.13.6
func (r *MaintenanceRepo) AssigneeForVisit(planID, customerID, product, date string) string {
	a := r.lookupAssignee(planID, customerID, product)
	if a == "" {
		return ""
	}
	if NewStaffLeaveRepo(r.db).IsNameOnLeave(a, date) {
		return ""
	}
	return a
}

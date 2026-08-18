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
	rows, err := r.db.Query(`
		SELECT p.plan_id, p.plan_year, COALESCE(p.title,''), p.status,
		       IFNULL(p.created_at,''), IFNULL(p.updated_at,''),
		       COUNT(CASE WHEN length(trim(COALESCE(v.visit_date,''))) >= 10 THEN 1 END),
		       COUNT(CASE WHEN COALESCE(v.completed,0)=1 AND length(trim(COALESCE(v.visit_date,''))) >= 10 THEN 1 END)
		FROM maintenance_plans p
		LEFT JOIN maintenance_visits v ON v.plan_id = p.plan_id
		GROUP BY p.plan_id
		ORDER BY p.plan_year DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlanListItem
	for rows.Next() {
		var it PlanListItem
		if err := rows.Scan(&it.PlanID, &it.PlanYear, &it.Title, &it.Status,
			&it.CreatedAt, &it.UpdatedAt, &it.VisitCount, &it.DoneCount); err != nil {
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
	title = strings.TrimSpace(title)
	if title == "" {
		title = fmt.Sprintf("%d년 정기점검", destYear)
	}
	dest, err := r.CreatePlan(destYear, title)
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
	wantRegion := strings.TrimSpace(f.Region)
	wantProd := strings.TrimSpace(f.Product)
	wantKey := ""
	if wantProd != "" {
		wantKey = model.VisitProductSlotKey(wantProd)
	}
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
		if wantKey != "" && model.VisitProductSlotKey(v.ProductType) != wantKey {
			continue
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

// AssignSlot 미배정 슬롯에 날짜를 넣는다. 계획 복사로 날짜만 비운 행이 있으면 그 행을 쓴다.
func (r *MaintenanceRepo) AssignSlot(v model.MaintenanceVisit) error {
	v.VisitDate = strings.TrimSpace(v.VisitDate)
	v.CustomerID = strings.TrimSpace(v.CustomerID)
	if v.PlanID == "" || v.VisitDate == "" || v.CustomerID == "" {
		return fmt.Errorf("날짜와 고객을 선택하세요")
	}
	if v.EntryCategory == "" {
		v.EntryCategory = "normal"
	}
	if strings.TrimSpace(v.Assignee) == "" {
		v.Assignee = r.AssigneeForVisit(v.PlanID, v.CustomerID, v.ProductType, v.VisitDate)
	}
	if tpl := r.findUndatedVisit(v.PlanID, v.CustomerID, v.ProductType); tpl != nil {
		tpl.VisitDate = v.VisitDate
		if strings.TrimSpace(v.Assignee) != "" {
			tpl.Assignee = v.Assignee
		}
		if strings.TrimSpace(v.ProjectID) != "" {
			tpl.ProjectID = v.ProjectID
		}
		if strings.TrimSpace(tpl.Notes) == "복사(날짜 미정)" {
			tpl.Notes = ""
		}
		return r.UpdateVisit(*tpl)
	}
	return r.InsertVisitFull(v)
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

func (r *MaintenanceRepo) lookupAssignee(planID, customerID, product string) string {
	plan, err := r.GetPlan(planID)
	if err != nil || plan == nil {
		return ""
	}
	want := model.VisitProductSlotKey(product)
	pick := func(list []model.MaintenanceVisit) string {
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
	if here, err := r.ListVisits(planID); err == nil {
		if a := pick(here); a != "" {
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
	return pick(list)
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

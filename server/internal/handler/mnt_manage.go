package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/auditlog"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func (h *MaintenanceHandler) manageErrRedirect(c echo.Context, planID, msg string) error {
	return c.Redirect(http.StatusSeeOther, "/maintenance/"+planID+"/manage?err="+url.QueryEscape(msg))
}

func (h *MaintenanceHandler) manageOKRedirect(c echo.Context, planID, msg string) error {
	return c.Redirect(http.StatusSeeOther, "/maintenance/"+planID+"/manage?ok="+url.QueryEscape(msg))
}

func parseVisitFilter(c echo.Context) repository.VisitFilter {
	month, _ := strconv.Atoi(firstNonEmpty(c.FormValue("month"), c.QueryParam("month")))
	return repository.VisitFilter{
		Month:   month,
		Region:  strings.TrimSpace(firstNonEmpty(c.FormValue("region"), c.QueryParam("region"))),
		Product: strings.TrimSpace(firstNonEmpty(c.FormValue("product_type"), c.QueryParam("product_type"))),
	}
}

// ManagePlan 정기점검 관리 화면 (§23.10).
func (h *MaintenanceHandler) ManagePlan(c echo.Context) error {
	if !canViewMaintenance(c) {
		return echo.ErrForbidden
	}
	plan, err := h.repo.GetPlan(c.Param("id"))
	if err != nil || plan == nil {
		return echo.NewHTTPError(http.StatusNotFound, "계획을 찾을 수 없습니다")
	}
	plans, err := h.repo.ListPlansWithCounts()
	if err != nil {
		return err
	}
	cfgs, _ := h.repo.ListSiteConfigs()
	regions := distinctRegions(cfgs)

	um, _ := strconv.Atoi(c.QueryParam("um"))
	if um < 1 || um > 12 {
		if plan.PlanYear == time.Now().Year() {
			um = int(time.Now().Month())
		} else {
			um = 1
		}
	}
	unassigned, _ := h.repo.ListUnassignedMonthSlots(plan.PlanID, plan.PlanYear, um)
	dupCount, _ := h.repo.CountDuplicateDrops(plan.PlanID, plan.PlanYear, 0)

	var assignees []model.User
	if h.userRepo != nil {
		assignees, _ = h.userRepo.ListAssignable()
	}

	var preview []model.MaintenanceVisit
	previewN, previewSkip := 0, 0
	if c.QueryParam("preview") == "delete" {
		f := parseVisitFilter(c)
		preview, _ = h.repo.ListVisitsFiltered(plan.PlanID, plan.PlanYear, f)
		now := time.Now()
		for _, v := range preview {
			if model.VisitDeleteProtected(v.VisitDate, now) {
				previewSkip++
			} else {
				previewN++
			}
		}
	}

	return c.Render(http.StatusOK, "maintenance/plan_manage.html", map[string]interface{}{
		"Title": fmt.Sprintf("정기점검 관리 %d년", plan.PlanYear), "Active": NavMaintenance,
		"Plan": plan, "Plans": plans, "IsAdmin": isAdminRole(c),
		"Regions": regions, "Assignees": assignees,
		"Unassigned": unassigned, "UnassignedMonth": um,
		"DupCount": dupCount,
		"FlashOK":  c.QueryParam("ok"), "FlashErr": c.QueryParam("err"),
		"Preview": preview, "PreviewN": previewN, "PreviewSkip": previewSkip,
		"FilterMonth": c.QueryParam("month"), "FilterRegion": c.QueryParam("region"),
		"FilterProduct": c.QueryParam("product_type"),
		"NewYear":       time.Now().Year() + 1, "CopyYear": plan.PlanYear + 1,
		"HolidayMissing": holidayMissingBanner(h.holidayRepo, plan.PlanYear),
	})
}

func distinctRegions(cfgs []model.MaintenanceSiteConfig) []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range cfgs {
		r := strings.TrimSpace(c.Region)
		if r == "" || seen[r] {
			continue
		}
		seen[r] = true
		out = append(out, r)
	}
	return out
}

func (h *MaintenanceHandler) CopyPlan(c echo.Context) error {
	srcID := c.Param("id")
	year, _ := strconv.Atoi(c.FormValue("plan_year"))
	title := strings.TrimSpace(c.FormValue("title"))
	dest, err := h.repo.CopyPlan(srcID, year, title)
	if err != nil {
		return h.manageErrRedirect(c, srcID, err.Error())
	}
	return c.Redirect(http.StatusSeeOther, "/maintenance/"+dest.PlanID+"/manage?ok="+url.QueryEscape(
		fmt.Sprintf("%d년 계획을 복사했습니다. 방문일은 비어 있고 담당자만 넘겼습니다.", dest.PlanYear)))
}

func (h *MaintenanceHandler) SetPlanStatus(c echo.Context) error {
	id := c.Param("id")
	status := strings.TrimSpace(c.FormValue("status"))
	if err := h.repo.SetPlanStatus(id, status); err != nil {
		return h.manageErrRedirect(c, id, err.Error())
	}
	label := map[string]string{"draft": "초안", "approved": "승인", "archived": "보관"}[status]
	if label == "" {
		label = status
	}
	return h.manageOKRedirect(c, id, "상태를 "+label+"으로 바꿨습니다")
}

func (h *MaintenanceHandler) BulkUpdateAssignees(c echo.Context) error {
	id := c.Param("id")
	plan, err := h.repo.GetPlan(id)
	if err != nil || plan == nil {
		return echo.NewHTTPError(http.StatusNotFound, "계획을 찾을 수 없습니다")
	}
	n, err := h.repo.BulkUpdateAssignees(id, plan.PlanYear, parseVisitFilter(c), c.FormValue("assignee"))
	if err != nil {
		return h.manageErrRedirect(c, id, err.Error())
	}
	return h.manageOKRedirect(c, id, fmt.Sprintf("담당자를 %d건 바꿨습니다", n))
}

func (h *MaintenanceHandler) BulkDeleteVisits(c echo.Context) error {
	id := c.Param("id")
	plan, err := h.repo.GetPlan(id)
	if err != nil || plan == nil {
		return echo.NewHTTPError(http.StatusNotFound, "계획을 찾을 수 없습니다")
	}
	f := parseVisitFilter(c)
	q := url.Values{}
	q.Set("preview", "delete")
	if f.Month > 0 {
		q.Set("month", strconv.Itoa(f.Month))
	}
	if f.Region != "" {
		q.Set("region", f.Region)
	}
	if f.Product != "" {
		q.Set("product_type", f.Product)
	}
	if c.FormValue("confirm") != "1" {
		return c.Redirect(http.StatusSeeOther, "/maintenance/"+id+"/manage?"+q.Encode())
	}
	list, err := h.repo.ListVisitsFiltered(id, plan.PlanYear, f)
	if err != nil {
		return h.manageErrRedirect(c, id, err.Error())
	}
	ids := make([]string, 0, len(list))
	for _, v := range list {
		ids = append(ids, v.VisitID)
	}
	deleted, skipped, err := h.repo.DeleteVisitIDs(ids)
	if err != nil {
		return h.manageErrRedirect(c, id, err.Error())
	}
	h.repo.TouchPlanUpdated(id)
	if deleted > 0 || skipped > 0 {
		accessLog(c, auditlog.Record{
			Action:      auditlog.ActionBulkDelete,
			TargetTable: "maintenance_visits",
			TargetID:    id,
			SubjectType: "maintenance_plan",
			SubjectID:   id,
			SubjectName: planConfirmTitle(plan),
			Detail:      fmt.Sprintf("방문 일괄삭제 %d건 (제외 %d건)", deleted, skipped),
			Reason:      accessReason(c, "방문 일괄삭제"),
			Result:      auditlog.ResultOK,
			BeforeJSON:  toJSON(map[string]interface{}{"visit_ids": ids, "count": len(ids)}),
		})
	}
	msg := fmt.Sprintf("방문 %d건을 삭제했습니다", deleted)
	if skipped > 0 {
		msg += fmt.Sprintf(" (당월·이전 %d건은 제외)", skipped)
	}
	return h.manageOKRedirect(c, id, msg)
}

func (h *MaintenanceHandler) AssignUnassignedSlots(c echo.Context) error {
	id := c.Param("id")
	_ = c.Request().ParseForm()
	custs := c.Request().PostForm["customer_id"]
	products := c.Request().PostForm["product_type"]
	dates := c.Request().PostForm["visit_date"]
	assignee := strings.TrimSpace(c.FormValue("assignee"))
	project := strings.TrimSpace(c.FormValue("project_id"))
	n := 0
	for i := range custs {
		date := ""
		if i < len(dates) {
			date = strings.TrimSpace(dates[i])
		}
		if date == "" || strings.TrimSpace(custs[i]) == "" {
			continue
		}
		product := ""
		if i < len(products) {
			product = products[i]
		}
		v := model.MaintenanceVisit{
			PlanID: id, VisitDate: date, CustomerID: strings.TrimSpace(custs[i]),
			ProductType: product, Assignee: assignee, ProjectID: project, EntryCategory: "normal",
		}
		if err := h.repo.AssignSlot(v); err != nil {
			return h.manageErrRedirect(c, id, err.Error())
		}
		n++
	}
	if h.wbRepo != nil && n > 0 {
		_, _ = h.wbRepo.EnsureMaintenanceTasks()
	}
	if n > 0 {
		h.repo.TouchPlanUpdated(id)
	}
	return h.manageOKRedirect(c, id, fmt.Sprintf("미배정 %d건에 방문일을 넣었습니다", n))
}

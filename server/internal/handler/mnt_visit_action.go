package handler

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

// VisitAction 정기점검 방문 조치 화면 — 해당 방문만 완료·비고 처리.
func (h *MaintenanceHandler) VisitAction(c echo.Context) error {
	vid := c.Param("visit_id")
	v, err := h.repo.GetVisit(vid)
	if err != nil || v == nil {
		return echo.ErrNotFound
	}
	assignees, _ := h.userRepo.ListAssignable()
	var projects []model.WorkProject
	if h.wbRepo != nil {
		projects, _ = h.wbRepo.ListProjects(true)
	}
	name := v.ShortName
	if name == "" {
		name = v.OrgName
	}
	back := strings.TrimSpace(c.QueryParam("back"))
	if back == "" {
		back = "/workboard/register"
	}
	return c.Render(http.StatusOK, "maintenance/visit_action.html", map[string]interface{}{
		"Title":     "정기점검 조치 · " + name,
		"Active":    NavWorkRegister,
		"Visit":     v,
		"SiteName":  name,
		"Assignees": assignees,
		"Projects":  projects,
		"CanEdit":   canEditMaintenanceSchedule(c),
		"FlashOK":   c.QueryParam("ok"),
		"FlashErr":  c.QueryParam("err"),
		"Today":     time.Now().Format("2006-01-02"),
		"BackURL":   back,
		"PlanHref":  mntVisitHref(*v),
	})
}

// UpdateVisitAction 방문 조치 저장(완료·실제방문일·비고·담당). 연간 목록으로 보내지 않는다.
func (h *MaintenanceHandler) UpdateVisitAction(c echo.Context) error {
	if !canEditMaintenanceSchedule(c) {
		return echo.ErrForbidden
	}
	vid := c.Param("visit_id")
	existing, err := h.repo.GetVisit(vid)
	if err != nil || existing == nil {
		return echo.ErrNotFound
	}
	v := *existing
	v.Assignee = strings.TrimSpace(c.FormValue("assignee"))
	v.Notes = strings.TrimSpace(c.FormValue("notes"))
	v.ProductType = strings.TrimSpace(c.FormValue("product_type"))
	v.ProjectID = strings.TrimSpace(c.FormValue("project_id"))
	if d := strings.TrimSpace(c.FormValue("visit_date")); d != "" {
		parsed, err := model.ParseAppDate(d)
		if err != nil {
			return err
		}
		if parsed != "" {
			v.VisitDate = parsed
		}
	}
	status := strings.TrimSpace(c.FormValue("status"))
	markDone := status == "done" || c.FormValue("completed") == "1" || c.FormValue("mark_done") == "1"
	if status == "open" {
		markDone = false
	}
	v.Completed = markDone
	if v.Completed {
		v.CompletedDate = strings.TrimSpace(c.FormValue("completed_date"))
		if v.CompletedDate == "" {
			v.CompletedDate = v.VisitDate
		}
	} else {
		v.CompletedDate = ""
	}
	if err := h.repo.UpdateVisit(v); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	h.syncVisitTask(vid, v.Completed)
	h.repo.TouchPlanUpdated(v.PlanID)

	back := strings.TrimSpace(c.FormValue("back"))
	loc := "/maintenance/visits/" + vid + "/action?ok=" + url.QueryEscape("저장했습니다")
	if back != "" {
		loc += "&back=" + url.QueryEscape(back)
	}
	return c.Redirect(http.StatusSeeOther, loc)
}

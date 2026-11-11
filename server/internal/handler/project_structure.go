package handler

import (
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/audit"
	"customer-support/internal/model"
	"customer-support/internal/repository"
	"customer-support/internal/service"
)

func (h *ProjectHandler) planContext(c echo.Context, write bool) (*model.WorkProject, error) {
	if !canViewProjects(c) {
		return nil, echo.ErrForbidden
	}
	p, err := h.loadVisibleProject(c, write)
	if err != nil {
		return nil, err
	}
	if write && !canManagePlans(c, h, p) {
		return nil, echo.ErrForbidden
	}
	return p, nil
}

func (h *ProjectHandler) attachStructure(c echo.Context, p *model.WorkProject, data map[string]interface{}) {
	if data == nil || p == nil || h.repo == nil {
		return
	}
	data["Structure"] = repository.StructureTree{}
	data["CanManagePlans"] = canManagePlans(c, h, p)
	biz := holidayBiz()
	today := time.Now().Format("2006-01-02")
	if v, ok := data["PhaseToday"].(string); ok && v != "" {
		today = v
	}
	which := strings.TrimSpace(c.QueryParam("report"))
	tree, err := h.repo.LoadStructurePeriod(p.ProjectID, today, which, biz)
	if err != nil {
		return
	}
	data["Structure"] = tree
	data["HasStructure"] = tree.Has
	data["ReportLines"] = model.BuildProjectProgressReport(tree.Report)
	if which != "week" && which != "month" {
		which = "today"
	}
	data["ReportWhich"] = which
	data["ChartSVG"] = template.HTML(tree.ChartSVG)
	names := []string{}
	if assigns, ok := data["Assignments"].([]model.ProjectAssignment); ok {
		for _, a := range assigns {
			if a.Duty == model.ProjectDutyExecution && strings.TrimSpace(a.UserName) != "" {
				names = append(names, a.UserName)
			}
		}
	}
	data["ExecNames"] = strings.Join(names, ", ")
	data["CanEditProject"] = canEditProjectPhase(c, h, p)
}

func holidayBiz() func(string) bool {
	cal := service.DefaultCalendar()
	return func(d string) bool {
		if cal != nil {
			return cal.IsWorkingDay(d)
		}
		return model.WeekendBiz(d)
	}
}

func (h *ProjectHandler) phaseRedirect(p *model.WorkProject, msg string) string {
	u := "/projects/" + url.PathEscape(p.ProjectID) + "#project-phases"
	if msg != "" {
		u = "/projects/" + url.PathEscape(p.ProjectID) + "?" + msg + "#project-phases"
	}
	return u
}

func (h *ProjectHandler) PackageCreate(c echo.Context) error {
	p, err := h.phaseContext(c, true)
	if err != nil {
		return err
	}
	id, err := h.repo.CreatePackage(c.Param("phaseID"), c.FormValue("name"), c.FormValue("track_mode"), 1)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "err="+url.QueryEscape(err.Error())))
	}
	audit.LogWithReason(audit.ActionCreate, "phase_packages", "package_id", id, c.FormValue("name"), "", "", "하부 업무")
	return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "ok=pkg"))
}

func (h *ProjectHandler) PlanCreate(c echo.Context) error {
	p, err := h.planContext(c, true)
	if err != nil {
		return err
	}
	qty := map[string]int{}
	_ = c.Request().ParseForm()
	for _, id := range c.Request().Form["site_id"] {
		if n, e := strconv.Atoi(c.FormValue("qty_" + id)); e == nil {
			qty[id] = n
		}
	}
	id, err := h.repo.CreatePlan(p.ProjectID, repository.PlanDraft{
		PackageID:   c.FormValue("package_id"),
		Name:        c.FormValue("name"),
		PlanStart:   c.FormValue("plan_start"),
		PlanEnd:     c.FormValue("plan_end"),
		Basis:       c.FormValue("progress_basis"),
		QtyUnit:     c.FormValue("qty_unit"),
		RegionLabel: c.FormValue("region_label"),
		NextNote:    c.FormValue("next_note"),
		SiteIDs:     c.Request().Form["site_id"],
		TargetQty:   qty,
		Actor:       currentUserID(c),
	})
	if err != nil {
		return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "err="+url.QueryEscape(err.Error())))
	}
	audit.LogWithReason(audit.ActionCreate, "package_plans", "plan_id", id, c.FormValue("name"), "", "", "작업 계획")
	return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "ok=plan"))
}

func (h *ProjectHandler) PlanStatus(c echo.Context) error {
	p, err := h.planContext(c, true)
	if err != nil {
		return err
	}
	if err := h.repo.SetPlanStatus(p.ProjectID, c.Param("planID"), c.FormValue("status"), currentUserID(c)); err != nil {
		return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "err="+url.QueryEscape(err.Error())))
	}
	audit.LogWithReason(audit.ActionUpdate, "package_plans", "plan_id", c.Param("planID"), c.FormValue("status"), "", "", "계획 상태")
	return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "ok=plan"))
}

func (h *ProjectHandler) PlanDelete(c echo.Context) error {
	p, err := h.planContext(c, true)
	if err != nil {
		return err
	}
	if err := h.repo.DeletePlan(p.ProjectID, c.Param("planID")); err != nil {
		return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "err="+url.QueryEscape(err.Error())))
	}
	audit.LogWithReason(audit.ActionDelete, "package_plans", "plan_id", c.Param("planID"), "", "", "", "계획 삭제 또는 비활성")
	return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "ok=plan"))
}

func (h *ProjectHandler) PlanMove(c echo.Context) error {
	p, err := h.planContext(c, true)
	if err != nil {
		return err
	}
	if err := h.repo.MovePlan(p.ProjectID, c.Param("planID"), c.FormValue("dir")); err != nil {
		return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "err="+url.QueryEscape(err.Error())))
	}
	return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, ""))
}

func (h *ProjectHandler) PlanMoveSites(c echo.Context) error {
	p, err := h.planContext(c, true)
	if err != nil {
		return err
	}
	_ = c.Request().ParseForm()
	if err := h.repo.MovePlanSites(p.ProjectID, c.Param("planID"), c.Request().Form["site_id"], currentUserID(c)); err != nil {
		return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "err="+url.QueryEscape(err.Error())))
	}
	audit.LogWithReason(audit.ActionUpdate, "package_plans", "plan_id", c.Param("planID"), "옮기기", "", "", "이 계획으로 옮기기")
	return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "ok=plan"))
}

func (h *ProjectHandler) PlanExcludeSite(c echo.Context) error {
	p, err := h.planContext(c, true)
	if err != nil {
		return err
	}
	confirm := c.FormValue("confirm_done") == "1"
	if err := h.repo.ExcludePlanSite(p.ProjectID, c.Param("planID"), c.FormValue("site_id"), c.FormValue("reason"), confirm); err != nil {
		return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "err="+url.QueryEscape(err.Error())))
	}
	audit.LogWithReason(audit.ActionUpdate, "package_sites", "plan_id", c.Param("planID"), c.FormValue("site_id"), "", c.FormValue("reason"), "사이트 제외")
	return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "ok=plan"))
}

func (h *ProjectHandler) PlanSites(c echo.Context) error {
	p, err := h.planContext(c, false)
	if err != nil {
		return err
	}
	offset, _ := strconv.Atoi(c.QueryParam("offset"))
	hits, total, err := h.repo.SearchPlanSites(p.ProjectID, c.QueryParam("package_id"), c.QueryParam("q"), c.QueryParam("region"), offset, 100)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"sites": hits, "total": total, "offset": offset})
}

func (h *ProjectHandler) OpenSites(c echo.Context) error {
	p, err := h.planContext(c, false)
	if err != nil {
		return err
	}
	add, _ := strconv.Atoi(c.QueryParam("add"))
	day := strings.TrimSpace(c.QueryParam("date"))
	if _, err := time.Parse("2006-01-02", day); err != nil {
		day = time.Now().Format("2006-01-02")
	}
	page, err := h.repo.ListOpenSites(p.ProjectID, c.QueryParam("package_id"), c.QueryParam("plan_id"), c.QueryParam("q"), c.QueryParam("region"), day, 100)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	page.Preview = planPreview(page.Name, page.Done, page.Total, add)
	return c.JSON(http.StatusOK, page)
}

func (h *ProjectHandler) MilestoneSeed(c echo.Context) error {
	p, err := h.planContext(c, true)
	if err != nil {
		return err
	}
	if err := h.repo.SeedMilestones(p.ProjectID, c.FormValue("package_id"), c.FormValue("plan_id")); err != nil {
		return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "err="+url.QueryEscape(err.Error())))
	}
	audit.LogWithReason(audit.ActionCreate, "package_milestones", "package_id", c.FormValue("package_id"), c.FormValue("plan_id"), "", "", "기본 마일스톤")
	return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "ok=milestone"))
}

func (h *ProjectHandler) MilestoneConfirm(c echo.Context) error {
	p, err := h.planContext(c, true)
	if err != nil {
		return err
	}
	on := c.FormValue("on") != "0"
	if err := h.repo.SetMilestoneConfirm(p.ProjectID, c.Param("mid"), currentUserID(c), c.FormValue("note"), on); err != nil {
		return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "err="+url.QueryEscape(err.Error())))
	}
	audit.LogWithReason(audit.ActionUpdate, "package_milestones", "milestone_id", c.Param("mid"), "", "", c.FormValue("note"), "마일스톤 확인")
	return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "ok=milestone"))
}

func planPreview(name string, done, total, add int) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "계획"
	}
	if add < 0 {
		add = 0
	}
	next := done + add
	if total > 0 && next > total {
		next = total
	}
	pct := "—"
	if total > 0 {
		pct = model.SiteCheckRateLabel(next, total)
	}
	frac := "—"
	if total > 0 {
		frac = strconv.Itoa(next) + "/" + strconv.Itoa(total)
	}
	return "오늘 " + strconv.Itoa(add) + "곳을 모두 완료하면 " + name + " " + frac + " (" + pct + ")"
}

func (h *ProjectHandler) StructureOptions(c echo.Context) error {
	p, err := h.planContext(c, false)
	if err != nil {
		return err
	}
	tree, err := h.repo.LoadStructure(p.ProjectID, time.Now().Format("2006-01-02"), holidayBiz())
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"has": tree.Has, "options": tree.Options, "unclassified": tree.Unclassified})
}

func (h *ProjectHandler) StructureCopy(c echo.Context) error {
	p, err := h.phaseContext(c, true)
	if err != nil {
		return err
	}
	src := strings.TrimSpace(c.FormValue("from_project_id"))
	if src == "" || src == p.ProjectID {
		return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "err="+url.QueryEscape("복사할 사업을 고르세요")))
	}
	other, oerr := h.repo.Get(src)
	if oerr != nil || other == nil {
		return echo.NewHTTPError(http.StatusNotFound, "사업 없음")
	}
	if _, err := h.repo.CopyStructure(p.ProjectID, src, p.OrgID); err != nil {
		return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "err="+url.QueryEscape(err.Error())))
	}
	audit.LogWithReason(audit.ActionCreate, "phase_packages", "project_id", p.ProjectID, src, "", "", "구조 복사")
	return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "ok=copy"))
}

func (h *WorkboardHandler) bindPostedPlan(c echo.Context, taskID, projectID string) error {
	if h == nil || h.repo == nil {
		return nil
	}
	pkg := strings.TrimSpace(c.FormValue("package_id"))
	plan := strings.TrimSpace(c.FormValue("plan_id"))
	if pkg == "" && plan == "" {
		return nil
	}
	return h.repo.SetTaskPlanLink(taskID, projectID, pkg, plan)
}

func (h *WorkboardHandler) SaveTaskPlanLink(c echo.Context) error {
	if !canWriteWorkboard(c) || isReadOnly(c) || currentRole(c) == model.RoleCLevel {
		return echo.ErrForbidden
	}
	t, err := h.repo.GetTask(c.Param("id"))
	if err != nil || t == nil {
		return echo.ErrNotFound
	}
	if err := h.bindPostedPlan(c, t.TaskID, firstNonEmpty(c.FormValue("project_id"), t.ProjectID)); err != nil {
		return c.Redirect(http.StatusSeeOther, "/workboard/tasks/"+t.TaskID+"?err="+url.QueryEscape(err.Error()))
	}
	return c.Redirect(http.StatusSeeOther, "/workboard/tasks/"+t.TaskID+"?ok=plan")
}

func attachPlanLink(data map[string]interface{}, repo *repository.WBRepo, projectID, taskID string) {
	if data == nil || repo == nil {
		return
	}
	if label := repo.TaskPlanLabel(taskID); label != "" {
		data["PlanLinkLabel"] = label
	}
	if repo.ProjectHasPackages(projectID) {
		data["StructurePanel"] = true
	}
}

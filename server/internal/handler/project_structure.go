package handler

import (
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"log"
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
	data["CanManageTemplates"] = canManageStructureTemplate(c) && canEditProjectPhase(c, h, p)
	data["KindOptions"] = model.ProjectKindOptions()
	if h.pe != nil {
		data["SiteCount"] = h.pe.CountActiveSites(p.ProjectID)
	}
	h.attachStructureTools(c, p, data)
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
	// 다른 사업의 단계에는 만들 수 없다.
	ph, err := h.repo.GetPhase(c.Param("phaseID"))
	if err != nil || ph == nil || ph.ProjectID != p.ProjectID {
		return echo.ErrNotFound
	}
	// 일자·배지(선택)는 만들기 전에 먼저 검사해 반쯤 만들어진 하부 업무가 남지 않게 한다. 95-179.
	meta := repository.PackageMeta{
		PlanStart: c.FormValue("plan_start"), PlanEnd: c.FormValue("plan_end"), Badge: c.FormValue("badge"),
	}
	if _, err := model.BadgeOK(meta.Badge); err != nil {
		return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "err="+url.QueryEscape(err.Error())))
	}
	if _, _, _, _, err := model.PhaseDatesOK(meta.PlanStart, meta.PlanEnd, "", ""); err != nil {
		return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "err="+url.QueryEscape(err.Error())))
	}
	id, err := h.repo.CreatePackage(ph.PhaseID, c.FormValue("name"), c.FormValue("track_mode"))
	if err != nil {
		return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "err="+url.QueryEscape(err.Error())))
	}
	if strings.TrimSpace(meta.PlanStart+meta.PlanEnd+meta.Badge) != "" {
		if err := h.repo.UpdatePackageMeta(p.ProjectID, id, meta, repository.DateChangeCtx{Actor: currentUserID(c)}); err != nil {
			return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "err="+url.QueryEscape(err.Error())))
		}
	}
	audit.LogWithReason(audit.ActionCreate, "phase_packages", "package_id", id, c.FormValue("name"), "", "", "하부 업무")
	return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "ok=pkg"))
}

// PackageUpdate 하부 업무의 예정·마감·실제 시작 일자와 배지. 마감 연장은 사유가 필수이고 이력에 남는다. 95-179.
func (h *ProjectHandler) PackageUpdate(c echo.Context) error {
	p, err := h.phaseContext(c, true)
	if err != nil {
		return err
	}
	pkgID := c.Param("pkgID")
	if _, _, err := h.repo.PackageMetaOf(p.ProjectID, pkgID); err != nil {
		return echo.ErrNotFound
	}
	meta := repository.PackageMeta{
		PlanStart: c.FormValue("plan_start"), PlanEnd: c.FormValue("plan_end"),
		ActualStart: c.FormValue("actual_start"), Badge: c.FormValue("badge"),
	}
	if err := h.repo.UpdatePackageMeta(p.ProjectID, pkgID, meta, repository.DateChangeCtx{Actor: currentUserID(c), Reason: c.FormValue("extend_reason")}); err != nil {
		return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "err="+url.QueryEscape(err.Error())))
	}
	audit.LogWithReason(audit.ActionUpdate, "phase_packages", "package_id", pkgID, "일자·배지", "", strings.TrimSpace(meta.PlanEnd), "하부 업무 일자")
	return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "ok=pkg-meta"))
}

// StructureChanges 사업의 마감일 변경(연장) 이력 「변경내역」. 95-179.
func (h *ProjectHandler) StructureChanges(c echo.Context) error {
	if !canViewProjects(c) {
		return echo.ErrForbidden
	}
	p, err := h.loadVisibleProject(c, false)
	if err != nil {
		return err
	}
	list, err := h.repo.ListDateChanges(p.ProjectID, 300)
	if err != nil {
		return err
	}
	return c.Render(http.StatusOK, "project/changes.html", map[string]interface{}{
		"Title": p.DisplayName() + " · 변경내역", "Active": NavProjects, "Project": p, "Changes": list,
	})
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

// PackageQtyTarget 수량만 하부 업무의 목표 수량·단위. 95-176 질문 1.
func (h *ProjectHandler) PackageQtyTarget(c echo.Context) error {
	p, err := h.planContext(c, true)
	if err != nil {
		return err
	}
	total, _ := strconv.Atoi(strings.TrimSpace(c.FormValue("qty_total")))
	if err := h.repo.SetPackageQtyTarget(p.ProjectID, c.Param("pkgID"), c.FormValue("qty_unit"), total); err != nil {
		return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "err="+url.QueryEscape(err.Error())))
	}
	audit.LogWithReason(audit.ActionUpdate, "phase_packages", "package_id", c.Param("pkgID"), "qty_total", "", strconv.Itoa(total), "수량만 목표")
	return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "ok=qty-target"))
}

// PackageQty 수량만 하부 업무의 처리 수량을 더한다(보정은 음수).
func (h *ProjectHandler) PackageQty(c echo.Context) error {
	p, err := h.planContext(c, true)
	if err != nil {
		return err
	}
	delta, convErr := strconv.Atoi(strings.TrimSpace(c.FormValue("qty")))
	if convErr != nil {
		return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "err="+url.QueryEscape("처리 수량은 정수로 입력하세요")))
	}
	done, err := h.repo.RecordPackageQty(p.ProjectID, c.Param("pkgID"), c.FormValue("date"), delta, currentUserID(c), c.FormValue("note"))
	if err != nil {
		return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "err="+url.QueryEscape(err.Error())))
	}
	audit.LogWithReason(audit.ActionUpdate, "package_events", "package_id", c.Param("pkgID"), "qty_delta", "", strconv.Itoa(delta), fmt.Sprintf("수량만 처리 수량 누적 %d", done))
	return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "ok=qty"))
}

// PlanUpdate 작업 계획의 이름·예정·마감·다음 할 일 수정. 마감 연장은 사유 필수, 이력은 「변경내역」에 남는다. 95-180.
func (h *ProjectHandler) PlanUpdate(c echo.Context) error {
	p, err := h.planContext(c, true)
	if err != nil {
		return err
	}
	e := repository.PlanEdit{
		Name: c.FormValue("name"), PlanStart: c.FormValue("plan_start"), PlanEnd: c.FormValue("plan_end"),
		NextNote: c.FormValue("next_note"),
	}
	planID := c.Param("planID")
	if err := h.repo.UpdatePlan(p.ProjectID, planID, e, repository.DateChangeCtx{Actor: currentUserID(c), Reason: c.FormValue("extend_reason")}); err != nil {
		return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "err="+url.QueryEscape(err.Error())))
	}
	audit.LogWithReason(audit.ActionUpdate, "package_plans", "plan_id", planID, e.Name, "", strings.TrimSpace(e.PlanEnd), "작업 계획 수정")
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
	srcID := strings.TrimSpace(c.FormValue("from_project_id"))
	if srcID == "" || srcID == p.ProjectID {
		return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "err="+url.QueryEscape("복사할 사업을 고르세요")))
	}
	if _, err := h.lookupCopySource(c, srcID); err != nil {
		return err
	}
	if c.FormValue("confirm") != "1" {
		return c.Redirect(http.StatusSeeOther, "/projects/"+url.PathEscape(p.ProjectID)+"?copy_from="+url.QueryEscape(srcID)+"#project-phases")
	}
	stat, err := h.repo.CopyStructure(p.ProjectID, srcID, p.OrgID)
	if err != nil {
		if errors.Is(err, repository.ErrNoStructureToCopy) {
			return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "err="+url.QueryEscape(err.Error())))
		}
		return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "err="+url.QueryEscape(err.Error())))
	}
	audit.LogWithReason(audit.ActionCreate, "phase_packages", "project_id", p.ProjectID, srcID, "",
		fmt.Sprintf("단계 %d 하부 %d 건너뜀 %d", stat.PhasesAdded, stat.PackagesAdded, stat.PhasesSkipped+stat.PackagesSkipped),
		"구조 복사")
	return c.Redirect(http.StatusSeeOther, "/projects/"+url.PathEscape(p.ProjectID)+
		"?ok=copy&ph="+strconv.Itoa(stat.PhasesAdded)+"&pk="+strconv.Itoa(stat.PackagesAdded)+
		"&sk="+strconv.Itoa(stat.PhasesSkipped+stat.PackagesSkipped)+"#project-phases")
}

func (h *ProjectHandler) lookupCopySource(c echo.Context, srcID string) (*model.WorkProject, error) {
	other, err := h.repo.GetVisible(srcID, currentOrg(c), false)
	if err != nil || other == nil {
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			log.Printf("structure copy source: %v", err)
		}
		return nil, echo.NewHTTPError(http.StatusNotFound, "사업을 찾을 수 없습니다")
	}
	return other, nil
}

func (h *ProjectHandler) attachStructureTools(c echo.Context, p *model.WorkProject, data map[string]interface{}) {
	if data == nil || p == nil || h.repo == nil || !canEditProjectPhase(c, h, p) {
		return
	}
	q := strings.TrimSpace(c.QueryParam("copy_q"))
	data["CopyQ"] = q
	if q != "" {
		hits, err := h.repo.SearchStructureSources(p.ProjectID, q)
		if err == nil {
			data["CopyHits"] = hits
		}
	}
	if from := strings.TrimSpace(c.QueryParam("copy_from")); from != "" && from != p.ProjectID {
		if _, err := h.lookupCopySource(c, from); err != nil {
			data["FlashErr"] = "사업을 찾을 수 없습니다"
		} else if plan, err := h.repo.PreviewStructureCopy(p.ProjectID, from); err != nil {
			if errors.Is(err, repository.ErrNoStructureToCopy) {
				data["CopyEmpty"] = true
			}
		} else {
			data["CopyFrom"] = from
			data["CopyPreview"] = plan
		}
	}
	active, _ := h.repo.ListStructureTemplates(true)
	data["StructureTemplates"] = sortTemplates(active, p.ProjectKind)
	if canManageStructureTemplate(c) {
		all, _ := h.repo.ListStructureTemplates(false)
		data["ManageTemplates"] = all
	}
	if id := strings.TrimSpace(c.QueryParam("tpl")); id != "" {
		plan, err := h.repo.PreviewTemplateApply(p.ProjectID, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				data["FlashErr"] = "템플릿을 찾을 수 없습니다"
			} else if errors.Is(err, repository.ErrNoStructureToCopy) {
				data["TplEmpty"] = true
			}
		} else {
			data["TplFrom"] = id
			data["TplPreview"] = plan
		}
	}
	if c.QueryParam("ok") == "copy" {
		data["CopyDone"] = true
		data["CopyPhaseN"] = c.QueryParam("ph")
		data["CopyPkgN"] = c.QueryParam("pk")
		data["CopySkipN"] = c.QueryParam("sk")
	}
	if c.QueryParam("ok") == "tpl" {
		data["TplDone"] = true
		data["CopyPhaseN"] = c.QueryParam("ph")
		data["CopyPkgN"] = c.QueryParam("pk")
		data["CopySkipN"] = c.QueryParam("sk")
	}
}

func sortTemplates(list []repository.StructureTemplate, kind string) []repository.StructureTemplate {
	kind = strings.TrimSpace(kind)
	if kind == "" || len(list) < 2 {
		return list
	}
	var same, rest []repository.StructureTemplate
	for _, t := range list {
		if t.Kind == kind {
			same = append(same, t)
		} else {
			rest = append(rest, t)
		}
	}
	return append(same, rest...)
}

func (h *ProjectHandler) StructureSources(c echo.Context) error {
	p, err := h.phaseContext(c, true)
	if err != nil {
		return err
	}
	hits, err := h.repo.SearchStructureSources(p.ProjectID, c.QueryParam("q"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"items": hits})
}

func (h *ProjectHandler) StructureCopyPreview(c echo.Context) error {
	p, err := h.phaseContext(c, true)
	if err != nil {
		return err
	}
	from := strings.TrimSpace(c.QueryParam("from"))
	if from == "" || from == p.ProjectID {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "복사할 사업을 고르세요"})
	}
	if _, err := h.lookupCopySource(c, from); err != nil {
		return err
	}
	plan, err := h.repo.PreviewStructureCopy(p.ProjectID, from)
	if err != nil {
		if errors.Is(err, repository.ErrNoStructureToCopy) {
			return c.JSON(http.StatusOK, map[string]interface{}{"empty": true, "message": err.Error()})
		}
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusOK, plan)
}

func (h *ProjectHandler) StructureTemplateSave(c echo.Context) error {
	p, err := h.phaseContext(c, true)
	if err != nil {
		return err
	}
	if !canManageStructureTemplate(c) {
		return echo.ErrForbidden
	}
	id, err := h.repo.SaveStructureTemplate(p.ProjectID, p.OrgID, c.FormValue("name"), c.FormValue("project_kind"), c.FormValue("note"), currentUserID(c))
	if err != nil {
		return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "err="+url.QueryEscape(err.Error())))
	}
	audit.LogWithReason(audit.ActionCreate, "structure_templates", "template_id", id, c.FormValue("name"), p.ProjectID, "", "템플릿 저장")
	return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "ok=tpl-save"))
}

func (h *ProjectHandler) StructureTemplateApply(c echo.Context) error {
	p, err := h.phaseContext(c, true)
	if err != nil {
		return err
	}
	id := strings.TrimSpace(c.FormValue("template_id"))
	if id == "" {
		return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "err="+url.QueryEscape("템플릿을 고르세요")))
	}
	if c.FormValue("confirm") != "1" {
		return c.Redirect(http.StatusSeeOther, "/projects/"+url.PathEscape(p.ProjectID)+"?tpl="+url.QueryEscape(id)+"#project-phases")
	}
	stat, err := h.repo.ApplyStructureTemplate(p.ProjectID, p.OrgID, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "템플릿을 찾을 수 없습니다")
		}
		return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "err="+url.QueryEscape(err.Error())))
	}
	audit.LogWithReason(audit.ActionCreate, "phase_packages", "project_id", p.ProjectID, id, "",
		fmt.Sprintf("단계 %d 하부 %d", stat.PhasesAdded, stat.PackagesAdded), "템플릿 적용")
	return c.Redirect(http.StatusSeeOther, "/projects/"+url.PathEscape(p.ProjectID)+
		"?ok=tpl&ph="+strconv.Itoa(stat.PhasesAdded)+"&pk="+strconv.Itoa(stat.PackagesAdded)+
		"&sk="+strconv.Itoa(stat.PhasesSkipped+stat.PackagesSkipped)+"#project-phases")
}

func (h *ProjectHandler) StructureTemplateRename(c echo.Context) error {
	p, err := h.phaseContext(c, true)
	if err != nil {
		return err
	}
	if !canManageStructureTemplate(c) {
		return echo.ErrForbidden
	}
	id := strings.TrimSpace(c.FormValue("template_id"))
	if err := h.repo.RenameStructureTemplate(id, c.FormValue("name")); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "템플릿을 찾을 수 없습니다")
		}
		return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "err="+url.QueryEscape(err.Error())))
	}
	audit.LogWithReason(audit.ActionUpdate, "structure_templates", "template_id", id, c.FormValue("name"), "", "", "템플릿 이름")
	return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "ok=tpl-name"))
}

func (h *ProjectHandler) StructureTemplateOff(c echo.Context) error {
	p, err := h.phaseContext(c, true)
	if err != nil {
		return err
	}
	if !canManageStructureTemplate(c) {
		return echo.ErrForbidden
	}
	id := strings.TrimSpace(c.FormValue("template_id"))
	if err := h.repo.DeactivateStructureTemplate(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "템플릿을 찾을 수 없습니다")
		}
		return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "err="+url.QueryEscape(err.Error())))
	}
	audit.LogWithReason(audit.ActionUpdate, "structure_templates", "template_id", id, "", "", "", "템플릿 비활성")
	return c.Redirect(http.StatusSeeOther, h.phaseRedirect(p, "ok=tpl-off"))
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

// checkPostedPackageSites 업무를 만들기 전에 호출한다. 95-182.
//   - 수행 구조(사이트형 하부 업무)가 있는 사업이면 하부 업무를 반드시 골라야 한다.
//   - 하부 업무와 사이트를 함께 보냈다면, 사이트는 그 하부 업무에 등록된 곳만 허용한다.
func (h *WorkboardHandler) checkPostedPackageSites(c echo.Context, projectID string) error {
	projectID = strings.TrimSpace(projectID)
	if h == nil || h.repo == nil || projectID == "" {
		return nil
	}
	pr := repository.NewProjectRepo(h.repo.DB())
	pkg := strings.TrimSpace(c.FormValue("package_id"))
	if pkg == "" && strings.TrimSpace(c.FormValue("plan_id")) == "" {
		if list, err := pr.PackageChoices(projectID); err == nil && len(list) > 0 {
			return fmt.Errorf("수행 구조가 있는 사업입니다. 하부 업무를 선택하세요")
		}
		return nil
	}
	if pkg == "" || strings.TrimSpace(c.FormValue("plan_id")) != "" {
		return nil
	}
	_ = c.Request().ParseForm()
	sites := c.Request().Form["site_id"]
	if len(sites) == 0 {
		return nil
	}
	return pr.PackageSitesAllowed(projectID, pkg, sites)
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

// workPackagePerm 작업·행정/지원 등록 화면의 하부 업무·사이트 조회 권한(대상 사이트 검색과 같은 규칙).
func workPackagePerm(c echo.Context) error {
	if !hasPerm(c, model.PermWorkboard) {
		return echo.ErrForbidden
	}
	if currentRole(c) == model.RoleCLevel || isReadOnly(c) {
		return echo.ErrForbidden
	}
	return nil
}

// ProjectPackageChoices GET /workboard/project-packages?project_id= — 수행 구조가 있는 사업의 하부 업무 선택지. 없으면 has=false. 95-182.
func (h *WorkboardHandler) ProjectPackageChoices(c echo.Context) error {
	if err := workPackagePerm(c); err != nil {
		return err
	}
	pid := strings.TrimSpace(c.QueryParam("project_id"))
	if pid == "" {
		return c.JSON(http.StatusOK, map[string]interface{}{"has": false, "packages": []repository.PackageChoice{}})
	}
	if _, err := h.repo.ProjectOrgID(pid); err != nil {
		return echo.ErrNotFound
	}
	list, err := repository.NewProjectRepo(h.repo.DB()).PackageChoices(pid)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"has": len(list) > 0, "packages": list})
}

// ProjectPackageSites GET /workboard/package-sites?project_id=&package_id= — 그 하부 업무에 등록된 사이트와 시도·시군. 95-182.
func (h *WorkboardHandler) ProjectPackageSites(c echo.Context) error {
	if err := workPackagePerm(c); err != nil {
		return err
	}
	pid, pkg := strings.TrimSpace(c.QueryParam("project_id")), strings.TrimSpace(c.QueryParam("package_id"))
	if _, err := h.repo.ProjectOrgID(pid); err != nil || pkg == "" {
		return echo.ErrNotFound
	}
	res, err := repository.NewProjectRepo(h.repo.DB()).PackageSiteChoices(pid, pkg)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, res)
}

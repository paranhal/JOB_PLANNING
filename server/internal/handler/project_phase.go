package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/audit"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func canEditProjectPhase(c echo.Context, h *ProjectHandler, p *model.WorkProject) bool {
	if p == nil || viewAsActive(c) || currentRole(c) == model.RoleObserver {
		return false
	}
	return canDesignProjectWork(c, h, p, p.OrgID)
}

func canManageStructureTemplate(c echo.Context) bool {
	if isReadOnly(c) || viewAsActive(c) || currentRole(c) == model.RoleObserver || currentRole(c) == model.RoleCLevel {
		return false
	}
	switch model.NormalizeRole(currentRole(c)) {
	case model.RoleVisionAdmin, model.RoleOrgAdmin:
		return true
	default:
		return false
	}
}

// canManagePlans 구조 수정 권한에 수행 담당을 더한 계획 전용 권한.
func canManagePlans(c echo.Context, h *ProjectHandler, p *model.WorkProject) bool {
	if p == nil || isReadOnly(c) || viewAsActive(c) || currentRole(c) == model.RoleCLevel || currentRole(c) == model.RoleObserver {
		return false
	}
	if canDesignProjectWork(c, h, p, p.OrgID) {
		return true
	}
	if h == nil || h.pe == nil {
		return false
	}
	return h.pe.AssignedDuty(p.ProjectID, currentUserID(c), model.ProjectDutyExecution)
}

func (h *ProjectHandler) phaseContext(c echo.Context, write bool) (*model.WorkProject, error) {
	if !canViewProjects(c) {
		return nil, echo.ErrForbidden
	}
	p, err := h.loadVisibleProject(c, write)
	if err != nil {
		return nil, err
	}
	if write && !canEditProjectPhase(c, h, p) {
		return nil, echo.ErrForbidden
	}
	return p, nil
}

func (h *ProjectHandler) attachPhases(c echo.Context, p *model.WorkProject, data map[string]interface{}) {
	if data == nil || p == nil {
		return
	}
	list, _ := h.repo.ListPhases(p.ProjectID)
	today := time.Now().Format("2006-01-02")
	data["Phases"] = list
	data["PhaseSummary"] = model.SummarizePhases(list, today)
	data["CanEditPhase"] = canEditProjectPhase(c, h, p)
	data["PhaseOpen"] = model.PhaseSectionOpen(p.ProjectKind)
	data["PhaseHasTemplate"] = model.PhaseTemplateGroup(p.ProjectKind) != ""
	hint := ""
	if len(list) == 0 {
		hint = model.PhaseEmptyHint(p.ProjectKind)
	}
	data["PhaseHint"] = hint
	data["PhaseToday"] = today
	if c.QueryParam("preview") == "1" && canEditProjectPhase(c, h, p) {
		data["PhasePreview"] = h.phaseDrafts(p, list)
	}
	if id := strings.TrimSpace(c.QueryParam("phase_confirm")); id != "" {
		data["PhaseConfirmID"] = id
		data["PhasePriorN"] = c.QueryParam("prior")
	}
	h.attachStructure(c, p, data)
}

type phaseDraft struct {
	Name      string
	PlanStart string
	PlanEnd   string
}

func (h *ProjectHandler) phaseDrafts(p *model.WorkProject, existing []model.ProjectPhase) []phaseDraft {
	group := model.PhaseTemplateGroup(p.ProjectKind)
	if group == "" || h.codeRepo == nil {
		return nil
	}
	codes, err := h.codeRepo.ActiveByGroup(group)
	if err != nil {
		return nil
	}
	have := map[string]bool{}
	for _, ph := range existing {
		have[strings.TrimSpace(ph.Name)] = true
	}
	var names []string
	for _, code := range codes {
		name := strings.TrimSpace(code.CodeName)
		if name == "" {
			name = strings.TrimSpace(code.CodeValue)
		}
		if name == "" || have[name] {
			continue
		}
		names = append(names, name)
	}
	spans := model.SplitPhaseDates(p.StartDate, p.EndDate, len(names))
	out := make([]phaseDraft, len(names))
	for i, name := range names {
		out[i].Name = name
		if i < len(spans) {
			out[i].PlanStart = spans[i].Start
			out[i].PlanEnd = spans[i].End
		}
	}
	return out
}

func (h *ProjectHandler) PhasePreview(c echo.Context) error {
	p, err := h.phaseContext(c, true)
	if err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/projects/"+p.ProjectID+"?preview=1#project-phases")
}

func (h *ProjectHandler) PhaseCreate(c echo.Context) error {
	p, err := h.phaseContext(c, true)
	if err != nil {
		return err
	}
	row := &model.ProjectPhase{
		ProjectID: p.ProjectID, OrgID: p.OrgID, Name: c.FormValue("name"),
		PlanStart: c.FormValue("plan_start"), PlanEnd: c.FormValue("plan_end"),
		Note: c.FormValue("note"), Status: model.PhasePlanned, IsActive: true,
	}
	if err := h.repo.CreatePhase(row); err != nil {
		return c.Redirect(http.StatusSeeOther, "/projects/"+p.ProjectID+"?err="+err.Error()+"#project-phases")
	}
	h.logPhase(audit.ActionCreate, row, "단계 추가")
	return c.Redirect(http.StatusSeeOther, "/projects/"+p.ProjectID+"?ok=phase#project-phases")
}

func (h *ProjectHandler) PhaseUpdate(c echo.Context) error {
	p, err := h.phaseContext(c, true)
	if err != nil {
		return err
	}
	cur, err := h.repo.GetPhase(c.Param("phaseID"))
	if err != nil || cur == nil || cur.ProjectID != p.ProjectID {
		return echo.ErrNotFound
	}
	cur.Name = c.FormValue("name")
	cur.PlanStart = c.FormValue("plan_start")
	cur.PlanEnd = c.FormValue("plan_end")
	cur.ActualStart = c.FormValue("actual_start")
	cur.ActualEnd = c.FormValue("actual_end")
	cur.Note = c.FormValue("note")
	if st := strings.TrimSpace(c.FormValue("status")); st != "" {
		cur.Status = st
	}
	cur.IsActive = c.FormValue("is_active") != "0"
	if err := h.repo.UpdatePhase(cur); err != nil {
		return c.Redirect(http.StatusSeeOther, "/projects/"+p.ProjectID+"?err="+err.Error()+"#project-phases")
	}
	h.logPhase(audit.ActionUpdate, cur, "단계 수정")
	return c.Redirect(http.StatusSeeOther, "/projects/"+p.ProjectID+"?ok=phase#project-phases")
}

func (h *ProjectHandler) PhaseStatus(c echo.Context) error {
	p, err := h.phaseContext(c, true)
	if err != nil {
		return err
	}
	id := c.Param("phaseID")
	cur, err := h.repo.GetPhase(id)
	if err != nil || cur == nil || cur.ProjectID != p.ProjectID {
		return echo.ErrNotFound
	}
	status := strings.TrimSpace(c.FormValue("status"))
	confirm := c.FormValue("confirm_prior") == "1"
	today := time.Now().Format("2006-01-02")
	n, err := h.repo.SetPhaseStatus(id, status, today, confirm)
	if err == repository.ErrPhasePriorConfirm {
		return c.Redirect(http.StatusSeeOther, "/projects/"+p.ProjectID+"?phase_confirm="+id+"&prior="+strconv.Itoa(n)+"#project-phases")
	}
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/projects/"+p.ProjectID+"?err="+err.Error()+"#project-phases")
	}
	h.logPhase(audit.ActionUpdate, cur, "단계 상태")
	return c.Redirect(http.StatusSeeOther, "/projects/"+p.ProjectID+"?ok=phase#project-phases")
}

func (h *ProjectHandler) PhaseDelete(c echo.Context) error {
	p, err := h.phaseContext(c, true)
	if err != nil {
		return err
	}
	cur, err := h.repo.GetPhase(c.Param("phaseID"))
	if err != nil || cur == nil || cur.ProjectID != p.ProjectID {
		return echo.ErrNotFound
	}
	off, err := h.repo.DeletePhase(cur.PhaseID)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/projects/"+p.ProjectID+"?err="+err.Error()+"#project-phases")
	}
	if off {
		h.logPhase(audit.ActionUpdate, cur, "단계 비활성")
	} else {
		h.logPhase(audit.ActionDelete, cur, "단계 삭제")
	}
	return c.Redirect(http.StatusSeeOther, "/projects/"+p.ProjectID+"?ok=phase#project-phases")
}

func (h *ProjectHandler) PhaseRestore(c echo.Context) error {
	p, err := h.phaseContext(c, true)
	if err != nil {
		return err
	}
	cur, err := h.repo.GetPhase(c.Param("phaseID"))
	if err != nil || cur == nil || cur.ProjectID != p.ProjectID {
		return echo.ErrNotFound
	}
	if err := h.repo.SetPhaseActive(cur.PhaseID, true); err != nil {
		return err
	}
	h.logPhase(audit.ActionUpdate, cur, "단계 복구")
	return c.Redirect(http.StatusSeeOther, "/projects/"+p.ProjectID+"?ok=phase#project-phases")
}

func (h *ProjectHandler) PhaseMove(c echo.Context) error {
	p, err := h.phaseContext(c, true)
	if err != nil {
		return err
	}
	cur, err := h.repo.GetPhase(c.Param("phaseID"))
	if err != nil || cur == nil || cur.ProjectID != p.ProjectID {
		return echo.ErrNotFound
	}
	dir := 1
	if c.FormValue("dir") == "up" {
		dir = -1
	}
	if err := h.repo.MovePhase(cur.PhaseID, dir); err != nil {
		return err
	}
	h.logPhase(audit.ActionUpdate, cur, "단계 순서")
	return c.Redirect(http.StatusSeeOther, "/projects/"+p.ProjectID+"?ok=phase#project-phases")
}

func (h *ProjectHandler) PhaseFill(c echo.Context) error {
	p, err := h.phaseContext(c, true)
	if err != nil {
		return err
	}
	_ = c.Request().ParseForm()
	names := c.Request().Form["name"]
	starts := c.Request().Form["plan_start"]
	ends := c.Request().Form["plan_end"]
	var rows []model.ProjectPhase
	for i, name := range names {
		row := model.ProjectPhase{Name: name}
		if i < len(starts) {
			row.PlanStart = starts[i]
		}
		if i < len(ends) {
			row.PlanEnd = ends[i]
		}
		rows = append(rows, row)
	}
	if _, _, err := h.repo.CreatePhasesSkipDup(p.ProjectID, p.OrgID, rows); err != nil {
		return c.Redirect(http.StatusSeeOther, "/projects/"+p.ProjectID+"?err="+err.Error()+"#project-phases")
	}
	audit.LogWithReason(audit.ActionCreate, "project_phases", "project_id", p.ProjectID, p.Name, "", "", "기본 단계 저장")
	return c.Redirect(http.StatusSeeOther, "/projects/"+p.ProjectID+"?ok=phase#project-phases")
}

func (h *ProjectHandler) logPhase(action string, p *model.ProjectPhase, reason string) {
	if p == nil {
		return
	}
	audit.LogWithReason(action, "project_phases", "phase_id", p.PhaseID, p.Name, "", p.Name, reason)
}

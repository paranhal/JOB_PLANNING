package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func parseRelatedOrgIDs(c echo.Context) []string {
	_ = c.Request().ParseForm()
	return append([]string{}, c.Request().Form["related_org_ids"]...)
}

func (h *ProjectHandler) saveRelatedOrgs(c echo.Context, p *model.WorkProject) error {
	if p == nil || strings.TrimSpace(p.ProjectID) == "" {
		return nil
	}
	if !canEditProjectOrgs(c, p) {
		return nil
	}
	owner := strings.TrimSpace(p.OrgID)
	if v := strings.TrimSpace(c.FormValue("org_id")); v != "" {
		owner = v
	}
	got, err := h.repo.Get(p.ProjectID)
	if err == nil && got != nil && strings.TrimSpace(got.OrgID) != "" {
		owner = got.OrgID
	}
	return h.repo.ReplaceRelatedOrgs(p.ProjectID, currentUserID(c), parseRelatedOrgIDs(c), owner)
}

func canEditProjectOrgs(c echo.Context, p *model.WorkProject) bool {
	if isReadOnly(c) || !canWriteProjects(c) {
		return false
	}
	role := currentRole(c)
	if role == model.RoleCLevel {
		return false
	}
	if role == model.RoleVisionAdmin {
		return true
	}
	if role != model.RoleOrgAdmin {
		return false
	}
	if p == nil || strings.TrimSpace(p.ProjectID) == "" {
		return true
	}
	home := loginHomeOrg(c)
	return home != "" && strings.TrimSpace(p.OrgID) == home
}

func canTransferProjectOrg(c echo.Context, p *model.WorkProject) bool {
	if !canEditProjectOrgs(c, p) {
		return false
	}
	role := currentRole(c)
	return role == model.RoleVisionAdmin || role == model.RoleOrgAdmin
}

func orgPickerLocked(c echo.Context, isEdit bool) bool {
	if isReadOnly(c) {
		return true
	}
	role := currentRole(c)
	if role == model.RoleVisionAdmin {
		return false
	}
	if role == model.RoleOrgAdmin && isEdit {
		return false
	}
	return true
}

func (h *ProjectHandler) moveProjectOrg(c echo.Context, existing *model.WorkProject, toOrg string) error {
	if !canTransferProjectOrg(c, existing) {
		return echo.NewHTTPError(http.StatusForbidden, "주관 부서를 바꿀 수 없습니다")
	}
	if currentRole(c) == model.RoleOrgAdmin {
		home := loginHomeOrg(c)
		if home == "" || strings.TrimSpace(existing.OrgID) != home {
			return echo.NewHTTPError(http.StatusForbidden, "자기 팀 사업만 이관할 수 있습니다")
		}
	}
	keep := c.FormValue("keep_prev_related") == "1"
	moveTasks := c.FormValue("move_open_tasks") == "1"
	dropAssign := c.FormValue("drop_foreign_assign") == "1"
	return h.repo.UpdateOrg(existing.ProjectID, repository.ProjectOrgMoveOpts{
		ToOrg:             toOrg,
		Reason:            strings.TrimSpace(c.FormValue("org_move_reason")),
		ByUser:            currentUserID(c),
		MoveOpenTasks:     moveTasks,
		KeepPrevAsRelated: keep,
		DropForeignAssign: dropAssign,
	})
}

func (h *ProjectHandler) orgFormData(c echo.Context, p *model.WorkProject, isEdit bool) map[string]interface{} {
	var orgs []model.Org
	if h.orgs != nil {
		orgs, _ = h.orgs.ListActive()
	}
	home := loginHomeOrg(c)
	if home == "" {
		home = currentOrg(c)
		if home == repository.OrgAll {
			home = ""
		}
	}
	selected := home
	if p != nil && strings.TrimSpace(p.OrgID) != "" {
		selected = p.OrgID
	}
	var related []string
	if p != nil {
		related = p.RelatedOrgIDs
	}
	impact := repository.ProjectOrgMoveImpact{}
	if isEdit && p != nil && p.ProjectID != "" {
		impact, _ = h.repo.OrgMoveImpact(p.ProjectID)
	}
	locked := orgPickerLocked(c, isEdit)
	role := currentRole(c)
	required := role == model.RoleVisionAdmin && !isEdit
	return map[string]interface{}{
		"OrgChoices":       orgs,
		"OrgSelected":      selected,
		"OrgLocked":        locked,
		"OrgRequired":      required,
		"OrgHomeName":      orgNameOf(orgs, selected),
		"RelatedOrgIDs":    related,
		"CanEditOrgs":      canEditProjectOrgs(c, p),
		"CanTransferOrg":   isEdit && canTransferProjectOrg(c, p),
		"OrgMoveImpact":    impact,
		"KeepPrevRelated":  true,
		"MoveOpenTasks":    true,
		"DropForeignAssign": true,
		"RelatedLine":      relatedLineForForm(p),
	}
}

func relatedLineForForm(p *model.WorkProject) string {
	if p == nil {
		return ""
	}
	return p.RelatedLine
}

func requireProjectRegisterAccess(c echo.Context, get func(string) (*model.WorkProject, error), projectID string) error {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil
	}
	if get == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "사업을 찾을 수 없습니다")
	}
	p, err := get(projectID)
	if err != nil || p == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "사업을 찾을 수 없습니다")
	}
	org := ""
	if c != nil {
		org = currentOrg(c)
		if org == "" {
			org = loginHomeOrg(c)
		}
	}
	if org == "" {
		return nil
	}
	if !model.CanRegisterWorkOnProject(*p, org) {
		return echo.NewHTTPError(http.StatusForbidden, "이 사업에 업무를 등록할 수 없습니다")
	}
	return nil
}

func orgNameOf(orgs []model.Org, id string) string {
	id = strings.TrimSpace(id)
	for _, o := range orgs {
		if o.OrgID == id {
			if s := strings.TrimSpace(o.OrgName); s != "" {
				return s
			}
			return o.OrgID
		}
	}
	return id
}

func formBoolDefaultOn(c echo.Context, name string) bool {
	v := strings.TrimSpace(c.FormValue(name))
	if v == "" {
		return true
	}
	return v != "0" && v != "false"
}

func parseMoveImpactN(v string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(v))
	return n
}

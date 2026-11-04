package handler

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func canDesignProjectWork(c echo.Context, h *ProjectHandler, p *model.WorkProject, orgID string) bool {
	return canRegisterProjectSites(c, h, p, orgID)
}

func canRegisterProjectSites(c echo.Context, h *ProjectHandler, p *model.WorkProject, orgID string) bool {
	if isReadOnly(c) || currentRole(c) == model.RoleCLevel {
		return false
	}
	role := currentRole(c)
	if role == model.RoleVisionAdmin {
		return true
	}
	orgID = strings.TrimSpace(orgID)
	if role == model.RoleOrgAdmin {
		home := loginHomeOrg(c)
		if home != "" && home == orgID {
			return true
		}
	}
	if h == nil || h.pe == nil || p == nil {
		return false
	}
	uid := currentUserID(c)
	if uid == "" || !hasPerm(c, model.PermWorkboard) {
		return false
	}
	return h.pe.AssignedDuty(p.ProjectID, uid, model.ProjectDutyPM)
}

func canCheckTaskSiteProgress(c echo.Context, t *model.WorkTask) bool {
	if t == nil || !canWriteWorkboard(c) || isReadOnly(c) || currentRole(c) == model.RoleCLevel {
		return false
	}
	if !model.TaskTracksSiteProgress(t.WorkType, t.SourceType) {
		return false
	}
	view := currentOrg(c)
	home := loginHomeOrg(c)
	taskOrg := strings.TrimSpace(t.OrgID)
	if taskOrg == "" {
		return true
	}
	if view == repository.OrgAll || view == taskOrg || home == taskOrg {
		return true
	}
	if currentRole(c) == model.RoleVisionAdmin {
		return true
	}
	return false
}

func (h *ProjectHandler) SitesPanel(c echo.Context) error {
	p, org, err := h.loadEquipProject(c)
	if err != nil {
		return err
	}
	n := 0
	if h.pe != nil {
		n = h.pe.CountActiveSites(p.ProjectID)
	}
	return c.Render(http.StatusOK, "project/sites_panel.html", map[string]interface{}{
		"Title":         "사이트",
		"Active":        NavProjects,
		"HideNav":       true,
		"Project":       p,
		"SiteCount":     n,
		"CanWriteSites": canRegisterProjectSites(c, h, p, org),
	})
}

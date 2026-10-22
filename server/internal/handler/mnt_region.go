package handler

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func canManageMntRegion(c echo.Context) bool {
	if isReadOnly(c) || viewAsActive(c) {
		return false
	}
	switch currentRole(c) {
	case model.RoleVisionAdmin, model.RoleOrgAdmin, model.RoleAdmin:
		return true
	default:
		return false
	}
}

func (h *MaintenanceHandler) RegionAssignees(c echo.Context) error {
	if !canViewMaintenance(c) {
		return echo.ErrForbidden
	}
	org := currentOrg(c)
	if org == "" || org == repository.OrgAll {
		if currentRole(c) != model.RoleVisionAdmin && currentRole(c) != model.RoleCLevel {
			return echo.ErrNotFound
		}
	}
	cfgs, err := h.repo.ListSiteConfigs()
	if err != nil {
		return err
	}
	cfgs = repository.FilterSiteConfigsByOrg(cfgs, org)
	type regionRow struct {
		Region string
		Sites  int
		People []model.MaintenanceRegionAssignee
	}
	idx := map[string]int{}
	var rows []regionRow
	unspecified := 0
	for _, cfg := range cfgs {
		reg := strings.TrimSpace(cfg.Region)
		if reg == "" {
			unspecified++
			continue
		}
		i, ok := idx[reg]
		if !ok {
			orgKey := org
			if orgKey == "" || orgKey == repository.OrgAll {
				orgKey = cfg.OrgID
			}
			people, _ := h.repo.ListRegionAssignees(orgKey, reg)
			rows = append(rows, regionRow{Region: reg, People: people})
			i = len(rows) - 1
			idx[reg] = i
		}
		rows[i].Sites++
	}
	total := len(cfgs)
	users := listAssignableForPicker(h.userRepo, c)
	return c.Render(http.StatusOK, "maintenance/regions.html", map[string]interface{}{
		"Title":       "지역별 담당",
		"Active":      NavMaintenance,
		"Rows":        rows,
		"Users":       users,
		"CanManage":   canManageMntRegion(c),
		"Total":       total,
		"WithRegion":  total - unspecified,
		"Unspecified": unspecified,
	})
}

func (h *MaintenanceHandler) SaveRegionAssignees(c echo.Context) error {
	if !canManageMntRegion(c) {
		return echo.ErrForbidden
	}
	org := currentOrg(c)
	if org == "" || org == repository.OrgAll {
		return echo.ErrNotFound
	}
	region := strings.TrimSpace(c.FormValue("region"))
	if region == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "지역을 선택하세요")
	}
	owner := strings.TrimSpace(c.FormValue("owner_id"))
	gid := strings.TrimSpace(c.FormValue("work_group_id"))
	var items []model.MaintenanceRegionAssignee
	if owner != "" || gid != "" {
		items = append(items, model.MaintenanceRegionAssignee{UserID: owner, WorkGroupID: gid, Role: model.WBMemberOwner})
	}
	for _, id := range c.Request().PostForm["support_id"] {
		id = strings.TrimSpace(id)
		if id == "" || id == owner {
			continue
		}
		items = append(items, model.MaintenanceRegionAssignee{UserID: id, Role: model.WBMemberSupport})
	}
	if err := h.repo.ReplaceRegionAssignees(org, region, items); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.Redirect(http.StatusSeeOther, "/maintenance/regions?ok=1")
}

func (h *MaintenanceHandler) ReapplyRegionAssignees(c echo.Context) error {
	if !canManageMntRegion(c) {
		return echo.ErrForbidden
	}
	org := currentOrg(c)
	if org == "" || org == repository.OrgAll {
		return echo.ErrNotFound
	}
	region := strings.TrimSpace(c.FormValue("region"))
	if region == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "지역을 선택하세요")
	}
	if err := h.repo.ApplyRegionDefaultsToOpenVisits(org, region); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	back := strings.TrimSpace(c.FormValue("redirect"))
	if back == "" {
		back = "/maintenance/regions?ok=reapply"
	}
	return c.Redirect(http.StatusSeeOther, back)
}

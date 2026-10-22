package handler

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func (h *MaintenanceHandler) renderSiteList(c echo.Context, flash string, changed, skipped, filled int) error {
	items, err := h.repo.ListSiteConfigs()
	if err != nil {
		return err
	}
	org := currentOrg(c)
	items = repository.FilterSiteConfigsByOrg(items, org)
	region := firstQueryOrForm(c.QueryParam("region"), c.FormValue("filter_region"))
	cycle := firstQueryOrForm(c.QueryParam("cycle"), c.FormValue("filter_cycle"))
	q := firstQueryOrForm(c.QueryParam("q"), c.FormValue("filter_q"))
	batch := firstQueryOrForm(c.QueryParam("batch"), c.FormValue("filter_batch"))
	sortKey := strings.TrimSpace(c.QueryParam("sort"))
	regions := repository.DistinctSiteRegions(items)
	items = repository.FilterMaintenanceSites(items, region, cycle, q, batch)
	if sortKey == "recent" {
		sort.SliceStable(items, func(i, j int) bool {
			return items[i].UpdatedAt > items[j].UpdatedAt
		})
	}
	for i := range items {
		items[i].DefaultAssignee = h.repo.SiteDefaultOwnerName(items[i].CustomerID)
	}
	now := time.Now()
	quota := model.SummarizeMonthVisitQuota(items, now.Year(), int(now.Month()))
	legacy := org == model.OrgIDLibrary && h.repo.CountInspectionProducts(org) == 0
	return c.Render(http.StatusOK, "maintenance/site_list.html", map[string]interface{}{
		"Title":        "정기점검 사이트 설정",
		"Active":       NavMaintenanceSites,
		"Items":        items,
		"Quota":        quota,
		"Legacy":       legacy,
		"Synced":       c.QueryParam("synced"),
		"Skipped":      c.QueryParam("skipped"),
		"Regions":      regions,
		"FilterRegion": region,
		"FilterCycle":  cycle,
		"FilterQ":      q,
		"FilterBatch":  batch,
		"SortRecent":   sortKey == "recent",
		"Flash":        flash,
		"Changed":      changed,
		"SkipN":        skipped,
		"Filled":       filled,
		"CanWrite":     canManageMntRegion(c) && org != "" && org != repository.OrgAll,
	})
}

func firstQueryOrForm(a, b string) string {
	a = strings.TrimSpace(a)
	if a != "" {
		return a
	}
	return strings.TrimSpace(b)
}

func (h *MaintenanceHandler) BulkSetSiteRegions(c echo.Context) error {
	if viewAsActive(c) || isReadOnly(c) || !canManageMntRegion(c) {
		return echo.ErrForbidden
	}
	org := currentOrg(c)
	if org == "" || org == repository.OrgAll {
		return echo.ErrNotFound
	}
	_ = c.Request().ParseForm()
	ids := c.Request().PostForm["customer_id"]
	if len(ids) > model.SiteBulkMax {
		return echo.NewHTTPError(http.StatusBadRequest, "한 번에 최대 3000곳까지 지정할 수 있습니다")
	}
	region := strings.TrimSpace(c.FormValue("region"))
	res, err := h.repo.BulkSetSiteRegions(org, ids, region)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	flash := fmt.Sprintf("변경 %d / 건너뜀 %d. 이미 만들어진 방문의 지역·담당은 바꾸지 않습니다.", res.Changed, len(res.Skipped))
	return h.renderSiteList(c, flash, res.Changed, len(res.Skipped), 0)
}

func (h *MaintenanceHandler) FillSiteRegionsFromAddress(c echo.Context) error {
	if viewAsActive(c) || isReadOnly(c) || !canManageMntRegion(c) {
		return echo.ErrForbidden
	}
	org := currentOrg(c)
	if org == "" || org == repository.OrgAll {
		return echo.ErrNotFound
	}
	_ = c.Request().ParseForm()
	ids := c.Request().PostForm["customer_id"]
	if len(ids) > model.SiteBulkMax {
		return echo.NewHTTPError(http.StatusBadRequest, "한 번에 최대 3000곳까지 지정할 수 있습니다")
	}
	res, err := h.repo.FillSiteRegionsFromAddress(org, ids)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	flash := fmt.Sprintf("주소로 채움 %d / 건너뜀 %d. 고객 주소는 바꾸지 않았습니다.", res.Changed, len(res.Skipped))
	return h.renderSiteList(c, flash, res.Changed, len(res.Skipped), res.Changed)
}

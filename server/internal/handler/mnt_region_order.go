package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

// ListRegionOrder 기준정보 > 지역 거리 순서. §34.4.3
func (h *MaintenanceHandler) ListRegionOrder(c echo.Context) error {
	saved, err := h.repo.ListRegionDistanceOrder()
	if err != nil {
		return err
	}
	cands, err := h.repo.ListSiteRegionCandidates()
	if err != nil {
		return err
	}
	byKey := map[string]model.RegionDistanceOrder{}
	for _, it := range saved {
		byKey[model.RegionKey(it.Sido, it.Sigungu)] = it
	}
	seen := map[string]bool{}
	var rows []model.RegionDistanceOrder
	for _, it := range saved {
		key := model.RegionKey(it.Sido, it.Sigungu)
		seen[key] = true
		rows = append(rows, it)
	}
	for _, it := range cands {
		key := model.RegionKey(it.Sido, it.Sigungu)
		if seen[key] {
			continue
		}
		seen[key] = true
		it.SortOrder = 0
		rows = append(rows, it)
	}
	return c.Render(http.StatusOK, "maintenance/region_order.html", map[string]interface{}{
		"Title":  "지역 거리 순서",
		"Active": NavRegionOrder,
		"Rows":   rows,
		"Saved":  c.QueryParam("ok") == "1",
	})
}

func (h *MaintenanceHandler) SaveRegionOrder(c echo.Context) error {
	if err := c.Request().ParseForm(); err != nil {
		return err
	}
	sidos := c.Request().PostForm["sido"]
	sigungus := c.Request().PostForm["sigungu"]
	orders := c.Request().PostForm["sort_order"]
	n := len(sidos)
	if len(sigungus) < n {
		n = len(sigungus)
	}
	if len(orders) < n {
		n = len(orders)
	}
	var items []model.RegionDistanceOrder
	for i := 0; i < n; i++ {
		ord, _ := strconv.Atoi(strings.TrimSpace(orders[i]))
		if ord <= 0 {
			continue
		}
		sido := strings.TrimSpace(sidos[i])
		sigungu := strings.TrimSpace(sigungus[i])
		if sido == "" && sigungu == "" {
			continue
		}
		items = append(items, model.RegionDistanceOrder{
			Sido: sido, Sigungu: sigungu, SortOrder: ord,
		})
	}
	if err := h.repo.ReplaceRegionDistanceOrder(items); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/maintenance/sites/distance?ok=1")
}

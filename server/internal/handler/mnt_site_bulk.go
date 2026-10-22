package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

const siteBulkSkipCookie = "jp_mnt_bulk_skips"

func bulkForbidden(c echo.Context) error {
	return c.Render(http.StatusForbidden, "auth/forbidden.html", map[string]interface{}{
		"Title": "접근 권한 없음", "Active": NavNone,
	})
}

func (h *MaintenanceHandler) SiteBulkPage(c echo.Context) error {
	if viewAsActive(c) && c.Request().Method != http.MethodGet {
		return bulkForbidden(c)
	}
	return h.renderSiteBulk(c, "", nil)
}

func (h *MaintenanceHandler) renderSiteBulk(c echo.Context, errMsg string, result *model.SiteBulkRegisterResult) error {
	org := currentOrg(c)
	needOrg := org == "" || org == repository.OrgAll
	buckets, _ := h.repo.ListSiteBulkIndustries()
	rawInd := strings.TrimSpace(c.QueryParam("industry"))
	if rawInd == "" {
		rawInd = strings.TrimSpace(c.FormValue("industry"))
	}
	industry := ""
	var cands []model.SiteBulkCandidate
	if rawInd != "" {
		industry = model.IndustryKey(rawInd)
		if !needOrg {
			cands, _ = h.repo.ListSiteBulkCandidates(industry, org)
		}
	}
	var products []model.InspectionProduct
	legacy := true
	if !needOrg {
		legacy = org == model.OrgIDLibrary && h.repo.CountInspectionProducts(org) == 0
		if !legacy {
			products, _ = h.repo.ListInspectionProducts(org, false)
		}
	}
	status := http.StatusOK
	if errMsg != "" && result == nil {
		status = http.StatusBadRequest
	}
	selectable := 0
	warnNoRegion := 0
	for _, row := range cands {
		if row.Selectable {
			selectable++
			if row.WarnNoRegion {
				warnNoRegion++
			}
		}
	}
	planHref := "/maintenance"
	if !needOrg {
		plans, _ := h.repo.ListPlansInOrg(org)
		y := time.Now().Year()
		for _, p := range plans {
			if p.PlanYear == y {
				planHref = "/maintenance/" + p.PlanID
				break
			}
		}
	}
	return c.Render(status, "maintenance/bulk.html", map[string]interface{}{
		"Title":          "업종별 일괄 등록",
		"Active":         NavMaintenanceSites,
		"NeedOrg":        needOrg,
		"Err":            errMsg,
		"Buckets":        buckets,
		"Industry":       industry,
		"Candidates":     cands,
		"Selectable":     selectable,
		"WarnNoRegion":   warnNoRegion,
		"Products":       products,
		"Legacy":         legacy,
		"Result":         result,
		"PlanHref":       planHref,
		"Cycle":          strings.TrimSpace(c.FormValue("inspection_cycle")),
		"Rounds":         parseVisitRound(c.FormValue("rounds_per_period")),
		"FallbackRegion": strings.TrimSpace(c.FormValue("fallback_region")),
	})
}

func (h *MaintenanceHandler) SiteBulkRegister(c echo.Context) error {
	if viewAsActive(c) || isReadOnly(c) {
		return bulkForbidden(c)
	}
	_ = c.Request().ParseForm()
	org, err := resolveInsertOrg(c)
	if err != nil {
		if err.Error() == errNeedInsertOrg {
			return h.renderSiteBulk(c, errNeedInsertOrg, nil)
		}
		return bulkForbidden(c)
	}
	cycle := strings.TrimSpace(c.FormValue("inspection_cycle"))
	if !model.ValidInspectionCycle(cycle) {
		return h.renderSiteBulk(c, "점검주기를 선택하세요", nil)
	}
	ids := c.Request().PostForm["customer_id"]
	if len(ids) == 0 {
		return h.renderSiteBulk(c, "등록할 사이트를 선택하세요", nil)
	}
	if len(ids) > model.SiteBulkMax {
		return echo.NewHTTPError(http.StatusBadRequest, "한 번에 최대 3000곳까지 등록할 수 있습니다")
	}
	shorts := map[string]string{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		shorts[id] = strings.TrimSpace(c.FormValue("short_" + id))
	}
	res, err := h.repo.BulkRegisterSites(model.SiteBulkRegisterInput{
		OrgID:          org,
		IndustryKey:    c.FormValue("industry"),
		Cycle:          cycle,
		Rounds:         parseVisitRound(c.FormValue("rounds_per_period")),
		CustomerIDs:    ids,
		ShortNames:     shorts,
		ProductIDs:     c.Request().PostForm["product_id"],
		ActorLabel:     ctxString(c, "user_name"),
		FallbackRegion: strings.TrimSpace(c.FormValue("fallback_region")),
	})
	if err != nil {
		return h.renderSiteBulk(c, err.Error(), nil)
	}
	setSiteBulkSkipCookie(c, res.Skipped)
	return h.renderSiteBulk(c, "", res)
}

func setSiteBulkSkipCookie(c echo.Context, skips []model.SiteBulkSkip) {
	if len(skips) == 0 {
		c.SetCookie(&http.Cookie{Name: siteBulkSkipCookie, Value: "", Path: "/", MaxAge: -1})
		return
	}
	b, err := json.Marshal(skips)
	if err != nil || len(b) > 3500 {
		return
	}
	c.SetCookie(&http.Cookie{Name: siteBulkSkipCookie, Value: string(b), Path: "/maintenance/sites", MaxAge: 600, HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

func readSiteBulkSkipCookie(c echo.Context) []model.SiteBulkSkip {
	ck, err := c.Cookie(siteBulkSkipCookie)
	if err != nil || ck == nil || strings.TrimSpace(ck.Value) == "" {
		return nil
	}
	var skips []model.SiteBulkSkip
	if json.Unmarshal([]byte(ck.Value), &skips) != nil {
		return nil
	}
	return skips
}

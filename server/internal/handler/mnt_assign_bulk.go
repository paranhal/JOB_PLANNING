package handler

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type unassignedRegionGroup struct {
	Key   string
	Label string
	Slots []model.UnassignedMonthSlot
}

func groupUnassignedByRegion(slots []model.UnassignedMonthSlot, cfgs []model.MaintenanceSiteConfig) []unassignedRegionGroup {
	byID := map[string]model.MaintenanceSiteConfig{}
	for _, c := range cfgs {
		byID[c.CustomerID] = c
	}
	order := []string{}
	groups := map[string]*unassignedRegionGroup{}
	for _, s := range slots {
		cfg := byID[s.CustomerID]
		sido, gu := model.SiteRegion(cfg)
		key := model.FormatRegionKey(sido, gu)
		lab := model.SiteRegionLabel(cfg)
		if lab == "" {
			lab = model.RegionUnspecified
			key = model.RegionUnspecified
		}
		g := groups[key]
		if g == nil {
			g = &unassignedRegionGroup{Key: key, Label: lab}
			groups[key] = g
			order = append(order, key)
		}
		g.Slots = append(g.Slots, s)
	}
	sort.Slice(order, func(i, j int) bool {
		return groups[order[i]].Label < groups[order[j]].Label
	})
	out := make([]unassignedRegionGroup, 0, len(order))
	for _, k := range order {
		g := groups[k]
		sort.Slice(g.Slots, func(i, j int) bool {
			a, b := g.Slots[i], g.Slots[j]
			na, nb := a.ShortName, b.ShortName
			if na == "" {
				na = a.OrgName
			}
			if nb == "" {
				nb = b.OrgName
			}
			if na != nb {
				return na < nb
			}
			return a.ProductType < b.ProductType
		})
		out = append(out, *g)
	}
	return out
}

func slotKeyOf(s model.UnassignedMonthSlot) string {
	return strings.TrimSpace(s.CustomerID) + "|" + strings.TrimSpace(s.ProductType)
}

func parseSlotKey(k string) (customerID, product string) {
	k = strings.TrimSpace(k)
	i := strings.Index(k, "|")
	if i < 0 {
		return k, ""
	}
	return k[:i], k[i+1:]
}

func workdaysInclusive(from, to string, skipWeekend bool, holidays map[string]bool) []string {
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	if len(from) < 10 || len(to) < 10 {
		return nil
	}
	a, err1 := time.Parse("2006-01-02", from[:10])
	b, err2 := time.Parse("2006-01-02", to[:10])
	if err1 != nil || err2 != nil || b.Before(a) {
		return nil
	}
	var out []string
	for d := a; !d.After(b); d = d.AddDate(0, 0, 1) {
		ds := d.Format("2006-01-02")
		if skipWeekend && (d.Weekday() == time.Saturday || d.Weekday() == time.Sunday) {
			continue
		}
		if holidays[ds] {
			continue
		}
		out = append(out, ds)
	}
	return out
}

func spreadSlotDates(slots []model.UnassignedMonthSlot, days []string, perDay int) (map[string]string, error) {
	if perDay < 1 {
		return nil, fmt.Errorf("하루 처리 건수를 입력하세요")
	}
	if len(days) == 0 {
		return nil, fmt.Errorf("배정할 영업일이 없습니다")
	}
	need := (len(slots) + perDay - 1) / perDay
	if need > len(days) {
		return nil, fmt.Errorf("날짜 범위가 부족합니다. 필요 %d일, 가능 %d일", need, len(days))
	}
	out := map[string]string{}
	di, used := 0, 0
	for _, s := range slots {
		if di >= len(days) {
			return nil, fmt.Errorf("날짜 범위가 부족합니다")
		}
		out[slotKeyOf(s)] = days[di]
		used++
		if used >= perDay {
			di++
			used = 0
		}
	}
	return out, nil
}

func (h *MaintenanceHandler) slotIndexForPlan(plan *model.MaintenancePlan, month int) (map[string]model.UnassignedMonthSlot, []model.UnassignedMonthSlot, error) {
	slots, err := h.repo.ListOpenUnassignedSlotsForOrg(plan.PlanID, plan.PlanYear, month, plan.OrgID)
	if err != nil {
		return nil, nil, err
	}
	idx := map[string]model.UnassignedMonthSlot{}
	for _, s := range slots {
		idx[slotKeyOf(s)] = s
	}
	return idx, slots, nil
}

type bulkAssignPreview struct {
	OK       bool                         `json:"ok"`
	Error    string                       `json:"error,omitempty"`
	Days     []bulkAssignDay              `json:"days"`
	Warnings []string                     `json:"warnings,omitempty"`
	Count    int                          `json:"count"`
	Skipped  []model.MonthTargetSkip      `json:"skipped,omitempty"`
}

type bulkAssignDay struct {
	Date  string   `json:"date"`
	N     int      `json:"n"`
	Sites []string `json:"sites"`
}

func (h *MaintenanceHandler) parseBulkAssign(c echo.Context, plan *model.MaintenancePlan) (dates map[string]string, selected []model.UnassignedMonthSlot, skipped []model.MonthTargetSkip, err error) {
	_ = c.Request().ParseForm()
	keys := uniqueNonEmptyIDs(c.Request().PostForm["slot_key"])
	if len(keys) == 0 {
		keys = uniqueNonEmptyIDs(c.Request().Form["slot_key"])
	}
	month, _ := strconv.Atoi(firstNonEmpty(c.FormValue("month"), c.QueryParam("month")))
	if month < 1 || month > 12 {
		month = int(time.Now().Month())
	}
	idx, _, err := h.slotIndexForPlan(plan, month)
	if err != nil {
		return nil, nil, nil, err
	}
	cfgs, _ := h.repo.ListSiteConfigs()
	allow := map[string]bool{}
	for _, cfg := range repository.FilterSiteConfigsByOrg(cfgs, plan.OrgID) {
		allow[cfg.CustomerID] = true
	}
	for _, k := range keys {
		s, ok := idx[k]
		if !ok {
			cid, prod := parseSlotKey(k)
			skipped = append(skipped, model.MonthTargetSkip{CustomerID: cid, Name: prod, Reason: "미배정 목록에 없음"})
			continue
		}
		if !allow[s.CustomerID] {
			skipped = append(skipped, model.MonthTargetSkip{CustomerID: s.CustomerID, Name: s.ShortName, Reason: "다른 조직"})
			continue
		}
		selected = append(selected, s)
	}
	mode := strings.TrimSpace(c.FormValue("mode"))
	dates = map[string]string{}
	if mode == "spread" {
		from := c.FormValue("date_from")
		to := c.FormValue("date_to")
		perDay, _ := strconv.Atoi(c.FormValue("per_day"))
		skipWE := c.FormValue("skip_weekend") != "0"
		holidays := map[string]bool{}
		if h.holidayRepo != nil && skipWE {
			if set, e := h.holidayRepo.DatesSet(plan.PlanYear); e == nil {
				holidays = set
			}
		}
		days := workdaysInclusive(from, to, skipWE, holidays)
		dates, err = spreadSlotDates(selected, days, perDay)
		return dates, selected, skipped, err
	}
	same := strings.TrimSpace(firstNonEmpty(c.FormValue("visit_date"), c.FormValue("date")))
	if same == "" {
		return nil, selected, skipped, fmt.Errorf("방문일을 선택하세요")
	}
	for _, s := range selected {
		dates[slotKeyOf(s)] = same
	}
	return dates, selected, skipped, nil
}

func (h *MaintenanceHandler) PreviewAssignUnassignedSlotsBulk(c echo.Context) error {
	if !canEditMaintenanceSchedule(c) {
		return echo.ErrForbidden
	}
	plan, err := h.requireVisiblePlan(c, c.Param("id"))
	if err != nil {
		return err
	}
	dates, selected, skipped, err := h.parseBulkAssign(c, plan)
	out := bulkAssignPreview{OK: err == nil, Count: len(selected), Skipped: skipped}
	if err != nil {
		out.Error = err.Error()
		return c.JSON(http.StatusBadRequest, out)
	}
	byDay := map[string][]string{}
	order := []string{}
	for _, s := range selected {
		d := dates[slotKeyOf(s)]
		name := s.ShortName
		if name == "" {
			name = s.OrgName
		}
		if _, ok := byDay[d]; !ok {
			order = append(order, d)
		}
		byDay[d] = append(byDay[d], name)
	}
	sort.Strings(order)
	assignee := strings.TrimSpace(c.FormValue("assignee"))
	for _, d := range order {
		sites := byDay[d]
		out.Days = append(out.Days, bulkAssignDay{Date: d, N: len(sites), Sites: sites})
		if assignee != "" && h.repo != nil {
			n, _ := h.repo.CountVisitsByAssigneeOn(plan.PlanID, assignee, d)
			if n > 0 {
				out.Warnings = append(out.Warnings, fmt.Sprintf("%s %s 기존 방문 %d건", d, assignee, n))
			}
		}
	}
	return c.JSON(http.StatusOK, out)
}

func (h *MaintenanceHandler) AssignUnassignedSlotsBulk(c echo.Context) error {
	if !canEditMaintenanceSchedule(c) {
		return echo.ErrForbidden
	}
	plan, err := h.requireVisiblePlan(c, c.Param("id"))
	if err != nil {
		return err
	}
	dates, selected, skipped, err := h.parseBulkAssign(c, plan)
	if err != nil {
		if wantsJSON(c) {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error()})
		}
		return h.manageErrRedirect(c, plan.PlanID, err.Error())
	}
	applied := 0
	for _, s := range selected {
		date := dates[slotKeyOf(s)]
		v := model.MaintenanceVisit{
			PlanID: plan.PlanID, VisitDate: date, CustomerID: s.CustomerID,
			ProductType: s.ProductType, EntryCategory: "normal",
		}
		if err := applyVisitDupForm(c, &v); err != nil {
			skipped = append(skipped, model.MonthTargetSkip{CustomerID: s.CustomerID, Name: s.ShortName, Reason: err.Error()})
			continue
		}
		if err := h.applyVisitAssigneesFromForm(c, &v); err != nil {
			skipped = append(skipped, model.MonthTargetSkip{CustomerID: s.CustomerID, Name: s.ShortName, Reason: err.Error()})
			continue
		}
		if strings.TrimSpace(v.Assignee) == "" {
			v.Assignee = strings.TrimSpace(firstNonEmpty(c.FormValue("assignee"), s.LastAssignee, s.RegionAssigneeName))
		}
		vid, err := h.repo.AssignSlotID(v)
		if err != nil {
			skipped = append(skipped, model.MonthTargetSkip{CustomerID: s.CustomerID, Name: s.ShortName, Reason: err.Error()})
			continue
		}
		applied++
		uid := currentUserID(c)
		on := viewAsActive(c) || currentRole(c) == model.RoleOrgAdmin
		_ = h.repo.StampVisitActedBy(vid, uid, ctxString(c, "user_name"), on)
	}
	if h.wbRepo != nil && applied > 0 {
		_, _ = h.wbRepo.EnsureMaintenanceTasks()
	}
	if applied > 0 {
		h.repo.TouchPlanUpdated(plan.PlanID)
	}
	msg := fmt.Sprintf("일괄 배정 %d건", applied)
	if n := len(skipped); n > 0 {
		msg += fmt.Sprintf(" / 제외 %d건", n)
		var bits []string
		for i, s := range skipped {
			if i >= 5 {
				break
			}
			bits = append(bits, strings.TrimSpace(s.Name+" "+s.Reason))
		}
		if len(bits) > 0 {
			msg += ": " + strings.Join(bits, ", ")
		}
	}
	if wantsJSON(c) {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"ok": true, "applied": applied, "skipped": skipped, "message": msg,
		})
	}
	return h.manageOKRedirect(c, plan.PlanID, msg)
}

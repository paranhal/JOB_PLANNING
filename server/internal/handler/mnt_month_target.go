package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func (h *MaintenanceHandler) MonthTargetPanelJSON(c echo.Context) error {
	if !canViewMaintenance(c) {
		return echo.ErrForbidden
	}
	plan, err := h.requireVisiblePlan(c, c.Param("id"))
	if err != nil {
		return err
	}
	month, _ := strconv.Atoi(c.QueryParam("month"))
	if month < 1 || month > 12 {
		month = 1
	}
	stats, sites, err := h.repo.MonthTargetPanelFilter(plan, month, c.QueryParam("q"), c.QueryParam("industry"),
		c.QueryParam("sido"), c.QueryParam("sigungu"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"month": month, "regions": stats, "sites": sites,
	})
}

func (h *MaintenanceHandler) RegisterMonthTargets(c echo.Context) error {
	if !canEditMaintenanceSchedule(c) {
		return echo.ErrForbidden
	}
	plan, err := h.requireVisiblePlan(c, c.Param("id"))
	if err != nil {
		return err
	}
	month, _ := strconv.Atoi(firstNonEmpty(c.FormValue("month"), c.QueryParam("month")))
	source := c.FormValue("source")
	_ = c.Request().ParseForm()
	custs := append([]string{}, c.Request().PostForm["customer_id"]...)
	regions := append([]string{}, c.Request().PostForm["region"]...)
	if len(regions) == 0 {
		if r := strings.TrimSpace(c.FormValue("region")); r != "" {
			regions = []string{r}
		}
	}
	actorID := strings.TrimSpace(currentUserID(c))
	actorName := strings.TrimSpace(ctxString(c, "user_name"))
	regions = model.CollapseMonthTargetRegionKeys(regions)
	if len(regions) == 0 && len(custs) == 0 {
		msg := "등록할 지역을 선택하세요"
		if source == model.MonthTargetSourceSite {
			msg = "등록할 사이트를 선택하세요"
		}
		if wantsJSON(c) {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{"ok": false, "error": msg, "message": msg})
		}
		return h.manageErrRedirect(c, plan.PlanID, msg)
	}
	var res model.MonthTargetRegisterResult
	if len(regions) > 0 && (source == model.MonthTargetSourceRegion || source == "") {
		for _, reg := range regions {
			part, err := h.repo.RegisterMonthTargets(plan, month, model.MonthTargetSourceRegion, reg, nil, actorID, actorName)
			if err != nil {
				return h.monthTargetErr(c, plan.PlanID, err)
			}
			res.Added += part.Added
			res.Skipped = append(res.Skipped, part.Skipped...)
		}
	}
	if len(custs) > 0 {
		part, err := h.repo.RegisterMonthTargets(plan, month, model.MonthTargetSourceSite, "", custs, actorID, actorName)
		if err != nil {
			return h.monthTargetErr(c, plan.PlanID, err)
		}
		res.Added += part.Added
		res.Skipped = append(res.Skipped, part.Skipped...)
	}
	msg := fmt.Sprintf("%d곳 등록", res.Added)
	if n := len(res.Skipped); n > 0 {
		msg += fmt.Sprintf(" / %d곳 건너뜀", n)
	}
	if len(regions) > 0 {
		msg = model.RegionRegisterSummary(regions, res.Added, len(res.Skipped))
	}
	if wantsJSON(c) {
		return c.JSON(http.StatusOK, map[string]interface{}{"ok": true, "added": res.Added, "skipped": res.Skipped, "message": msg})
	}
	return h.manageOKRedirect(c, plan.PlanID, msg)
}

func (h *MaintenanceHandler) CancelMonthTarget(c echo.Context) error {
	if !canEditMaintenanceSchedule(c) {
		return echo.ErrForbidden
	}
	plan, err := h.requireVisiblePlan(c, c.Param("id"))
	if err != nil {
		return err
	}
	if err := h.repo.CancelMonthTarget(c.Param("tid")); err != nil {
		return h.monthTargetErr(c, plan.PlanID, err)
	}
	if wantsJSON(c) {
		return c.JSON(http.StatusOK, map[string]interface{}{"ok": true})
	}
	return h.manageOKRedirect(c, plan.PlanID, "등록을 취소했습니다")
}

func (h *MaintenanceHandler) MoveMonthTarget(c echo.Context) error {
	if !canEditMaintenanceSchedule(c) {
		return echo.ErrForbidden
	}
	plan, err := h.requireVisiblePlan(c, c.Param("id"))
	if err != nil {
		return err
	}
	month, _ := strconv.Atoi(c.FormValue("month"))
	if err := h.repo.MoveMonthTarget(c.Param("tid"), month); err != nil {
		return h.monthTargetErr(c, plan.PlanID, err)
	}
	if wantsJSON(c) {
		return c.JSON(http.StatusOK, map[string]interface{}{"ok": true})
	}
	return h.manageOKRedirect(c, plan.PlanID, "점검 월을 옮겼습니다")
}

func (h *MaintenanceHandler) monthTargetErr(c echo.Context, planID string, err error) error {
	if wantsJSON(c) {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{"ok": false, "error": err.Error(), "message": err.Error()})
	}
	return h.manageErrRedirect(c, planID, err.Error())
}

func (h *WorkboardHandler) QuickRegisterMaintenance(c echo.Context) error {
	if !canWriteWorkboard(c) || isReadOnly(c) {
		return echo.ErrForbidden
	}
	if h.mntRepo == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "정기점검 저장소를 쓸 수 없습니다")
	}
	var req struct {
		CustomerID  string `json:"customer_id" form:"customer_id"`
		ProductType string `json:"product_type" form:"product_type"`
		Month       int    `json:"month" form:"month"`
		VisitDate   string `json:"visit_date" form:"visit_date"`
		Assignee    string `json:"assignee" form:"assignee"`
	}
	if strings.Contains(c.Request().Header.Get(echo.HeaderContentType), "json") {
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "요청을 확인하세요"})
		}
	} else {
		req.CustomerID = c.FormValue("customer_id")
		req.ProductType = c.FormValue("product_type")
		req.Month, _ = strconv.Atoi(c.FormValue("month"))
		req.VisitDate = c.FormValue("visit_date")
		req.Assignee = c.FormValue("assignee")
	}
	req.CustomerID = strings.TrimSpace(req.CustomerID)
	req.Assignee = strings.TrimSpace(req.Assignee)
	req.VisitDate = strings.TrimSpace(req.VisitDate)
	if req.CustomerID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "사이트를 선택하세요"})
	}
	if requiresAssigneeChoice(c) && !assigneeFilled(req.Assignee, "", "") {
		return c.JSON(http.StatusUnprocessableEntity, map[string]string{"error": msgAssigneeRequired})
	}
	cfg, err := h.mntRepo.GetSiteConfig(req.CustomerID)
	if err != nil || cfg == nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "점검 사이트가 아닙니다"})
	}
	org := currentOrg(c)
	if org != "" && org != repository.OrgAll && strings.TrimSpace(cfg.OrgID) != "" && cfg.OrgID != org {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "다른 조직 사이트입니다"})
	}
	planOrg := strings.TrimSpace(cfg.OrgID)
	if planOrg == "" {
		planOrg = org
	}
	year := time.Now().Year()
	if req.Month < 1 || req.Month > 12 {
		req.Month = int(time.Now().Month())
	}
	plan, err := h.mntRepo.EnsureYearPlan(planOrg, year)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	hint := ""
	if model.IsPeriodCycle(cfg.InspectionCycle) {
		if err := model.PeriodMonthTargetAllowed(cfg.InspectionCycle, req.Month); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		p, _ := model.CyclePeriodOf(cfg.InspectionCycle, req.Month)
		hint = fmt.Sprintf("이 사이트는 %s 주기입니다. %d월로 등록합니다.", maintCycleKindLabel(cfg.InspectionCycle), req.Month)
		res, err := h.mntRepo.RegisterMonthTargets(plan, req.Month, model.MonthTargetSourceSite, "", []string{req.CustomerID}, currentUserID(c), ctxString(c, "user_name"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		if res.Added == 0 && len(res.Skipped) > 0 {
			existing := h.findPeriodVisit(plan.PlanID, req.CustomerID, req.ProductType, year, p)
			if existing != nil {
				return c.JSON(http.StatusOK, map[string]interface{}{
					"ok": true, "visit_id": existing.VisitID, "title": existing.ShortName, "hint": res.Skipped[0].Reason, "existing": true,
				})
			}
			return c.JSON(http.StatusBadRequest, map[string]string{"error": res.Skipped[0].Reason})
		}
	}
	v := model.MaintenanceVisit{
		PlanID: plan.PlanID, VisitDate: req.VisitDate, CustomerID: req.CustomerID,
		ProductType: req.ProductType, Assignee: req.Assignee, EntryCategory: "normal",
	}
	visitID, err := h.mntRepo.InsertVisitFullID(v)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	num := model.FormatMaintenanceVisitNumber(req.VisitDate, req.ProductType)
	name := cfg.ShortName
	if name == "" {
		name = cfg.OrgName
	}
	title := model.FormatMaintenanceWorkTitle(name, num)
	return c.JSON(http.StatusOK, map[string]interface{}{
		"ok": true, "visit_id": visitID, "title": title, "source_number": num, "hint": hint,
	})
}

func (h *WorkboardHandler) findPeriodVisit(planID, customerID, product string, year int, p model.CyclePeriod) *model.MaintenanceVisit {
	visits, err := h.mntRepo.ListVisits(planID)
	if err != nil {
		return nil
	}
	for i := range visits {
		v := visits[i]
		if v.CustomerID != customerID {
			continue
		}
		if !model.VisitInCyclePeriod(v.VisitDate, year, p) {
			continue
		}
		if strings.TrimSpace(product) != "" && strings.TrimSpace(v.ProductType) != "" &&
			!strings.EqualFold(v.ProductType, product) {
			continue
		}
		return &v
	}
	return nil
}

func maintCycleKindLabel(cycle string) string {
	switch strings.TrimSpace(cycle) {
	case "quarterly":
		return "분기"
	case "semi":
		return "반기"
	case "yearly":
		return "년 1회"
	default:
		return cycle
	}
}

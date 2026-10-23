package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// ConfirmQuarterSlot 분기 점검 확인(D-71): 일정 정하기 / 이미 점검함 / 건너뜀.
func (h *MaintenanceHandler) ConfirmQuarterSlot(c echo.Context) error {
	if !canEditMaintenanceSchedule(c) {
		return echo.ErrForbidden
	}
	planID := c.Param("id")
	plan, err := h.requireVisiblePlan(c, planID)
	if err != nil {
		return err
	}
	action := strings.TrimSpace(c.FormValue("action"))
	cust := strings.TrimSpace(c.FormValue("customer_id"))
	product := strings.TrimSpace(c.FormValue("product_type"))
	slotKey := model.VisitProductSlotKey(product)
	formMonth, _ := strconv.Atoi(strings.TrimSpace(c.FormValue("month")))
	if formMonth < 1 || formMonth > 12 {
		formMonth = int(time.Now().Month())
	}
	cycle := strings.TrimSpace(c.FormValue("cycle"))
	if cfg, _ := h.repo.GetSiteConfig(cust); cfg != nil && model.IsPeriodCycle(cfg.InspectionCycle) {
		cycle = cfg.InspectionCycle
	}
	period, ok := model.CyclePeriodOf(cycle, formMonth)
	if !ok {
		period, ok = model.CyclePeriodOf("quarterly", formMonth)
	}
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "점검 주기를 확인하세요")
	}
	qNum := period.Index
	year := plan.PlanYear
	from, to := model.PeriodDateBounds(year, period)
	custOrg := h.repo.CustomerOrgID(cust)
	if strings.TrimSpace(custOrg) != "" && !repository.VisibleInOrg(custOrg, currentOrg(c)) {
		return echo.ErrForbidden
	}
	redirectErr := func(msg string) error {
		back := planURLFrom(c, planID, "")
		sep := "?"
		if strings.Contains(back, "?") {
			sep = "&"
		}
		return c.Redirect(http.StatusSeeOther, back+sep+"err="+url.QueryEscape(msg))
	}
	redirectOK := func(code string) error {
		back := planURLFrom(c, planID, "")
		sep := "?"
		if strings.Contains(back, "?") {
			sep = "&"
		}
		return c.Redirect(http.StatusSeeOther, back+sep+"ok="+code)
	}

	lastOfPeriod := formMonth == period.EndMonth
	switch action {
	case "skip":
		if !lastOfPeriod {
			return redirectErr("건너뜀은 기간 마지막 달에만 할 수 있습니다")
		}
		if err := h.repo.SkipQuarterSlot(planID, cust, slotKey, c.FormValue("reason"), currentUserDisplayName(c), year, qNum); err != nil {
			return redirectErr(err.Error())
		}
		return redirectOK("quarter-skip")
	case "schedule", "done":
		date, err := model.ParseAppDate(c.FormValue("visit_date"))
		if err != nil {
			return redirectErr(err.Error())
		}
		if date < from || date > to {
			return redirectErr(fmt.Sprintf("방문일은 %s ~ %s 사이여야 합니다", from, to))
		}
		v := model.MaintenanceVisit{
			PlanID: planID, VisitDate: date, CustomerID: cust,
			ProductType: product, EntryCategory: "normal",
			ProjectID: strings.TrimSpace(c.FormValue("project_id")),
		}
		if err := applyVisitDupForm(c, &v); err != nil {
			return redirectErr(err.Error())
		}
		if err := h.applyVisitAssigneesFromForm(c, &v); err != nil {
			return redirectErr(err.Error())
		}
		if strings.TrimSpace(v.Assignee) == "" && len(v.People) == 0 {
			name := strings.TrimSpace(c.FormValue("assignee"))
			kind, _ := parseAssigneeFilter(name)
			if name != "" && kind == "all" && name != "all" {
				v.Assignee = name
			}
		}
		if strings.TrimSpace(v.Assignee) == "" && len(v.People) == 0 {
			return redirectErr("담당자를 선택하세요")
		}
		if action == "done" && !lastOfPeriod {
			return redirectErr("이미 점검함은 기간 마지막 달에만 할 수 있습니다")
		}
		visitID, err := h.repo.AssignSlotID(v)
		if err != nil {
			return redirectErr(err.Error())
		}
		v.VisitID = visitID
		if action == "done" {
			if got := h.repo.FindVisitBySlot(planID, cust, product, date); got != nil {
				_ = h.repo.SetVisitCompleted(got.VisitID, true, date)
				h.syncVisitTask(got.VisitID, true)
			}
		}
		if h.wbRepo != nil {
			_, _ = h.wbRepo.EnsureMaintenanceTasks()
		}
		h.recordVisitAssign(c, v.VisitID, v.Assignee, planID, cust, product, date)
		h.repo.TouchPlanUpdated(planID)
		if action == "done" {
			return redirectOK("quarter-done")
		}
		return redirectOK("quarter-schedule")
	default:
		return redirectErr("확인 방법을 선택하세요")
	}
}

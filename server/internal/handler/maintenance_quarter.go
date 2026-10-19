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
	plan, err := h.repo.GetPlan(planID)
	if err != nil || plan == nil {
		return echo.NewHTTPError(http.StatusNotFound, "계획을 찾을 수 없습니다")
	}
	action := strings.TrimSpace(c.FormValue("action"))
	cust := strings.TrimSpace(c.FormValue("customer_id"))
	product := strings.TrimSpace(c.FormValue("product_type"))
	slotKey := model.VisitProductSlotKey(product)
	qNum, _ := strconv.Atoi(strings.TrimSpace(c.FormValue("quarter")))
	if qNum < 1 || qNum > 4 {
		qNum = model.CalendarQuarter(int(time.Now().Month()))
	}
	year := plan.PlanYear
	from, to := model.QuarterDateBounds(year, qNum)
	formMonth, _ := strconv.Atoi(strings.TrimSpace(c.FormValue("month")))
	if formMonth < 1 || formMonth > 12 {
		formMonth = int(time.Now().Month())
	}
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

	lastOfQuarter := model.IsLastMonthOfQuarter(formMonth) && model.CalendarQuarter(formMonth) == qNum
	switch action {
	case "skip":
		if !lastOfQuarter {
			return redirectErr("건너뜀은 분기 마지막 달에만 할 수 있습니다")
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
		assignee := strings.TrimSpace(c.FormValue("assignee"))
		if assignee == "" {
			return redirectErr("담당자를 선택하세요")
		}
		v := model.MaintenanceVisit{
			PlanID: planID, VisitDate: date, CustomerID: cust,
			ProductType: product, Assignee: assignee, EntryCategory: "normal",
			ProjectID: strings.TrimSpace(c.FormValue("project_id")),
		}
		if err := applyVisitDupForm(c, &v); err != nil {
			return redirectErr(err.Error())
		}
		if action == "done" && !lastOfQuarter {
			return redirectErr("이미 점검함은 분기 마지막 달에만 할 수 있습니다")
		}
		if err := h.repo.AssignSlot(v); err != nil {
			return redirectErr(err.Error())
		}
		if action == "done" {
			if got := h.repo.FindVisitBySlot(planID, cust, product, date); got != nil {
				_ = h.repo.SetVisitCompleted(got.VisitID, true, date)
				h.syncVisitTask(got.VisitID, true)
			}
		}
		if h.wbRepo != nil {
			_, _ = h.wbRepo.EnsureMaintenanceTasks()
		}
		h.recordVisitAssign(c, v.VisitID, assignee, planID, cust, product, date)
		h.repo.TouchPlanUpdated(planID)
		if action == "done" {
			return redirectOK("quarter-done")
		}
		return redirectOK("quarter-schedule")
	default:
		return redirectErr("확인 방법을 선택하세요")
	}
}

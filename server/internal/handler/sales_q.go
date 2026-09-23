package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func (h *SalesHandler) Sleep(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	until := strings.TrimSpace(c.FormValue("until"))
	if until == "" {
		until = model.DefaultDormantUntil(time.Now(), h.repo.DormantDefaultMonth())
	}
	reason := strings.TrimSpace(c.FormValue("reason"))
	noPlan := c.FormValue("no_plan") == "1" || c.FormValue("no_plan") == "on"
	warn := h.repo.NextActionBeforeDormant(id, until)
	err := h.repo.Sleep(id, until, reason, currentUserID(c), ctxString(c, "user_name"), noPlan)
	loc := "/sales/" + id + "?ok=dormant"
	if warn {
		loc += "&warn=next"
	}
	return h.replyStage(c, err, id, loc)
}

func (h *SalesHandler) WakeForm(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	p, err := h.repo.Get(c.Param("id"))
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/sales?err=notfound")
	}
	year := p.BudgetYear
	if year > 0 {
		year++
	}
	return c.Render(http.StatusOK, "sales/wake.html", map[string]interface{}{
		"Title": "휴면 해제", "Active": NavSales, "Project": p, "BudgetYear": year,
	})
}

func (h *SalesHandler) Wake(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	y, _ := strconv.Atoi(strings.TrimSpace(c.FormValue("budget_year")))
	err := h.repo.Wake(id, currentUserID(c), ctxString(c, "user_name"), y, c.FormValue("budget_status"))
	return h.replyStage(c, err, id, "/sales/"+id+"?ok=wake")
}

func (h *SalesHandler) BulkBizType(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	_, err := h.repo.BulkSetBizType(formIDs(c, "sales_id"), c.FormValue("biz_type"))
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/sales?err="+err.Error())
	}
	return c.Redirect(http.StatusSeeOther, "/sales?ok=bulk")
}

func (h *SalesHandler) BulkBudgetStatus(c echo.Context) error {
	if !canWriteSales(c) {
		return echo.ErrForbidden
	}
	_, err := h.repo.BulkSetBudgetStatus(formIDs(c, "sales_id"), c.FormValue("budget_status"))
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/sales?err="+err.Error())
	}
	return c.Redirect(http.StatusSeeOther, "/sales?ok=bulk")
}

func parseSalesListFilterQ(c echo.Context, f repository.SalesListFilter) repository.SalesListFilter {
	f.IncludeDormant = c.QueryParam("dormant") == "1"
	f.BizType = strings.TrimSpace(c.QueryParam("biz_type"))
	f.BudgetStatus = strings.TrimSpace(c.QueryParam("budget_status"))
	if y, err := strconv.Atoi(strings.TrimSpace(c.QueryParam("budget_year"))); err == nil {
		f.BudgetYear = y
	}
	f.GroupID = strings.TrimSpace(c.QueryParam("group_id"))
	f.ExpectedFrom = strings.TrimSpace(c.QueryParam("expected_from"))
	f.ExpectedTo = strings.TrimSpace(c.QueryParam("expected_to"))
	return f
}

func formIDs(c echo.Context, key string) []string {
	_ = c.Request().ParseForm()
	return c.Request().Form[key]
}

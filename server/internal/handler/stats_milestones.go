package handler

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/xuri/excelize/v2"

	"customer-support/internal/audit"
	"customer-support/internal/model"
	"customer-support/internal/repository"
	"customer-support/internal/service"
)

func (h *StatsHandler) Milestones(c echo.Context) error {
	now := time.Now()
	weeks := repository.MilestoneWeekOptions(now, 12)
	week := strings.TrimSpace(c.QueryParam("week"))
	if week == "" && len(weeks) > 0 {
		week = weeks[0]
	}
	domain := strings.TrimSpace(c.QueryParam("domain"))
	if domain == "" {
		domain = model.MilestoneDomainAS
	}
	scope := strings.TrimSpace(c.QueryParam("scope"))
	if scope == "" {
		scope = model.MilestoneScopeOrg
	}
	org := currentOrg(c)
	if h.repo != nil && week != "" {
		if t, err := time.ParseInLocation("2006-01-02", week, now.Location()); err == nil {
			if !h.repo.HasOrgMilestone(week, orgIDForCompute(org)) {
				_ = h.repo.ComputeMilestones(repository.WeekMonday(t), orgIDForCompute(org))
			}
		}
	}
	rows, _ := h.repo.ListMilestones(week, org, domain, scope)
	trend, _ := h.milestoneTrend(org, domain)
	mon := now
	if t, err := time.ParseInLocation("2006-01-02", week, now.Location()); err == nil {
		mon = t
	}
	deadline, deadDate, missing := service.WeekDeadline(h.repo, repository.WeekMonday(mon))
	st := h.repo.MilestoneOrgState(week, orgIDForCompute(org))
	type weekOpt struct {
		Value, Label, State string
	}
	var opts []weekOpt
	for _, w := range weeks {
		opts = append(opts, weekOpt{Value: w, Label: w, State: h.repo.MilestoneOrgState(w, orgIDForCompute(org))})
	}
	return c.Render(http.StatusOK, "stats/milestones.html", map[string]interface{}{
		"Title": "마일스톤", "Active": NavStatsMilestones,
		"Weeks": opts, "Week": week, "Domain": domain, "Scope": scope,
		"Rows": rows, "Trend": trend, "State": st,
		"Deadline": deadDate + " 10:00", "DeadlineAt": deadline.Format("2006-01-02 15:04"),
		"HolidayMissing": missing, "CanFix": canSwitchOrg(c),
	})
}

func orgIDForCompute(org string) string {
	if strings.TrimSpace(org) == "" || org == repository.OrgAll {
		return model.OrgIDLibrary
	}
	return org
}

func (h *StatsHandler) milestoneTrend(org, domain string) ([]model.MilestoneRow, error) {
	weeks := repository.MilestoneWeekOptions(time.Now(), 12)
	var out []model.MilestoneRow
	for i := len(weeks) - 1; i >= 0; i-- {
		rows, err := h.repo.ListMilestones(weeks[i], org, domain, model.MilestoneScopeOrg)
		if err != nil {
			return out, err
		}
		if len(rows) > 0 {
			out = append(out, rows[0])
		}
	}
	return out, nil
}

func (h *StatsHandler) RecomputeMilestone(c echo.Context) error {
	if !canSwitchOrg(c) {
		return echo.ErrForbidden
	}
	week := strings.TrimSpace(c.FormValue("week"))
	org := orgIDForCompute(currentOrg(c))
	if h.repo.MilestoneOrgState(week, org) == model.MilestoneFixed {
		return c.Redirect(http.StatusSeeOther, "/stats/milestones?week="+week+"&err=fixed")
	}
	t, err := time.ParseInLocation("2006-01-02", week, time.Local)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/stats/milestones")
	}
	_ = h.repo.ComputeMilestones(repository.WeekMonday(t), org)
	return c.Redirect(http.StatusSeeOther, "/stats/milestones?week="+week)
}

func (h *StatsHandler) FixMilestone(c echo.Context) error {
	if !canSwitchOrg(c) {
		return echo.ErrForbidden
	}
	week := strings.TrimSpace(c.FormValue("week"))
	org := orgIDForCompute(currentOrg(c))
	by := strings.TrimSpace(ctxString(c, "user_name"))
	_ = h.repo.FixMilestones(week, org, by)
	return c.Redirect(http.StatusSeeOther, "/stats/milestones?week="+week)
}

func (h *StatsHandler) UnfixMilestone(c echo.Context) error {
	if !canSwitchOrg(c) {
		return echo.ErrForbidden
	}
	week := strings.TrimSpace(c.FormValue("week"))
	org := orgIDForCompute(currentOrg(c))
	by := strings.TrimSpace(ctxString(c, "user_name"))
	if by == "" {
		by = strings.TrimSpace(ctxString(c, "username"))
	}
	_ = h.repo.UnfixMilestones(week, org, by)
	audit.Log(audit.ActionUpdate, "milestones", "week_start", week, "확정 되돌림", `{"state":"fixed"}`, `{"state":"draft","by":"`+by+`"}`)
	return c.Redirect(http.StatusSeeOther, "/stats/milestones?week="+week)
}

func (h *StatsHandler) MilestoneItems(c echo.Context) error {
	week := c.QueryParam("week")
	domain := c.QueryParam("domain")
	metric := c.QueryParam("metric")
	org := orgIDForCompute(currentOrg(c))
	items, _ := h.repo.ListMilestoneEvidence(week, org, domain, metric)
	return c.Render(http.StatusOK, "stats/milestone_items.html", map[string]interface{}{
		"Title": "근거", "HideNav": true, "Items": items, "Week": week, "Domain": domain, "Metric": metric,
	})
}

func (h *StatsHandler) ExportMilestones(c echo.Context) error {
	week := strings.TrimSpace(c.QueryParam("week"))
	org := currentOrg(c)
	rows, _ := h.repo.ListMilestones(week, org, "", "")
	f := excelize.NewFile()
	sheet := "마일스톤"
	idx, _ := f.NewSheet(sheet)
	f.SetActiveSheet(idx)
	_ = f.SetCellValue(sheet, "A1", "주")
	headers := []string{"주", "조직", "영역", "축", "키", "접수", "조치", "이월", "신규", "정보", "활동", "견적", "수주", "계약", "상태"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
	}
	for i, row := range rows {
		vals := []interface{}{row.WeekStart, row.OrgID, row.Domain, row.Scope, row.ScopeKey,
			row.Received, row.Processed, row.Carried, row.SalesNew, row.SalesInfo, row.SalesAct,
			row.SalesQuote, row.SalesOrder, row.SalesContract, row.State}
		for j, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(j+1, i+2)
			_ = f.SetCellValue(sheet, cell, v)
		}
	}
	_ = f.DeleteSheet("Sheet1")
	name := fmt.Sprintf("마일스톤_%s.xlsx", week)
	c.Response().Header().Set(echo.HeaderContentType, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Response().Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	return f.Write(c.Response())
}

package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/auditlog"
	"customer-support/internal/repository"
)

func (h *StatsHandler) ListCompanyWeeklyRows(c echo.Context) error {
	org := currentOrg(c)
	if org == repository.OrgAll {
		rows, _ := h.repo.ListWeeklyReportRowsOrg(repository.OrgAll, false)
		return c.Render(http.StatusOK, "stats/company_weekly_rows.html", map[string]interface{}{
			"Title": "주간보고 행", "Active": NavStatsReports,
			"Rows": rows, "CanWrite": false, "OrgAll": true,
			"Hint": "조직을 선택해야 행을 등록·수정할 수 있습니다. 95-2 사업 업로드 전에는 활동 일반 행만 등록하세요.",
		})
	}
	orgID, err := repository.RequireInsertOrg(org)
	if err != nil {
		return echo.ErrForbidden
	}
	rows, err := h.repo.ListWeeklyReportRowsOrg(orgID, false)
	if err != nil {
		return err
	}
	projects, _ := h.repo.ListWeeklyRowProjects(orgID)
	return c.Render(http.StatusOK, "stats/company_weekly_rows.html", map[string]interface{}{
		"Title": "주간보고 행", "Active": NavStatsReports,
		"Rows": rows, "Projects": projects, "CanWrite": canWriteCompanyWeekly(c),
		"OrgName": currentOrgName(c),
		"Hint":    "사업 매핑 행은 95-2 사업 업로드 후에 등록할 수 있습니다. 그 전에는 활동 일반 행만 등록하세요.",
	})
}

func (h *StatsHandler) CreateCompanyWeeklyRow(c echo.Context) error {
	if !canWriteCompanyWeekly(c) {
		return echo.ErrForbidden
	}
	orgID, err := resolveInsertOrg(c)
	if err != nil {
		return echo.ErrForbidden
	}
	in := parseWeeklyRowForm(c, orgID)
	key, err := h.repo.InsertWeeklyReportRow(in)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	accessLog(c, auditlog.Record{
		Action: auditlog.ActionCreate, TargetTable: "weekly_report_rows", TargetID: key,
		Reason: "주간보고 행 등록", Result: auditlog.ResultOK,
	})
	return c.Redirect(http.StatusSeeOther, "/stats/company-weekly/rows")
}

func (h *StatsHandler) UpdateCompanyWeeklyRow(c echo.Context) error {
	if !canWriteCompanyWeekly(c) {
		return echo.ErrForbidden
	}
	orgID, err := resolveInsertOrg(c)
	if err != nil {
		return echo.ErrForbidden
	}
	in := parseWeeklyRowForm(c, orgID)
	in.Key = strings.TrimSpace(c.Param("key"))
	if err := h.repo.UpdateWeeklyReportRow(in); err != nil {
		if strings.Contains(err.Error(), "다른 조직") {
			return echo.ErrForbidden
		}
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	accessLog(c, auditlog.Record{
		Action: auditlog.ActionUpdate, TargetTable: "weekly_report_rows", TargetID: in.Key,
		Reason: "주간보고 행 수정", Result: auditlog.ResultOK,
	})
	return c.Redirect(http.StatusSeeOther, "/stats/company-weekly/rows")
}

func (h *StatsHandler) ToggleCompanyWeeklyRow(c echo.Context) error {
	if !canWriteCompanyWeekly(c) {
		return echo.ErrForbidden
	}
	orgID, err := resolveInsertOrg(c)
	if err != nil {
		return echo.ErrForbidden
	}
	off := c.FormValue("off") == "1"
	if err := h.repo.SetWeeklyReportRowActive(orgID, c.Param("key"), !off); err != nil {
		if strings.Contains(err.Error(), "다른 조직") {
			return echo.ErrForbidden
		}
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	accessLog(c, auditlog.Record{
		Action: auditlog.ActionUpdate, TargetTable: "weekly_report_rows", TargetID: c.Param("key"),
		Reason: "주간보고 행 활성", Result: auditlog.ResultOK,
	})
	return c.Redirect(http.StatusSeeOther, "/stats/company-weekly/rows")
}

func parseWeeklyRowForm(c echo.Context, orgID string) repository.WeeklyReportRowInput {
	sortN, _ := strconv.Atoi(strings.TrimSpace(c.FormValue("sort_order")))
	return repository.WeeklyReportRowInput{
		OrgID:        orgID,
		Division:     c.FormValue("division"),
		Team:         c.FormValue("team"),
		NoLabel:      c.FormValue("no_label"),
		DisplayName:  c.FormValue("display_name"),
		ProjectID:    c.FormValue("project_id"),
		Kind:         c.FormValue("row_kind"),
		ActivityKind: c.FormValue("activity_kind"),
		SortOrder:    sortN,
		Highlight:    c.FormValue("highlight") == "1",
		Active:       c.FormValue("is_active") != "0",
	}
}

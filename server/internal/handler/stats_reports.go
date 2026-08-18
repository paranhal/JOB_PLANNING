package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

// Reports 확인 > 보고서. 기존 엑셀 엔드포인트를 카드로 모은다. §16 · §16.5
func (h *StatsHandler) Reports(c echo.Context) error {
	now := time.Now()
	weeks := reportsWeekOptions(now)
	defaultWeek := now.Format("2006-01-02")
	if len(weeks) > 0 {
		defaultWeek = weeks[0].Value
	}
	if d := strings.TrimSpace(c.QueryParam("date")); d != "" {
		if t, err := time.ParseInLocation("2006-01-02", d, now.Location()); err == nil {
			wd := int(t.Weekday())
			if wd == 0 {
				wd = 7
			}
			defaultWeek = t.AddDate(0, 0, -(wd - 1)).Format("2006-01-02")
		}
	}

	var plans []model.MaintenancePlan
	if h.mntRepo != nil {
		plans, _ = h.mntRepo.ListPlans()
	}
	defaultPlanID := ""
	year := now.Year()
	for _, p := range plans {
		if p.PlanYear == year {
			defaultPlanID = p.PlanID
			break
		}
	}
	if defaultPlanID == "" && len(plans) > 0 {
		defaultPlanID = plans[0].PlanID
	}

	var holidayBanners []string
	seenY := map[int]bool{}
	addYear := func(y int) {
		if y <= 0 || seenY[y] {
			return
		}
		seenY[y] = true
		if msg := holidayMissingBanner(h.mntRepo.Holidays(), y); msg != "" {
			holidayBanners = append(holidayBanners, msg)
		}
	}
	addYear(year)
	for _, p := range plans {
		addYear(p.PlanYear)
	}
	if t, err := time.ParseInLocation("2006-01-02", defaultWeek, now.Location()); err == nil {
		addYear(t.Year())
	}

	companyDate := strings.TrimSpace(c.QueryParam("company_date"))
	companyAnchor := reportMeetingMonday(now)
	if t, err := time.ParseInLocation("2006-01-02", companyDate, now.Location()); err == nil {
		companyAnchor = t
	}
	var companyDraft model.CompanyWeeklyDraft
	if h.repo != nil {
		companyDraft, _ = h.repo.BuildCompanyWeeklyDraft(companyAnchor)
	}
	sheetName := strings.TrimSpace(c.QueryParam("sheet_name"))
	if sheetName == "" {
		sheetName = companyDraft.Period.SheetNameDefault
	}

	return c.Render(http.StatusOK, "stats/reports.html", map[string]interface{}{
		"Title":            "보고서",
		"Active":           "stats_reports",
		"WeekOptions":      weeks,
		"DefaultWeek":      defaultWeek,
		"MonthOptions":     monthOptions(now),
		"DefaultMonth":     now.Format("2006-01"),
		"Plans":            plans,
		"DefaultPlanID":    defaultPlanID,
		"MetricOptions":    reportsMetricOptions(),
		"ScopeOptions":     reportsScopeOptions(),
		"PeriodOptions":    reportsPeriodOptions(),
		"DefaultMetric":    model.StatsMetricProgress,
		"DefaultScope":     model.StatsScopeTeam,
		"DefaultPeriod":    model.StatsPeriodWeek,
		"DefaultMonthDate": now.Format("2006-01-02"),
		"HolidayBanners":   holidayBanners,
		"CompanyWeekly":    companyDraft,
		"CompanyDate":      companyDraft.Period.Anchor,
		"CompanySheetName": sheetName,
	})
}

func reportMeetingMonday(now time.Time) time.Time {
	loc := now.Location()
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, loc)
	switch today.Weekday() {
	case time.Monday:
		return today
	case time.Sunday:
		return today.AddDate(0, 0, 1)
	default:
		return today.AddDate(0, 0, 8-int(today.Weekday()))
	}
}

func reportsWeekOptions(now time.Time) []struct{ Value, Label string } {
	weeks := weekOptions(now)
	out := make([]struct{ Value, Label string }, 0, len(weeks))
	for _, w := range weeks {
		t, err := time.ParseInLocation("2006-01-02", w.Value, now.Location())
		if err != nil {
			out = append(out, w)
			continue
		}
		sun := t.AddDate(0, 0, 6)
		out = append(out, struct{ Value, Label string }{
			Value: w.Value,
			Label: t.Format("2006-01-02") + " ~ " + sun.Format("01-02"),
		})
	}
	return out
}

func reportsMetricOptions() []struct{ Value, Label string } {
	return []struct{ Value, Label string }{
		{model.StatsMetricProgress, "업무진행"},
		{model.StatsMetricCompleted, "완료업무"},
		{model.StatsMetricReceived, "접수업무"},
		{model.StatsMetricOverdue, "지연"},
	}
}

func reportsScopeOptions() []struct{ Value, Label string } {
	return []struct{ Value, Label string }{
		{model.StatsScopeTeam, "도서관사업팀 전체"},
		{model.StatsScopeAssignee, "담당자별"},
	}
}

func reportsPeriodOptions() []struct{ Value, Label string } {
	return []struct{ Value, Label string }{
		{model.StatsPeriodDay, "일별"},
		{model.StatsPeriodWeek, "주간별"},
		{model.StatsPeriodMonth, "월별"},
		{model.StatsPeriodRange, "원하는 기간"},
	}
}

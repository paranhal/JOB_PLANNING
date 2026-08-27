package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/service"
)

func parseRecurrenceForm(c echo.Context) model.WorkRecurrence {
	_ = c.Request().ParseForm()
	n := 1
	fmtScanInt(c.FormValue("interval_n"), &n)
	if n < 1 {
		n = 1
	}
	return model.WorkRecurrence{
		StartDate:             strings.TrimSpace(c.FormValue("start_date")),
		EndDate:               strings.TrimSpace(c.FormValue("end_date")),
		RuleType:              c.FormValue("rule_type"),
		IntervalN:             n,
		Weekdays:              model.JoinWeekdays(c.Request().PostForm["weekdays"]),
		HolidayPolicy:         c.FormValue("holiday_policy"),
		CompletePolicy:        c.FormValue("complete_policy"),
		ProgressIncludeFuture: c.FormValue("progress_include_future") == "1",
		ManualDates:           model.ParseDateList(c.FormValue("manual_dates")),
	}
}

func (h *WorkboardHandler) holidayYearMissing() func(int) bool {
	var hr interface{ YearMissing(int) bool }
	if h != nil && h.mntRepo != nil {
		if r := h.mntRepo.Holidays(); r != nil {
			hr = r
		}
	}
	return func(y int) bool {
		return hr != nil && hr.YearMissing(y)
	}
}

func defaultRecurrenceForm(t *model.WorkTask, today string) model.WorkRecurrence {
	start := ""
	end := ""
	if t != nil {
		start = strings.TrimSpace(t.WorkDate)
		if start == "" {
			start = strings.TrimSpace(t.DueDate)
		}
		end = strings.TrimSpace(t.DueDate)
	}
	if start == "" {
		start = today
	}
	if end == "" {
		end = start
	}
	return model.WorkRecurrence{
		StartDate:      start,
		EndDate:        end,
		RuleType:       model.RecurrenceNone,
		IntervalN:      1,
		HolidayPolicy:  model.HolidayPolicyAsIs,
		CompletePolicy: model.CompletePolicyManual,
	}
}

func recurrenceYears(from, to string) []int {
	seen := map[int]bool{}
	var years []int
	for _, s := range []string{from, to} {
		t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(s), time.Local)
		if err != nil {
			continue
		}
		y := t.Year()
		if seen[y] {
			continue
		}
		seen[y] = true
		years = append(years, y)
	}
	return years
}

func (h *WorkboardHandler) PreviewRecurrence(c echo.Context) error {
	if !canWriteWorkboard(c) {
		return echo.ErrForbidden
	}
	t, err := h.repo.GetTask(c.Param("id"))
	if err != nil || t == nil {
		return echo.ErrNotFound
	}
	if t.ParentTaskID != "" || t.SourceType != "" {
		return c.Redirect(http.StatusSeeOther, "/workboard/tasks/"+t.TaskID+"?err=rec_parent")
	}
	rule := parseRecurrenceForm(c)
	rule.TaskID = t.TaskID
	prev := service.ExpandOccurrencePreview(rule, service.DefaultCalendar(), h.holidayYearMissing())
	c.Set("recurrence_form", &rule)
	c.Set("recurrence_preview", &prev)
	return h.ShowTask(c)
}

func (h *WorkboardHandler) GenerateRecurrence(c echo.Context) error {
	return h.runOccurrenceWrite(c, false)
}

func (h *WorkboardHandler) RegenerateRecurrence(c echo.Context) error {
	return h.runOccurrenceWrite(c, true)
}

func (h *WorkboardHandler) runOccurrenceWrite(c echo.Context, regenerate bool) error {
	if !canWriteWorkboard(c) {
		return echo.ErrForbidden
	}
	t, err := h.repo.GetTask(c.Param("id"))
	if err != nil || t == nil {
		return echo.ErrNotFound
	}
	loc := "/workboard/tasks/" + t.TaskID
	if t.ParentTaskID != "" {
		return c.Redirect(http.StatusSeeOther, loc+"?err=rec_parent")
	}
	if t.SourceType != "" {
		return c.Redirect(http.StatusSeeOther, loc+"?err=rec_source")
	}
	rule := parseRecurrenceForm(c)
	rule.TaskID = t.TaskID
	prev := service.ExpandOccurrencePreview(rule, service.DefaultCalendar(), h.holidayYearMissing())
	if prev.RejectYear {
		return c.Redirect(http.StatusSeeOther, loc+"?err=rec_year")
	}
	if prev.Reject500 {
		return c.Redirect(http.StatusSeeOther, loc+"?err=rec_limit")
	}
	if prev.Count == 0 || prev.Error != "" && !prev.Warn200 {
		code := "rec_rule"
		if strings.Contains(prev.Error, "500") {
			code = "rec_limit"
		}
		return c.Redirect(http.StatusSeeOther, loc+"?err="+code)
	}
	if prev.Warn200 && c.FormValue("confirm_over_200") != "1" {
		return c.Redirect(http.StatusSeeOther, loc+"?err=rec_warn")
	}
	var res model.OccurrenceGenerateResult
	if regenerate {
		res, err = h.repo.RegenerateOccurrences(t, rule, prev.Dates)
	} else {
		res, err = h.repo.GenerateOccurrences(t, rule, prev.Dates)
	}
	if err != nil {
		if strings.Contains(err.Error(), "이미 실행 작업") {
			return c.Redirect(http.StatusSeeOther, loc+"?err=rec_exists")
		}
		return err
	}
	ok := "rec_gen"
	if regenerate {
		ok = "rec_regen"
	}
	return c.Redirect(http.StatusSeeOther, loc+"?ok="+ok+
		"&n="+fmt.Sprintf("%d", res.Created)+
		"&report="+url.QueryEscape(res.Report()))
}

package handler

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/service"
)

const overlapSkipKey = "overlap_skip_dates"

// PlanOverlapsForDates 기간/반복/하위 업무가 만들 모든 날짜의 겹침. 조회는 기간 1회.
func (h *WorkboardHandler) PlanOverlapsForDates(dates []string, start, end, excludeID string, names []string) []overlapHit {
	dates = uniqSortedDates(dates)
	if h == nil || h.repo == nil || len(dates) == 0 || strings.TrimSpace(start) == "" {
		return nil
	}
	from, to := dates[0], dates[len(dates)-1]
	rows, err := h.repo.ListAssigneeTimeOverlapsRange(from, to, start, end, excludeID, names)
	if err != nil {
		return nil
	}
	want := map[string]bool{}
	for _, d := range dates {
		want[d] = true
	}
	loaded := map[string]bool{}
	var hits []overlapHit
	for _, row := range rows {
		if !want[row.Date] {
			continue
		}
		t := row.Task
		if !loaded[t.TaskID] {
			_ = h.repo.LoadTaskActionFields(&t)
			loaded[t.TaskID] = true
		}
		hits = append(hits, overlapHit{Task: t, Assignee: row.Assignee, Date: row.Date})
	}
	return hits
}

func collectPlanDatesFromForm(c echo.Context, workDate, dueDate string, missing func(int) bool) []string {
	if c == nil {
		return enumerateInclusiveDates(workDate, firstNonEmpty(dueDate, workDate), 366)
	}
	_ = c.Request().ParseForm()
	if strings.TrimSpace(c.FormValue("recurrence_enabled")) == "1" {
		rule := parseRecurrenceForm(c)
		if model.NormalizeRecurrenceRuleType(rule.RuleType) != model.RecurrenceNone {
			prev := service.ExpandOccurrencePreview(rule, service.DefaultCalendar(), missing)
			if len(prev.Dates) > 0 {
				return uniqSortedDates(prev.Dates)
			}
		}
	}
	if mode := strings.TrimSpace(c.FormValue("sub_mode")); mode != "" {
		dates, err := collectDatesFromForm(mode, c.FormValue("daily_from"), c.FormValue("daily_to"), c.FormValue("sub_dates"))
		if err == nil && len(dates) > 0 {
			return uniqSortedDates(dates)
		}
	}
	end := strings.TrimSpace(dueDate)
	if end == "" {
		end = strings.TrimSpace(workDate)
	}
	return enumerateInclusiveDates(workDate, end, 366)
}

func overlapSkipDateSet(c echo.Context) map[string]bool {
	out := map[string]bool{}
	if c == nil {
		return out
	}
	if v, ok := c.Get(overlapSkipKey).([]string); ok {
		for _, d := range v {
			d = strings.TrimSpace(d)
			if d != "" {
				out[d] = true
			}
		}
	}
	if c.Request() == nil {
		return out
	}
	_ = c.Request().ParseForm()
	for _, raw := range append(append([]string{}, c.Request().PostForm["overlap_skip_date"]...), c.Request().PostForm["overlap_skip_date[]"]...) {
		d := strings.TrimSpace(raw)
		if d != "" {
			out[d] = true
		}
	}
	return out
}

func mergeOverlapSkipDates(c echo.Context, days []string) {
	if c == nil {
		return
	}
	set := overlapSkipDateSet(c)
	for _, d := range days {
		d = strings.TrimSpace(d)
		if d != "" {
			set[d] = true
		}
	}
	var list []string
	for d := range set {
		list = append(list, d)
	}
	sort.Strings(list)
	c.Set(overlapSkipKey, list)
}

func filterSkippedDates(dates []string, skip map[string]bool) []string {
	if len(skip) == 0 {
		return uniqSortedDates(dates)
	}
	var out []string
	for _, d := range dates {
		d = strings.TrimSpace(d)
		if d == "" || skip[d] {
			continue
		}
		out = append(out, d)
	}
	return uniqSortedDates(out)
}

func uniqSortedDates(dates []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, d := range dates {
		d = strings.TrimSpace(d)
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

func enumerateInclusiveDates(from, to string, max int) []string {
	from, to = strings.TrimSpace(from), strings.TrimSpace(to)
	if from == "" {
		return nil
	}
	if to == "" {
		to = from
	}
	a, err1 := time.ParseInLocation(dateLayout, from, time.Local)
	b, err2 := time.ParseInLocation(dateLayout, to, time.Local)
	if err1 != nil || err2 != nil || b.Before(a) {
		if err1 == nil {
			return []string{from}
		}
		return nil
	}
	var out []string
	for i := 0; !a.After(b); i++ {
		if i >= max {
			break
		}
		out = append(out, a.Format(dateLayout))
		a = a.AddDate(0, 0, 1)
	}
	return out
}

func groupOverlapDays(conflicts []overlapConflictJSON) []overlapDayJSON {
	order := []string{}
	idx := map[string]int{}
	for _, c := range conflicts {
		d := strings.TrimSpace(c.Date)
		if d == "" {
			d = "_"
		}
		i, ok := idx[d]
		if !ok {
			idx[d] = len(order)
			order = append(order, d)
			i = idx[d]
			_ = i
		}
	}
	days := make([]overlapDayJSON, 0, len(order))
	pos := map[string]int{}
	for _, d := range order {
		label := d
		if d == "_" {
			label = ""
		}
		pos[d] = len(days)
		days = append(days, overlapDayJSON{Date: label})
	}
	for _, c := range conflicts {
		d := strings.TrimSpace(c.Date)
		if d == "" {
			d = "_"
		}
		i := pos[d]
		days[i].Conflicts = append(days[i].Conflicts, c)
	}
	return days
}

func planSaveSummaryQuery(placed, split, skip int) string {
	q := "ok=task"
	if placed > 0 || split > 0 || skip > 0 {
		q = "ok=plan_saved"
	}
	if placed > 0 {
		q += "&placed=" + strconv.Itoa(placed)
	}
	if split > 0 {
		q += "&split=" + strconv.Itoa(split)
	}
	if skip > 0 {
		q += "&skip=" + strconv.Itoa(skip)
	}
	return q
}

func pendingSplitCount(c echo.Context) int {
	v, _ := c.Get("pending_interrupt").(*pendingInterrupt)
	if v == nil {
		return 0
	}
	seen := map[string]bool{}
	n := 0
	for _, hit := range v.Hits {
		id := strings.TrimSpace(hit.Task.TaskID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		n++
	}
	return n
}


package service

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"customer-support/internal/model"
)

// ExpandOccurrencePreview 실행 예정일 미리보기. 쓰기는 하지 않는다. §13.15.5
func ExpandOccurrencePreview(rule model.WorkRecurrence, cal *Calendar, holidayYearMissing func(int) bool) model.OccurrencePreview {
	out := model.OccurrencePreview{}
	rule.RuleType = model.NormalizeRecurrenceRuleType(rule.RuleType)
	rule.HolidayPolicy = model.NormalizeHolidayPolicy(rule.HolidayPolicy)
	if rule.IntervalN < 1 {
		rule.IntervalN = 1
	}
	if rule.RuleType == model.RecurrenceManual {
		rule.ManualDates = model.ParseDateList(strings.Join(rule.ManualDates, "\n"))
		if len(rule.ManualDates) == 0 {
			out.Error = "실행할 날짜를 고르세요."
			return out
		}
		if strings.TrimSpace(rule.StartDate) == "" {
			rule.StartDate = rule.ManualDates[0]
		}
		if strings.TrimSpace(rule.EndDate) == "" {
			rule.EndDate = rule.ManualDates[len(rule.ManualDates)-1]
		}
	}
	start, err1 := time.ParseInLocation("2006-01-02", strings.TrimSpace(rule.StartDate), time.Local)
	end, err2 := time.ParseInLocation("2006-01-02", strings.TrimSpace(rule.EndDate), time.Local)
	if err1 != nil || err2 != nil {
		out.Error = "시작일·최종 마감일을 확인하세요."
		return out
	}
	if rule.RuleType == model.RecurrenceNone || rule.RuleType == "" {
		out.Error = "반복 규칙을 선택하세요."
		return out
	}
	if rule.RuleType == model.RecurrenceWeeklyDOW && len(model.ParseWeekdays(rule.Weekdays)) == 0 {
		out.Error = "요일을 선택하세요."
		return out
	}
	if end.Before(start) && rule.RuleType != model.RecurrenceManual {
		out.Error = "최종 마감일이 시작일보다 앞입니다."
		return out
	}
	spanDays := int(end.Sub(start).Hours()/24) + 1
	if rule.RuleType == model.RecurrenceDaily && spanDays >= model.RecurrenceDailyYearN {
		out.RejectYear = true
		out.Error = "매일 × 1년은 만들 수 없습니다. 기간이나 주기를 줄이세요."
		return out
	}

	raw := rawOccurrenceDates(rule, start, end, cal)
	seen := map[string]bool{}
	yearHit := map[int]bool{}
	for _, d := range raw {
		t, err := time.ParseInLocation("2006-01-02", d, time.Local)
		if err == nil {
			yearHit[t.Year()] = true
		}
		var item model.OccurrencePreviewItem
		if rule.RuleType == model.RecurrenceManual {
			item = warnManualDate(d, cal)
		} else {
			item = applyHolidayPolicy(d, rule.HolidayPolicy, cal)
		}
		if item.Skipped || item.Date == "" {
			if item.Note != "" {
				out.Notes = append(out.Notes, item.Note)
			}
			continue
		}
		if rule.RuleType != model.RecurrenceManual && (item.Date < rule.StartDate || item.Date > rule.EndDate) {
			note := fmt.Sprintf("※ %s은 기간 밖이라 제외됩니다", model.FormatOccurrenceLabel(d))
			out.Notes = append(out.Notes, note)
			continue
		}
		if seen[item.Date] {
			if item.Moved && item.Note != "" {
				out.Notes = append(out.Notes, item.Note)
			}
			continue
		}
		seen[item.Date] = true
		out.Items = append(out.Items, item)
		out.Dates = append(out.Dates, item.Date)
		if item.Note != "" && (item.Moved || rule.RuleType == model.RecurrenceManual) {
			out.Notes = append(out.Notes, item.Note)
		}
	}
	out.Count = len(out.Dates)
	if out.Count == 0 && out.Error == "" {
		out.Error = "실행 예정일이 없습니다. 기간·주기·휴일 정책을 확인하세요."
	}
	out.Warn200 = out.Count > model.RecurrenceWarnLimit
	out.Reject500 = out.Count > model.RecurrenceRejectLimit
	if out.Reject500 {
		out.Error = fmt.Sprintf("실행 예정일이 %d건입니다. 500건을 넘으면 만들 수 없습니다.", out.Count)
	}
	if holidayYearMissing != nil {
		years := make([]int, 0, len(yearHit))
		for y := range yearHit {
			years = append(years, y)
		}
		sort.Ints(years)
		for _, y := range years {
			if holidayYearMissing(y) {
				out.HolidayBanners = append(out.HolidayBanners, model.HolidayMissingBanner(y))
			}
		}
	}
	return out
}

func rawOccurrenceDates(rule model.WorkRecurrence, start, end time.Time, cal *Calendar) []string {
	var out []string
	switch rule.RuleType {
	case model.RecurrenceManual:
		return append([]string{}, rule.ManualDates...)
	case model.RecurrenceDaily:
		for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
			out = append(out, d.Format("2006-01-02"))
		}
	case model.RecurrenceWeeklyDOW:
		want := model.ParseWeekdays(rule.Weekdays)
		if len(want) == 0 {
			return nil
		}
		for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
			if want[model.ISOWeekday(d)] {
				out = append(out, d.Format("2006-01-02"))
			}
		}
	case model.RecurrenceEveryNDays:
		for d := start; !d.After(end); d = d.AddDate(0, 0, rule.IntervalN) {
			out = append(out, d.Format("2006-01-02"))
		}
	case model.RecurrenceEveryNWeeks:
		for d := start; !d.After(end); d = d.AddDate(0, 0, 7*rule.IntervalN) {
			out = append(out, d.Format("2006-01-02"))
		}
	case model.RecurrenceMonthly:
		out = monthlyDates(rule, start, end, cal)
	case model.RecurrenceQuarterly:
		out = quarterlyDates(rule, start, end)
	case model.RecurrenceYearly:
		out = yearlyDates(rule, start, end)
	}
	return out
}

func monthDayOf(rule model.WorkRecurrence, fallback time.Time) int {
	d := rule.MonthDay
	if d < 1 {
		d = fallback.Day()
	}
	if d < 1 {
		d = 1
	}
	return d
}

func monthlyDates(rule model.WorkRecurrence, start, end time.Time, cal *Calendar) []string {
	var out []string
	cursor := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.Local)
	endMonth := time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, time.Local)
	day := monthDayOf(rule, start)
	for !cursor.After(endMonth) {
		var ds string
		if rule.LastWorkday {
			ds = lastWorkingDayOfMonth(cal, cursor.Year(), cursor.Month())
		} else {
			ds = model.ClampMonthDay(cursor.Year(), cursor.Month(), day).Format("2006-01-02")
		}
		if ds != "" && ds >= start.Format("2006-01-02") && ds <= end.Format("2006-01-02") {
			out = append(out, ds)
		}
		cursor = cursor.AddDate(0, 1, 0)
	}
	return out
}

func quarterlyDates(rule model.WorkRecurrence, start, end time.Time) []string {
	months := model.QuarterMonths(rule.MonthN)
	day := monthDayOf(rule, start)
	var out []string
	seen := map[string]bool{}
	for y := start.Year(); y <= end.Year(); y++ {
		for _, m := range months {
			ds := model.ClampMonthDay(y, time.Month(m), day).Format("2006-01-02")
			if ds < start.Format("2006-01-02") || ds > end.Format("2006-01-02") || seen[ds] {
				continue
			}
			seen[ds] = true
			out = append(out, ds)
		}
	}
	sort.Strings(out)
	return out
}

func yearlyDates(rule model.WorkRecurrence, start, end time.Time) []string {
	m := rule.MonthN
	d := rule.MonthDay
	if m < 1 || m > 12 {
		m = int(start.Month())
	}
	if d < 1 {
		d = start.Day()
	}
	var out []string
	for y := start.Year(); y <= end.Year(); y++ {
		ds := model.ClampMonthDay(y, time.Month(m), d).Format("2006-01-02")
		if ds >= start.Format("2006-01-02") && ds <= end.Format("2006-01-02") {
			out = append(out, ds)
		}
	}
	return out
}

func lastWorkingDayOfMonth(cal *Calendar, year int, month time.Month) string {
	if cal == nil {
		cal = DefaultCalendar()
	}
	if cal == nil {
		cal = &Calendar{}
	}
	return cal.LastWorkingDayOfMonth(year, month)
}

func warnManualDate(date string, cal *Calendar) model.OccurrencePreviewItem {
	item := model.OccurrencePreviewItem{RawDate: date, Date: date, Label: model.FormatOccurrenceLabel(date)}
	if cal == nil {
		cal = DefaultCalendar()
	}
	if cal == nil {
		cal = &Calendar{}
	}
	if cal.IsWorkingDay(date) {
		return item
	}
	hol, holName := cal.IsHoliday(date)
	reason := "주말"
	if hol && holName != "" {
		reason = holName
	}
	item.HolidayName = holName
	item.Note = fmt.Sprintf("※ %s은 %s입니다. 지정일자는 옮기지 않습니다", model.FormatOccurrenceLabel(date), reason)
	return item
}

func applyHolidayPolicy(date, policy string, cal *Calendar) model.OccurrencePreviewItem {
	item := model.OccurrencePreviewItem{RawDate: date, Date: date, Label: model.FormatOccurrenceLabel(date)}
	if cal == nil {
		cal = DefaultCalendar()
	}
	if cal == nil || policy == model.HolidayPolicyAsIs {
		return item
	}
	working := cal.IsWorkingDay(date)
	if working {
		return item
	}
	hol, holName := cal.IsHoliday(date)
	reason := "주말"
	if hol && holName != "" {
		reason = holName
	}
	switch policy {
	case model.HolidayPolicySkip:
		item.Skipped = true
		item.Date = ""
		item.Note = fmt.Sprintf("※ %s은 %s이라 제외됩니다", model.FormatOccurrenceLabel(date), reason)
		return item
	case model.HolidayPolicyNextWorkday:
		next := cal.NextWorkingDay(date)
		if next == "" || next == date {
			item.Skipped = true
			item.Date = ""
			return item
		}
		item.Date = next
		item.Label = model.FormatOccurrenceLabel(next)
		item.Moved = true
		item.HolidayName = holName
		item.Note = fmt.Sprintf("※ %s은 %s이라 %s로 옮겨집니다",
			model.FormatOccurrenceLabel(date), reason, model.FormatOccurrenceLabel(next))
		return item
	case model.HolidayPolicyPrevWorkday:
		prev := cal.PrevWorkingDay(date)
		if prev == "" || prev == date {
			item.Skipped = true
			item.Date = ""
			return item
		}
		item.Date = prev
		item.Label = model.FormatOccurrenceLabel(prev)
		item.Moved = true
		item.HolidayName = holName
		item.Note = fmt.Sprintf("※ %s은 %s이라 %s로 옮겨집니다",
			model.FormatOccurrenceLabel(date), reason, model.FormatOccurrenceLabel(prev))
		return item
	}
	return item
}

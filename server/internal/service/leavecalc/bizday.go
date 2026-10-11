package leavecalc

import (
	"time"
)

// HolidaySet is YYYY-MM-DD holidays/company off days (weekends are always excluded).
type HolidaySet map[string]struct{}

// NewHolidaySet builds a set from date strings. Invalid entries are skipped.
func NewHolidaySet(dates []string) HolidaySet {
	s := HolidaySet{}
	for _, d := range dates {
		t, err := parseDate(d)
		if err != nil {
			continue
		}
		s[formatDate(t)] = struct{}{}
	}
	return s
}

func (h HolidaySet) has(t time.Time) bool {
	if h == nil {
		return false
	}
	_, ok := h[formatDate(t)]
	return ok
}

// IsWeekend is Saturday or Sunday.
func IsWeekend(t time.Time) bool {
	w := dateOnly(t).Weekday()
	return w == time.Saturday || w == time.Sunday
}

// IsBusinessDay is not weekend and not in the provided holiday list.
func IsBusinessDay(t time.Time, holidays HolidaySet) bool {
	t = dateOnly(t)
	if IsWeekend(t) || holidays.has(t) {
		return false
	}
	return true
}

const bizWalkLimit = 366

// AdjustStart moves a holiday/weekend start to the next business day.
func AdjustStart(t time.Time, holidays HolidaySet) time.Time {
	t = dateOnly(t)
	for i := 0; i < bizWalkLimit && !IsBusinessDay(t, holidays); i++ {
		t = t.AddDate(0, 0, 1)
	}
	return t
}

// AdjustEnd moves a holiday/weekend end to the previous business day.
func AdjustEnd(t time.Time, holidays HolidaySet) time.Time {
	t = dateOnly(t)
	for i := 0; i < bizWalkLimit && !IsBusinessDay(t, holidays); i++ {
		t = t.AddDate(0, 0, -1)
	}
	return t
}

// BusinessDates returns inclusive business days after start/end holiday adjust.
// If the adjusted start is after the adjusted end, the result is empty.
func BusinessDates(start, end time.Time, holidays HolidaySet) []time.Time {
	s := AdjustStart(start, holidays)
	e := AdjustEnd(end, holidays)
	if cmpDate(s, e) > 0 {
		return nil
	}
	var out []time.Time
	for d := s; cmpDate(d, e) <= 0; d = d.AddDate(0, 0, 1) {
		if IsBusinessDay(d, holidays) {
			out = append(out, d)
		}
	}
	return out
}

// BusinessDayCount is len(BusinessDates). Same function for preview and server.
func BusinessDayCount(start, end time.Time, holidays HolidaySet) int {
	return len(BusinessDates(start, end, holidays))
}

// BusinessDatesISO parses YYYY-MM-DD and returns YYYY-MM-DD business days.
func BusinessDatesISO(start, end string, holidayDates []string) ([]string, error) {
	s, err := parseDate(start)
	if err != nil {
		return nil, err
	}
	e, err := parseDate(end)
	if err != nil {
		return nil, err
	}
	days := BusinessDates(s, e, NewHolidaySet(holidayDates))
	out := make([]string, len(days))
	for i, d := range days {
		out[i] = formatDate(d)
	}
	return out, nil
}

// BusinessDayCountISO is BusinessDayCount for YYYY-MM-DD strings.
func BusinessDayCountISO(start, end string, holidayDates []string) (int, error) {
	days, err := BusinessDatesISO(start, end, holidayDates)
	if err != nil {
		return 0, err
	}
	return len(days), nil
}

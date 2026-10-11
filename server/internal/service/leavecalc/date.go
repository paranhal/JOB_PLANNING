package leavecalc

import (
	"fmt"
	"strings"
	"time"
)

const DateLayout = "2006-01-02"

func parseDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if len(s) >= 10 {
		s = s[:10]
	}
	t, err := time.ParseInLocation(DateLayout, s, time.UTC)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date %q", s)
	}
	return dateOnly(t), nil
}

func parseDateOpt(s string) (time.Time, bool, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false, nil
	}
	t, err := parseDate(s)
	if err != nil {
		return time.Time{}, false, err
	}
	return t, true, nil
}

func dateOnly(t time.Time) time.Time {
	y, m, d := t.In(time.UTC).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func formatDate(t time.Time) string {
	return dateOnly(t).Format(DateLayout)
}

func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// addMonthsClamped adds calendar months. If the day does not exist
// (1/31→2월), it uses the last day of the target month (§4.2 월말).
func addMonthsClamped(t time.Time, months int) time.Time {
	t = dateOnly(t)
	y, m, d := t.Date()
	total := int(m) + months
	year := y
	for total > 12 {
		total -= 12
		year++
	}
	for total < 1 {
		total += 12
		year--
	}
	month := time.Month(total)
	last := daysInMonth(year, month)
	if d > last {
		d = last
	}
	return time.Date(year, month, d, 0, 0, 0, 0, time.UTC)
}

func addYearsClamped(t time.Time, years int) time.Time {
	return addMonthsClamped(t, years*12)
}

func cmpDate(a, b time.Time) int {
	a, b = dateOnly(a), dateOnly(b)
	switch {
	case a.Before(b):
		return -1
	case a.After(b):
		return 1
	default:
		return 0
	}
}

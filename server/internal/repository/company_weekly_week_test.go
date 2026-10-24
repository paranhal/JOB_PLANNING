package repository

import (
	"testing"
	"time"
)

func TestWeekInfoThursdayRule(t *testing.T) {
	loc := time.Local
	cases := []struct {
		mon        string
		year, month, n int
	}{
		{"2026-10-19", 2026, 10, 4},
		{"2026-09-28", 2026, 10, 1},
		{"2025-12-29", 2026, 1, 1},
		{"2026-12-28", 2026, 12, 5},
	}
	for _, tc := range cases {
		d, _ := time.ParseInLocation("2006-01-02", tc.mon, loc)
		w := WeekInfoOf(d)
		if w.Year != tc.year || w.Month != tc.month || w.WeekN != tc.n {
			t.Errorf("%s -> %d-%d W%d want %d-%d W%d", tc.mon, w.Year, w.Month, w.WeekN, tc.year, tc.month, tc.n)
		}
		if w.Monday.Weekday() != time.Monday || w.Sunday.Weekday() != time.Sunday {
			t.Errorf("%s 월~일 아님 %s %s", tc.mon, w.Monday, w.Sunday)
		}
	}
}

func TestWeeklyPlanWindowMatchesPeriod(t *testing.T) {
	mon := time.Date(2026, 8, 10, 0, 0, 0, 0, time.Local)
	w := WeeklyPlanWindow(mon)
	p := CompanyWeeklyPeriod(mon.AddDate(0, 0, 7))
	if w.WeekFrom != p.PrevFrom || w.PlanFrom != p.ThisFrom || w.PlanToEx != p.ThisToEx {
		t.Fatalf("%+v vs %+v", w, p)
	}
}

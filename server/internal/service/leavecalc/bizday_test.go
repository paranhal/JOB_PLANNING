package leavecalc

import (
	"reflect"
	"testing"
	"time"
)

func TestBusinessDaysWeekendAndHoliday(t *testing.T) {
	// 2026-09-24 Thu … 2026-09-28 Mon; 추석 9/24–26 가정
	holidays := []string{"2026-09-24", "2026-09-25", "2026-09-26"}
	got, err := BusinessDatesISO("2026-09-24", "2026-09-28", holidays)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"2026-09-28"}) {
		t.Fatalf("got %v", got)
	}
	n, err := BusinessDayCountISO("2026-09-25", "2026-09-25", holidays)
	if err != nil || n != 0 {
		t.Fatalf("single holiday n=%d err=%v", n, err)
	}
	n, err = BusinessDayCountISO("2026-08-22", "2026-08-23", nil) // Sat–Sun
	if err != nil || n != 0 {
		t.Fatalf("weekend n=%d", n)
	}
}

func TestBusinessDaysAdjustStartEnd(t *testing.T) {
	// start Sat 2026-08-22 → Mon 24; end Sun 2026-08-30 → Fri 28
	got, err := BusinessDatesISO("2026-08-22", "2026-08-30", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != "2026-08-24" || got[len(got)-1] != "2026-08-28" {
		t.Fatalf("adjusted range %v", got)
	}
	if len(got) != 5 {
		t.Fatalf("weekdays=%d %v", len(got), got)
	}
}

func TestBusinessDaysYearWrap(t *testing.T) {
	// 2025-12-31 Wed, 2026-01-01 Thu holiday, 2026-01-02 Fri
	got, err := BusinessDatesISO("2025-12-31", "2026-01-02", []string{"2026-01-01"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"2025-12-31", "2026-01-02"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestAdjustStartEndHelpers(t *testing.T) {
	h := NewHolidaySet([]string{"2026-01-01"})
	sat := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC) // Sat
	if formatDate(AdjustStart(sat, h)) != "2026-01-05" {
		t.Fatalf("sat start → Mon, got %s", formatDate(AdjustStart(sat, h)))
	}
	if formatDate(AdjustEnd(sat, h)) != "2026-01-02" {
		t.Fatalf("sat end → Fri, got %s", formatDate(AdjustEnd(sat, h)))
	}
	newYear := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if formatDate(AdjustStart(newYear, h)) != "2026-01-02" {
		t.Fatalf("holiday start=%s", formatDate(AdjustStart(newYear, h)))
	}
}

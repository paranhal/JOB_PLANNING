package service

import (
	"path/filepath"
	"testing"

	"customer-support/internal/repository"
)

func TestExpandLeaveWorkingDatesSkipsWeekendAndHoliday(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "leave.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	SetHolidayCalendar(NewCalendar(repository.NewHolidayRepo(db)))
	t.Cleanup(func() { SetHolidayCalendar(nil) })

	dates, err := ExpandLeaveWorkingDates("2026-09-24", "2026-09-28")
	if err != nil {
		t.Fatal(err)
	}
	if len(dates) != 1 || dates[0] != "2026-09-28" {
		t.Fatalf("추석 연휴 건너뛴 근무일=%v want [2026-09-28]", dates)
	}
	empty, err := ExpandLeaveWorkingDates("2026-09-25", "2026-09-25")
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("공휴일 단독=%v", empty)
	}
	weekend, err := ExpandLeaveWorkingDates("2026-08-22", "2026-08-23")
	if err != nil {
		t.Fatal(err)
	}
	if len(weekend) != 0 {
		t.Fatalf("주말=%v", weekend)
	}
}

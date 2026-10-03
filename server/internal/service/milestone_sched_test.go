package service

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/repository"
)

func TestWeekDeadlineHangulDayAndMissingYear(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "dl.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	SetHolidayCalendar(NewCalendar(repository.NewHolidayRepo(db)))
	t.Cleanup(func() { SetHolidayCalendar(nil) })
	_, _ = db.Exec(`INSERT OR IGNORE INTO holidays(holiday_date, holiday_year, name, kind) VALUES ('2026-10-09', 2026, '한글날', 'public')`)
	repo := repository.NewStatsRepo(db)
	mon := time.Date(2026, 10, 5, 0, 0, 0, 0, time.Local)
	at, day, missing := WeekDeadline(repo, mon)
	if missing {
		t.Fatal("2026 공휴일 미등록이면 안 된다")
	}
	if day != "2026-10-08" {
		t.Fatalf("마감일=%s want 2026-10-08", day)
	}
	if at.Hour() != 10 {
		t.Fatalf("hour=%d", at.Hour())
	}

	_, _ = db.Exec(`DELETE FROM holidays WHERE holiday_year=2099`)
	mon2 := time.Date(2099, 3, 2, 0, 0, 0, 0, time.Local)
	_, day2, missing2 := WeekDeadline(repo, mon2)
	if !missing2 {
		t.Fatal("2099 공휴일 미등록이어야 한다")
	}
	if day2 != "2099-03-06" {
		t.Fatalf("금요일=%s", day2)
	}
}

package handler

import (
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/repository"
	"customer-support/internal/service"
)

func TestToneForDateKoreanCalendar(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "tone.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	service.SetHolidayCalendar(service.NewCalendar(repository.NewHolidayRepo(db)))
	t.Cleanup(func() { service.SetHolidayCalendar(nil) })

	sat := toneForDate("2026-08-08", "")
	if sat.DayStyle != "color:#2563EB" || !sat.IsSaturday {
		t.Fatalf("토요일=%+v", sat)
	}
	sun := toneForDate("2026-08-09", "")
	if sun.DayStyle != "color:#DC2626" || !sun.IsSunday {
		t.Fatalf("일요일=%+v", sun)
	}
	hol := toneForDate("2026-05-05", "")
	if hol.DayStyle != "color:#DC2626" || hol.CellStyle != "background-color:#FEF2F2" || hol.HolidayName != "어린이날" {
		t.Fatalf("평일 공휴일=%+v", hol)
	}
	satHol := toneForDate("2026-08-15", "")
	if satHol.DayStyle != "color:#DC2626" || !strings.Contains(satHol.HolidayName, "광복절") {
		t.Fatalf("토요 공휴일은 빨강이어야 함 %+v", satHol)
	}
	today := toneForDate("2026-05-05", "2026-05-05")
	if today.DayStyle != "color:#1D4ED8" {
		t.Fatalf("오늘 강조가 휴일보다 우선이어야 함 %+v", today)
	}

	if err := repository.NewHolidayRepo(db).Create(model.Holiday{
		Date: "2026-07-15", Name: "창립기념일", Kind: model.HolidayKindCompany,
	}); err != nil {
		t.Fatal(err)
	}
	co := toneForDate("2026-07-15", "")
	applyHolidayKind(&co, model.HolidayKindCompany)
	if co.DayStyle != "color:#DC2626" || co.CellStyle != "background-color:#F3F4F6" {
		t.Fatalf("회사 휴무일=%+v", co)
	}
}

func TestGroupLeaveBadgesOverflow(t *testing.T) {
	g := groupLeaveBadges([]LeaveBadge{{Text: "a"}, {Text: "b"}, {Text: "c"}, {Text: "d"}})
	if len(g.Shown) != 3 || len(g.Hidden) != 1 || g.More != 1 {
		t.Fatalf("shown=%d hidden=%d more=%d", len(g.Shown), len(g.Hidden), g.More)
	}
}

func TestLeaveBadgesHiddenOnHoliday(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "badge.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	service.SetHolidayCalendar(service.NewCalendar(repository.NewHolidayRepo(db)))
	t.Cleanup(func() { service.SetHolidayCalendar(nil) })

	m := leaveBadgesByDate([]model.StaffLeave{
		{UserID: "u1", UserName: "최혜영", Date: "2026-05-05", Kind: model.LeaveKindAnnual},
		{UserID: "u1", UserName: "최혜영", Date: "2026-05-06", Kind: model.LeaveKindAnnual},
	}, "u1")
	if _, ok := m["2026-05-05"]; ok {
		t.Fatal("공휴일 칸에 연차 뱃지가 있으면 안 됨")
	}
	if len(m["2026-05-06"]) != 1 || m["2026-05-06"][0].Text != "최혜영 연차" || !m["2026-05-06"][0].Bold {
		t.Fatalf("평일 뱃지=%+v", m["2026-05-06"])
	}
	if m["2026-05-06"][0].Color != leaveBadgeAnnual {
		t.Fatalf("연차 색=%s", m["2026-05-06"][0].Color)
	}
}

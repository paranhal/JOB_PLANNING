package service

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestExpandEveryNDaysSeptember(t *testing.T) {
	prev := ExpandOccurrencePreview(model.WorkRecurrence{
		StartDate:      "2026-09-01",
		EndDate:        "2026-09-30",
		RuleType:       model.RecurrenceEveryNDays,
		IntervalN:      3,
		HolidayPolicy:  model.HolidayPolicyAsIs,
		CompletePolicy: model.CompletePolicyManual,
	}, NewCalendar(nil), nil)
	if prev.Error != "" || prev.Count != 10 {
		t.Fatalf("count=%d err=%s dates=%v", prev.Count, prev.Error, prev.Dates)
	}
	want := []string{
		"2026-09-01", "2026-09-04", "2026-09-07", "2026-09-10", "2026-09-13",
		"2026-09-16", "2026-09-19", "2026-09-22", "2026-09-25", "2026-09-28",
	}
	if strings.Join(prev.Dates, ",") != strings.Join(want, ",") {
		t.Fatalf("dates=%v want %v", prev.Dates, want)
	}
	if !strings.Contains(prev.SummaryLine(), "실행 예정일 10건이 생성됩니다.") {
		t.Fatalf("summary=%q", prev.SummaryLine())
	}
}

func TestExpandNextWorkdayMovesChuseok(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "rec.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	cal := NewCalendar(repository.NewHolidayRepo(db))

	prev := ExpandOccurrencePreview(model.WorkRecurrence{
		StartDate:     "2026-09-01",
		EndDate:       "2026-09-30",
		RuleType:      model.RecurrenceDaily,
		HolidayPolicy: model.HolidayPolicyNextWorkday,
	}, cal, repository.NewHolidayRepo(db).YearMissing)

	for _, d := range prev.Dates {
		if d == "2026-09-24" || d == "2026-09-25" || d == "2026-09-26" {
			t.Fatalf("추석이 그대로 남음: %v", prev.Dates)
		}
	}
	found := false
	for _, d := range prev.Dates {
		if d == "2026-09-28" {
			found = true
		}
	}
	if !found {
		t.Fatalf("다음 근무일 9/28이 없다: %v", prev.Dates)
	}
	joined := strings.Join(prev.Notes, "\n")
	if !strings.Contains(joined, "옮겨집니다") {
		t.Fatalf("이동 안내가 없다: %s", joined)
	}
	if !strings.Contains(joined, "추석") {
		t.Fatalf("추석 안내가 없다: %s", joined)
	}
}

func TestExpandDailyYearRejected(t *testing.T) {
	prev := ExpandOccurrencePreview(model.WorkRecurrence{
		StartDate:     "2026-01-01",
		EndDate:       "2026-12-31",
		RuleType:      model.RecurrenceDaily,
		HolidayPolicy: model.HolidayPolicyAsIs,
	}, NewCalendar(nil), nil)
	if !prev.RejectYear || prev.Count != 0 {
		t.Fatalf("reject=%v count=%d err=%s", prev.RejectYear, prev.Count, prev.Error)
	}
	if !strings.Contains(prev.Error, "매일") {
		t.Fatalf("err=%s", prev.Error)
	}
}

func TestExpandHolidayMissingBanner(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "miss.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	hr := repository.NewHolidayRepo(db)
	prev := ExpandOccurrencePreview(model.WorkRecurrence{
		StartDate:     "2028-03-01",
		EndDate:       "2028-03-05",
		RuleType:      model.RecurrenceDaily,
		HolidayPolicy: model.HolidayPolicySkip,
	}, NewCalendar(hr), hr.YearMissing)
	if len(prev.HolidayBanners) == 0 || !strings.Contains(prev.HolidayBanners[0], "2028년 휴무일 미등록") {
		t.Fatalf("banners=%v", prev.HolidayBanners)
	}
}

func TestNextWorkingDayUsesIsWorkingDay(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "nwd.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	cal := NewCalendar(repository.NewHolidayRepo(db))
	if got := cal.NextWorkingDay("2026-09-24"); got != "2026-09-28" {
		t.Fatalf("next=%s want 2026-09-28", got)
	}
	if got := cal.PrevWorkingDay("2026-09-24"); got != "2026-09-23" {
		t.Fatalf("prev=%s want 2026-09-23", got)
	}
}

func datesOf(prev model.OccurrencePreview) string {
	return strings.Join(prev.Dates, ",")
}

func TestExpandMonthlyDaySeptemberToNovember(t *testing.T) {
	prev := ExpandOccurrencePreview(model.WorkRecurrence{
		StartDate:     "2026-09-01",
		EndDate:       "2026-11-30",
		RuleType:      model.RecurrenceMonthly,
		MonthDay:      15,
		HolidayPolicy: model.HolidayPolicyAsIs,
	}, NewCalendar(nil), nil)
	if prev.Error != "" || datesOf(prev) != "2026-09-15,2026-10-15,2026-11-15" {
		t.Fatalf("count=%d err=%s dates=%v", prev.Count, prev.Error, prev.Dates)
	}
}

func TestExpandMonthlyLastWorkdayOctober2026(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "lastwd.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	cal := NewCalendar(repository.NewHolidayRepo(db))
	if got := cal.LastWorkingDayOfMonth(2026, time.October); got != "2026-10-30" {
		t.Fatalf("last workday=%s want 2026-10-30", got)
	}
	prev := ExpandOccurrencePreview(model.WorkRecurrence{
		StartDate:     "2026-10-01",
		EndDate:       "2026-10-31",
		RuleType:      model.RecurrenceMonthly,
		LastWorkday:   true,
		HolidayPolicy: model.HolidayPolicyAsIs,
	}, cal, nil)
	if prev.Error != "" || datesOf(prev) != "2026-10-30" {
		t.Fatalf("count=%d err=%s dates=%v", prev.Count, prev.Error, prev.Dates)
	}
}

func TestExpandQuarterlyJanuaryStart2026(t *testing.T) {
	prev := ExpandOccurrencePreview(model.WorkRecurrence{
		StartDate:     "2026-01-01",
		EndDate:       "2026-12-31",
		RuleType:      model.RecurrenceQuarterly,
		MonthN:        1,
		MonthDay:      1,
		HolidayPolicy: model.HolidayPolicyAsIs,
	}, NewCalendar(nil), nil)
	if prev.Error != "" || datesOf(prev) != "2026-01-01,2026-04-01,2026-07-01,2026-10-01" {
		t.Fatalf("count=%d err=%s dates=%v", prev.Count, prev.Error, prev.Dates)
	}
}

func TestExpandYearlyMarch15(t *testing.T) {
	prev := ExpandOccurrencePreview(model.WorkRecurrence{
		StartDate:     "2026-01-01",
		EndDate:       "2026-12-31",
		RuleType:      model.RecurrenceYearly,
		MonthN:        3,
		MonthDay:      15,
		HolidayPolicy: model.HolidayPolicyAsIs,
	}, NewCalendar(nil), nil)
	if prev.Error != "" || datesOf(prev) != "2026-03-15" {
		t.Fatalf("count=%d err=%s dates=%v", prev.Count, prev.Error, prev.Dates)
	}
}

func TestExpandManualWeekendAndChuseokStayPut(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "manual.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	cal := NewCalendar(repository.NewHolidayRepo(db))
	prev := ExpandOccurrencePreview(model.WorkRecurrence{
		RuleType:      model.RecurrenceManual,
		ManualDates:   []string{"2026-09-24", "2026-09-26", "2026-09-27"},
		HolidayPolicy: model.HolidayPolicyNextWorkday,
	}, cal, nil)
	if prev.Error != "" || datesOf(prev) != "2026-09-24,2026-09-26,2026-09-27" {
		t.Fatalf("지정일자가 옮겨졌다 count=%d err=%s dates=%v", prev.Count, prev.Error, prev.Dates)
	}
	joined := strings.Join(prev.Notes, "\n")
	if strings.Contains(joined, "옮겨집니다") {
		t.Fatalf("지정일자에 이동 안내가 있으면 안 된다: %s", joined)
	}
	if !strings.Contains(joined, "옮기지 않습니다") {
		t.Fatalf("지정일자 경고가 없다: %s", joined)
	}
	if !strings.Contains(joined, "추석") {
		t.Fatalf("추석 경고가 없다: %s", joined)
	}
}

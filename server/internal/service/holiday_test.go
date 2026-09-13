package service

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestIsHolidayNotWeekend(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "cal.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cal := NewCalendar(repository.NewHolidayRepo(db))

	// 토요일은 휴일 테이블에 없어도 IsHoliday=false. 예전 IsKRHoliday 는 true 였다.
	ok, name := cal.IsHoliday("2026-08-08")
	if ok || name != "" {
		t.Fatalf("토요일을 공휴일로 보면 안 됨 ok=%v name=%s", ok, name)
	}
	if cal.IsWorkingDay("2026-08-08") {
		t.Fatal("토요일은 근무일이 아니다")
	}

	ok, name = cal.IsHoliday("2026-05-05")
	if !ok || name == "" {
		t.Fatalf("어린이날 ok=%v name=%s", ok, name)
	}
	if cal.IsWorkingDay("2026-05-05") {
		t.Fatal("어린이날은 근무일이 아니다")
	}

	ok, _ = cal.IsHoliday("2026-10-06")
	if ok {
		t.Fatal("2026-10-06 은 오기재 추석 복사다")
	}
	if !cal.IsWorkingDay("2026-10-06") {
		t.Fatal("2026-10-06 화요일은 근무일이다")
	}

	ok, name = cal.IsHoliday("2026-09-25")
	if !ok || name != "추석" {
		t.Fatalf("추석 ok=%v name=%s", ok, name)
	}

	// 광복절이자 토요일 — 공휴일이지만 색은 IsHoliday 로만 빨강
	ok, _ = cal.IsHoliday("2026-08-15")
	if !ok {
		t.Fatal("2026-08-15 광복절")
	}
	if cal.IsWorkingDay("2026-08-15") {
		t.Fatal("광복절 토요일은 근무일이 아니다")
	}

	ok, _ = cal.IsHoliday("2028-03-01")
	if ok {
		t.Fatal("미등록 연도 평일은 테이블에 없으면 휴일이 아니다")
	}
	if !cal.IsWorkingDay("2028-03-01") {
		t.Fatal("미등록 연도 수요일은 근무일")
	}
}

func TestWorkingDaysBetweenInclusive(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "wd.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cal := NewCalendar(repository.NewHolidayRepo(db))
	// 2026-08-10(월)~2026-08-14(금). 8/15는 토·광복절.
	n := cal.WorkingDaysBetween("2026-08-10", "2026-08-14")
	if n != 5 {
		t.Fatalf("근무일=%d want 5", n)
	}
	n = cal.WorkingDaysBetween("2026-08-10", "2026-08-17")
	// 15 토, 16 일, 17 월(대체공휴일)
	if n != 5 {
		t.Fatalf("광복절 연휴 포함=%d want 5", n)
	}
	if cal.WorkingDaysBetween("2026-08-14", "2026-08-10") != 0 {
		t.Fatal("역순은 0")
	}
}

func TestLeadBusinessDaysSection414(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "lead.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cal := NewCalendar(repository.NewHolidayRepo(db))
	d, ok := cal.LeadBusinessDays("2026-08-14", "2026-08-14")
	if !ok || d != 0 {
		t.Fatalf("같은 날=%d ok=%v", d, ok)
	}
	d, ok = cal.LeadBusinessDays("2026-08-14", "2026-08-17") // 금→월(대체공휴일) → 앞 근무일 금 = 0? 17 is holiday so prev=14, same 0
	// 금요일 접수 → 월요일 완료(월이 근무일이면 1). 2026-08-17은 대체공휴일.
	d, ok = cal.LeadBusinessDays("2026-08-07", "2026-08-10") // 금→월
	if !ok || d != 1 {
		t.Fatalf("금→월=%d ok=%v want 1", d, ok)
	}
	d, ok = cal.LeadBusinessDays("2026-08-08", "2026-08-10") // 토→월
	if !ok || d != 0 {
		t.Fatalf("토→월=%d ok=%v want 0", d, ok)
	}
	d, ok = cal.LeadBusinessDays("2026-07-31", "2026-08-06")
	if !ok || d != 4 {
		t.Fatalf("7/31→8/6=%d want 4", d)
	}
	d, ok = cal.LeadBusinessDays("2026-07-30", "2026-08-05")
	if !ok || d != 4 {
		t.Fatalf("7/30→8/5=%d want 4", d)
	}
	d, ok = cal.LeadBusinessDays("2026-08-05", "2026-08-17")
	if !ok || d != 7 {
		t.Fatalf("8/5→8/17=%d want 7", d)
	}
}

func TestHolidayCacheInvalidatesOnTableChange(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := repository.NewHolidayRepo(db)
	cal := NewCalendar(repo)
	if cal.IsWorkingDay("2028-07-17") != true {
		t.Fatal("빈 연도 평일")
	}
	ok, _ := cal.IsHoliday("2028-07-17")
	if ok {
		t.Fatal("등록 전")
	}
	if err := repo.Create(model.Holiday{Date: "2028-07-17", Name: "창립기념일", Kind: model.HolidayKindCompany}); err != nil {
		t.Fatal(err)
	}
	ok, name := cal.IsHoliday("2028-07-17")
	if !ok || name != "창립기념일" {
		t.Fatalf("캐시가 안 비워짐 ok=%v name=%s", ok, name)
	}
	if cal.IsWorkingDay("2028-07-17") {
		t.Fatal("회사 휴무일은 근무일이 아니다")
	}
}

func TestHolidaySeed2026(t *testing.T) {
	items := repository.HolidaySeedItems()
	seen := map[string]bool{}
	n2026 := 0
	for _, h := range items {
		if h.Date == "" || h.Name == "" || !model.ValidHolidayKind(h.Kind) {
			t.Fatalf("잘못된 시드 %+v", h)
		}
		seen[h.Date] = true
		if h.Year == 0 && len(h.Date) >= 4 {
			// Year 는 insert 시 날짜에서 채운다
		}
		if h.Date[:4] == "2026" {
			n2026++
		}
	}
	if n2026 != 20 {
		t.Fatalf("2026 시드=%d want 20", n2026)
	}
	for _, d := range []string{"2026-05-24", "2026-09-24", "2026-09-25", "2026-09-26"} {
		if !seen[d] {
			t.Fatalf("필수 추가 누락 %s", d)
		}
	}
	for _, d := range []string{"2026-10-06", "2026-10-07", "2026-10-08"} {
		if seen[d] {
			t.Fatalf("오기재 추석 복사가 남음 %s", d)
		}
	}
}

func TestApplyHolidaySourceRemovesCopiedChuseokKeepsManual(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "fix.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := repository.NewHolidayRepo(db)
	if _, err := db.Exec(`
		INSERT INTO holidays (holiday_date, holiday_year, name, kind, source)
		VALUES ('2026-10-07', 2026, '추석 연휴', 'public', 'api')`); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(model.Holiday{Date: "2026-12-31", Name: "창립", Kind: model.HolidayKindCompany}); err != nil {
		t.Fatal(err)
	}
	repository.ReapplyHolidaySource(db)
	ok, _ := repo.HasDate("2026-10-07")
	if ok {
		t.Fatal("10-07이 남아 있다")
	}
	ok, _ = repo.HasDate("2026-09-25")
	if !ok {
		t.Fatal("09-25가 없다")
	}
	h, err := repo.Get("2026-12-31")
	if err != nil || h == nil || h.Source != model.HolidaySourceManual {
		t.Fatalf("회사 휴무일을 지우면 안 됨 %+v err=%v", h, err)
	}
}

func TestPackageFuncsUseDefaultCalendar(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "pkg.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	SetHolidayCalendar(NewCalendar(repository.NewHolidayRepo(db)))
	t.Cleanup(func() { SetHolidayCalendar(nil) })
	ok, _ := IsHoliday("2026-09-25")
	if !ok {
		t.Fatal("기본 달력 추석")
	}
	if IsWorkingDay("2026-08-08") {
		t.Fatal("토요일")
	}
	if WorkingDaysBetween("2026-08-10", "2026-08-14") != 5 {
		t.Fatal("패키지 WorkingDaysBetween")
	}
}

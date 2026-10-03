package repository

import (
	"path/filepath"
	"testing"
)

func TestBusinessDaysFridayToMondayAndHolidayVisit(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "bd.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// 시드에 2026-08-15 광복절·08-17 대체공휴일이 있다.
	span := func(from, to string) int {
		t.Helper()
		_, nextFrom, _, err := lookupBusinessDay(db, from)
		if err != nil {
			t.Fatalf("%s: %v", from, err)
		}
		bdTo, _, _, err := lookupBusinessDay(db, to)
		if err != nil {
			t.Fatalf("%s: %v", to, err)
		}
		return bdTo - nextFrom
	}

	if got := span("2026-07-31", "2026-08-03"); got != 1 {
		t.Fatalf("금→월 = %d want 1 (달력 3일 아님)", got)
	}
	if got := span("2026-08-13", "2026-08-17"); got != 1 {
		t.Fatalf("목→대체공휴일 방문 = %d want 1 (음수 아님)", got)
	}
	if got := span("2026-08-05", "2026-08-05"); got != 0 {
		t.Fatalf("같은 날 = %d want 0", got)
	}
	if got := span("2026-08-01", "2026-08-03"); got < 0 {
		t.Fatalf("토요일 접수→월 완료 음수: %d", got)
	}

	// 공휴일 수가 바뀌면 그 해부터 다시 채운다.
	var before int
	if err := db.QueryRow(`SELECT bd_index FROM business_days WHERE d='2026-08-18'`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO holidays(holiday_date, holiday_year, name, kind) VALUES ('2026-08-18', 2026, '임시휴일', 'company')`); err != nil {
		t.Fatal(err)
	}
	if err := RebuildBusinessDays(db, 0); err != nil {
		t.Fatal(err)
	}
	var after int
	if err := db.QueryRow(`SELECT is_workday, bd_index FROM business_days WHERE d='2026-08-18'`).Scan(new(int), &after); err != nil {
		t.Fatal(err)
	}
	var work int
	if err := db.QueryRow(`SELECT is_workday FROM business_days WHERE d='2026-08-18'`).Scan(&work); err != nil {
		t.Fatal(err)
	}
	if work != 0 {
		t.Fatal("회사 휴무일을 영업일로 남겼다")
	}
	if after >= before {
		t.Fatalf("다시 채운 뒤 bd_index 가 줄어야 한다 before=%d after=%d", before, after)
	}
}

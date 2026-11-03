package repository

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/appclock"
	"customer-support/internal/model"
)

func TestYearRollover2027LaborWorkTodayWeekly(t *testing.T) {
	restore := appclock.SetFixed(time.Date(2027, 1, 1, 9, 0, 0, 0, time.Local))
	t.Cleanup(restore)

	db, err := InitDB(filepath.Join(t.TempDir(), "rollover-2027.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	quotes := NewQuoteRepo(db)
	use, missing, note := quotes.ResolveLaborYear(0)
	if use <= 0 {
		t.Fatalf("노임 연도=%d missing=%v note=%s", use, missing, note)
	}
	rates, err := quotes.ListLaborRates(use)
	if err != nil {
		t.Fatal(err)
	}
	if len(rates) == 0 {
		t.Fatal("시드 연도 단가가 없다")
	}

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('C-27','해넘이도서관','해넘이도서관',1,'O01')`); err != nil {
		t.Fatal(err)
	}
	today := appclock.Now().Format("2006-01-02")
	if today != "2027-01-01" {
		t.Fatalf("clock today=%s", today)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, receipt_datetime, customer_id, symptom, status,
		assigned_to, visit_scheduled_date, schedule_confirmed, org_id, data_origin
	) VALUES ('AS-27','R2701-001','2026-12-20 10:00:00','C-27','해넘이','in_progress','테크',?,1,'O01','app')`, today); err != nil {
		t.Fatal(err)
	}

	items, err := NewWorkBoardRepo(db).ListWorkToday("", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	_ = items

	from, toEx := "2026-12-28", "2027-01-04"
	if err := NewStatsRepo(db).FillPeriodOverview(BuildStatsPeriodColumns(model.StatsViewWeek, appclock.Now()), ParseMeetingFilter(model.StatsScopeTeam, "", "")); err != nil {
		t.Fatalf("주간 개요: %v", err)
	}
	_ = from
	_ = toEx
}

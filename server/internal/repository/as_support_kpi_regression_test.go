package repository

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestSupportDoesNotChangeCompleteLeadKPI(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "kpi_sup.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('c1','도서관','도서관',1)`); err != nil {
		t.Fatal(err)
	}
	setMetricsPolicy(t, db, "2026-08-01", "as,maintenance")
	_, err = db.Exec(`
		INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date, start_datetime, status, assigned_to, complete_datetime, process_type, visit_date)
		VALUES
		('a1','R1','c1','2026-08-01 09:00:00','2026-08-03','2026-08-03','completed','양기헌','2026-08-05','visit','2026-08-03')`)
	if err != nil {
		t.Fatal(err)
	}
	repo := NewStatsRepo(db)
	anchor := time.Date(2026, 8, 7, 0, 0, 0, 0, time.Local)
	cols := BuildStatsPeriodColumns(model.StatsViewMonth, anchor)
	f := ParseMeetingFilter(model.StatsScopeTeam, "", "")
	before, err := repo.LoadStatsKPI(model.StatsViewMonth, cols, f)
	if err != nil {
		t.Fatal(err)
	}
	w := &model.ASWorkItem{ASID: "a1"}
	if err := NewASWorkRepo(db).CreateSupport(w, model.ASSupportDetail{
		SupportType: model.SupportFactoryInbound, VendorName: "한빛", DueDate: "2026-08-10", ActionDate: "2026-08-04",
	}); err != nil {
		t.Fatal(err)
	}
	after, err := repo.LoadStatsKPI(model.StatsViewMonth, cols, f)
	if err != nil {
		t.Fatal(err)
	}
	if before.CompleteAvgDays != after.CompleteAvgDays || before.CompleteSample != after.CompleteSample {
		t.Fatalf("KPI changed before=%+v after=%+v", before.CompleteDisplay, after.CompleteDisplay)
	}
	if before.VisitAvgDays != after.VisitAvgDays || before.VisitToDoneAvgDays != after.VisitToDoneAvgDays {
		t.Fatalf("visit KPI changed")
	}
	card := repo.LoadFactoryWaitCard("2026-08-01", "2026-09-01")
	if card.Count < 1 {
		t.Fatalf("factory wait card empty: %+v from=%s to=%s", card, cols[1].From, cols[1].ToExclusive)
	}
}

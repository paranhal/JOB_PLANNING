package repository

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestV214RollbackStatsBaseline(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "v214_stats.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c1','도서관','도서관',1)`); err != nil {
		t.Fatal(err)
	}
	setMetricsPolicy(t, db, "2026-08-01", "as,maintenance")
	if _, err := db.Exec(`
		INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date,
			start_datetime, status, assigned_to, complete_datetime, data_origin)
		VALUES ('a1','R1','c1','2026-08-01','2026-08-03','2026-08-03','completed','양기헌','2026-08-05','app')`); err != nil {
		t.Fatal(err)
	}

	repo := NewStatsRepo(db)
	anchor := time.Date(2026, 8, 7, 0, 0, 0, 0, time.Local)
	cols := BuildStatsPeriodColumns(model.StatsViewMonth, anchor)
	f := ParseMeetingFilter(model.StatsScopeTeam, "", "")
	if err := repo.FillPeriodOverview(cols, f); err != nil {
		t.Fatal(err)
	}
	kpi, err := repo.LoadStatsKPI(model.StatsViewMonth, cols, f)
	if err != nil {
		t.Fatal(err)
	}
	an, err := repo.LoadStatsWorkAnalysis("2026-08-01", "2026-09-01", f)
	if err != nil {
		t.Fatal(err)
	}
	events, err := repo.listWeeklyASEvents("2026-08-01", "2026-09-01")
	if err != nil {
		t.Fatal(err)
	}

	got := fmt.Sprintf("exec=%.6f visit=%.6f complete=%.6f receipt=%d completed=%d events=%d",
		kpi.ExecutionRate, kpi.VisitAvgDays, kpi.CompleteAvgDays, an.AS.Receipt, an.AS.Completed, len(events))
	t.Log(got)

	const want = "exec=0.000000 visit=2.000000 complete=4.000000 receipt=1 completed=1 events=2"
	if got != want {
		t.Fatalf("통계가 이관 전과 다름\n got %s\nwant %s", got, want)
	}
}

func TestV214RollbackDropsLegacyColumns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.db")
	db, err := InitDB(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`ALTER TABLE work_projects ADD COLUMN project_kind TEXT NOT NULL DEFAULT 'maintenance'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`ALTER TABLE work_projects ADD COLUMN sales_stage TEXT`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO codes (code_id, code_group, code_value, code_name, sort_order)
		VALUES ('PK001','project_kind','maintenance','유지보수',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM id_sequences WHERE seq_key=?`, v214ProjectKindRollbackMeta); err != nil {
		t.Fatal(err)
	}
	db.Close()

	db, err = InitDB(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if workProjectHasColumn(db, "project_kind") {
		t.Fatal("project_kind 컬럼이 남았다")
	}
	if workProjectHasColumn(db, "sales_stage") {
		t.Fatal("sales_stage 컬럼이 남았다")
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM codes WHERE code_group='project_kind'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("project_kind 코드 n=%d", n)
	}
}

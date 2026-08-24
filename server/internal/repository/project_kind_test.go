package repository

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestProjectKindSeedAndCodes(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "kind.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	for _, id := range []string{"WPSEED01", "WPSEED02", "WPSEED03", "WPSEED04", "WPSEED05", "WPSEED06"} {
		var kind string
		if err := db.QueryRow(`SELECT COALESCE(project_kind,'') FROM work_projects WHERE project_id=?`, id).Scan(&kind); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if kind != model.ProjectKindMaintenance {
			t.Fatalf("%s kind=%q want maintenance", id, kind)
		}
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM codes WHERE code_group='project_kind'`).Scan(&n); err != nil || n != 5 {
		t.Fatalf("project_kind codes n=%d err=%v", n, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM codes WHERE code_group='weekly_ref_project_kind'`).Scan(&n); err != nil || n < 6 {
		t.Fatalf("weekly_ref_project_kind codes n=%d err=%v", n, err)
	}
	var ref string
	if err := db.QueryRow(`SELECT code_name FROM codes WHERE code_group='weekly_ref_project_kind' AND code_value=? ORDER BY sort_order LIMIT 1`,
		model.ProjectKindMaintenance).Scan(&ref); err != nil {
		t.Fatal(err)
	}
	if ref != "4_인프라_유지_관리" {
		t.Fatalf("maintenance ref=%q", ref)
	}
}

func TestSalesProjectExcludedFromStatsKPI(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "kind_stats.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c1','도서관','도서관',1)`); err != nil {
		t.Fatal(err)
	}
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
	before, err := repo.LoadStatsKPI(model.StatsViewMonth, cols, f)
	if err != nil {
		t.Fatal(err)
	}
	beforeAn, err := repo.LoadStatsWorkAnalysis("2026-08-01", "2026-09-01", f)
	if err != nil {
		t.Fatal(err)
	}
	beforeEvents, err := repo.listWeeklyASEvents("2026-08-01", "2026-09-01")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(`
		INSERT INTO work_projects (project_id, name, short_name, plan_year, is_paid, sort_order, color, status, project_kind)
		VALUES ('WP-SALES','영업건','영업',2026,1,99,'#000','active','build')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date,
			start_datetime, status, assigned_to, complete_datetime, project_id, data_origin)
		VALUES ('a-sales','RS','c1','2026-08-02','2026-08-04','2026-08-04','completed','양기헌','2026-08-06','WP-SALES','app')`); err != nil {
		t.Fatal(err)
	}

	cols2 := BuildStatsPeriodColumns(model.StatsViewMonth, anchor)
	if err := repo.FillPeriodOverview(cols2, f); err != nil {
		t.Fatal(err)
	}
	after, err := repo.LoadStatsKPI(model.StatsViewMonth, cols2, f)
	if err != nil {
		t.Fatal(err)
	}
	if after.VisitAvgDays != before.VisitAvgDays || after.CompleteAvgDays != before.CompleteAvgDays {
		t.Fatalf("리드타임이 바뀜 visit %v→%v complete %v→%v",
			before.VisitAvgDays, after.VisitAvgDays, before.CompleteAvgDays, after.CompleteAvgDays)
	}
	if after.ExecutionRate != before.ExecutionRate {
		t.Fatalf("실행률이 바뀜 %.1f→%.1f", before.ExecutionRate, after.ExecutionRate)
	}

	afterAn, err := repo.LoadStatsWorkAnalysis("2026-08-01", "2026-09-01", f)
	if err != nil {
		t.Fatal(err)
	}
	if afterAn.AS.Receipt != beforeAn.AS.Receipt || afterAn.AS.Completed != beforeAn.AS.Completed {
		t.Fatalf("업무분석 AS가 바뀜 receipt %d→%d completed %d→%d",
			beforeAn.AS.Receipt, afterAn.AS.Receipt, beforeAn.AS.Completed, afterAn.AS.Completed)
	}
	afterEvents, err := repo.listWeeklyASEvents("2026-08-01", "2026-09-01")
	if err != nil {
		t.Fatal(err)
	}
	if len(afterEvents) != len(beforeEvents) {
		t.Fatalf("주간보고 AS 이벤트가 바뀜 %d→%d", len(beforeEvents), len(afterEvents))
	}
	nSales, err := repo.countCompanyASDone("WP-SALES", "2026-08-01", "2026-09-01")
	if err != nil {
		t.Fatal(err)
	}
	if nSales != 0 {
		t.Fatalf("영업 사업 주간보고 AS 건수=%d want 0", nSales)
	}

	list, err := NewProjectRepo(db).List(2026, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range list {
		if p.ProjectID == "WP-SALES" {
			t.Fatal("기본 목록에 영업 건이 보인다")
		}
	}
}

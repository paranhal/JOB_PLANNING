package repository

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestAvgASVisitLeadTimeSkipsMissingVisitDate(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "visit.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('c1','도서관','도서관',1)`); err != nil {
		t.Fatal(err)
	}
	setMetricsPolicy(t, db, "2026-08-01", "as,maintenance")
	_, err = db.Exec(`
		INSERT INTO as_receipts (
			as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date,
			start_datetime, complete_datetime, status, assigned_to, data_origin,
			process_type, visit_date
		) VALUES
		('v1','R-V1','c1','2026-08-01','2026-08-03',
		 '2026-08-10 18:00:00','2026-08-10 18:00:00','completed','양기헌','app',
		 'visit','2026-08-03'),
		('v2','R-V2','c1','2026-08-01','2026-08-04',
		 '2026-08-10 18:00:00','2026-08-10 18:00:00','completed','양기헌','app',
		 'visit',''),
		('r1','R-R1','c1','2026-08-01','2026-08-03',
		 '2026-08-10 18:00:00','2026-08-10 18:00:00','completed','양기헌','app',
		 'remote','')
	`)
	if err != nil {
		t.Fatal(err)
	}

	repo := NewStatsRepo(db)
	f := ParseMeetingFilter(model.StatsScopeTeam, "", "")
	visit, nV, err := repo.avgASDays("2026-08-01", "2026-09-01", f, "visit")
	if err != nil {
		t.Fatal(err)
	}
	_, nC, err := repo.avgASDays("2026-08-01", "2026-09-01", f, "complete")
	if err != nil {
		t.Fatal(err)
	}
	if nV != 1 {
		t.Fatalf("visit n=%d want 1 (visit_date 없는 옛 데이터·원격 제외)", nV)
	}
	if nC != 3 {
		t.Fatalf("complete n=%d want 3 (착수·완료 있는 완료 건)", nC)
	}
	if visit < 1.9 || visit > 2.1 {
		t.Fatalf("visit avg=%.2f want 2", visit)
	}

	anchor := time.Date(2026, 8, 15, 0, 0, 0, 0, time.Local)
	cols := BuildStatsPeriodColumns(model.StatsViewMonth, anchor)
	if err := repo.FillPeriodOverview(cols, f); err != nil {
		t.Fatal(err)
	}
	kpi, err := repo.LoadStatsKPI(model.StatsViewMonth, cols, f)
	if err != nil {
		t.Fatal(err)
	}
	if kpi.VisitDisplay.GradeLabel != "판단 보류" {
		t.Fatalf("표본 1건 판단 보류: %+v", kpi.VisitDisplay)
	}
}

func TestVisitLeadHoldJudgmentThreeSamples(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "hold.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('c1','도서관','도서관',1)`); err != nil {
		t.Fatal(err)
	}
	setMetricsPolicy(t, db, "2026-08-01", "as,maintenance")
	_, err = db.Exec(`
		INSERT INTO as_receipts (
			as_id, as_number, customer_id, receipt_datetime, start_datetime, complete_datetime,
			status, assigned_to, data_origin, process_type, visit_date
		) VALUES
		('a','R-A','c1','2026-08-01','2026-08-10','2026-08-10','completed','양기헌','app','visit','2026-08-02'),
		('b','R-B','c1','2026-08-01','2026-08-10','2026-08-10','completed','양기헌','app','visit','2026-08-03'),
		('c','R-C','c1','2026-08-01','2026-08-10','2026-08-10','completed','양기헌','app','visit','2026-08-04')
	`)
	if err != nil {
		t.Fatal(err)
	}
	repo := NewStatsRepo(db)
	f := ParseMeetingFilter(model.StatsScopeTeam, "", "")
	anchor := time.Date(2026, 8, 15, 0, 0, 0, 0, time.Local)
	cols := BuildStatsPeriodColumns(model.StatsViewMonth, anchor)
	if err := repo.FillPeriodOverview(cols, f); err != nil {
		t.Fatal(err)
	}
	kpi, err := repo.LoadStatsKPI(model.StatsViewMonth, cols, f)
	if err != nil {
		t.Fatal(err)
	}
	if kpi.VisitSample != 3 || kpi.VisitDisplay.ShowValue || kpi.VisitDisplay.GradeLabel != "판단 보류" {
		t.Fatalf("표본 3건 판단 보류: sample=%d show=%v label=%s",
			kpi.VisitSample, kpi.VisitDisplay.ShowValue, kpi.VisitDisplay.GradeLabel)
	}
}

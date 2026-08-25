package repository

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestSalesProjectNameOnlyAndStageRules(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "sales.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewSalesRepo(db)

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM codes WHERE code_group='sales_stage'`).Scan(&n); err != nil || n < 8 {
		t.Fatalf("sales_stage codes n=%d err=%v", n, err)
	}

	p := &model.SalesProject{Name: "세종 RFID 증설(가칭)", IsTentativeName: true}
	if err := repo.Create(p); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(p.SalesID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Stage != model.SalesStageLead || got.Probability != 10 {
		t.Fatalf("기본 단계/확도: stage=%s prob=%d", got.Stage, got.Probability)
	}
	if got.CustomerID != "" || got.ExpectedYM != "" || got.ExpectedAmount != 0 {
		t.Fatalf("빈 칸이 채워졌다: %+v", got)
	}

	if err := repo.ChangeStage(p.SalesID, model.SalesStageSubmit, "", "u1", "최혜영", false); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.Get(p.SalesID)
	if got.Probability != 50 || got.HasOverride {
		t.Fatalf("단계 변경 후 확도: prob=%d override=%v", got.Probability, got.HasOverride)
	}

	got.HasOverride, got.OverrideValue = true, 35
	if err := repo.Update(got); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.Get(p.SalesID)
	if !got.HasOverride || got.EffectiveProbability() != 35 {
		t.Fatalf("수동 조정 미반영: %+v", got)
	}

	if err := repo.ChangeStage(p.SalesID, model.SalesStageLead, "", "u1", "최혜영", false); err == nil {
		t.Fatal("후퇴에 사유 없이 통과했다")
	}
	if err := repo.ChangeStage(p.SalesID, model.SalesStageLead, "내년으로 이연", "u1", "최혜영", false); err != nil {
		t.Fatal(err)
	}
	hist, err := repo.ListHistory(p.SalesID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, h := range hist {
		if h.ToStage == model.SalesStageLead && strings.Contains(h.Reason, "내년으로 이연") {
			found = true
		}
	}
	if !found {
		t.Fatalf("후퇴 이력이 없다: %+v", hist)
	}

	if err := repo.ChangeStage(p.SalesID, model.SalesStageWon, "", "u1", "최혜영", false); err == nil {
		t.Fatal("미확정인데 수주확정이 통과했다")
	}
	got, _ = repo.Get(p.SalesID)
	got.IsTentativeName = false
	got.CustomerConfirmed = true
	got.ExpectedYMConfirmed = true
	got.ExpectedAmountConfirmed = true
	if err := repo.Update(got); err != nil {
		t.Fatal(err)
	}
	if err := repo.ChangeStage(p.SalesID, model.SalesStageWon, "", "u1", "최혜영", false); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.Get(p.SalesID)
	if got.Stage != model.SalesStageWon || got.Probability != 90 {
		t.Fatalf("수주확정: stage=%s prob=%d", got.Stage, got.Probability)
	}
}

func TestSalesProjectDoesNotAffectStatsKPI(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "sales_stats.db"))
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
	if err := NewSalesRepo(db).Create(&model.SalesProject{
		Name: "영업 건은 지표에 안 탄다", IsTentativeName: true, ExpectedAmount: 180000000,
	}); err != nil {
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
	const want = "exec=0.000000 visit=2.000000 complete=4.000000 receipt=1 completed=1 events=2"
	if got != want {
		t.Fatalf("영업 건이 통계에 섞였다\n got %s\nwant %s", got, want)
	}
}

func TestSalesStageProbabilityReadsFromCodes(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "sales_codes.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`UPDATE codes SET code_name='18' WHERE code_group='sales_stage_prob' AND code_value='proposal'`); err != nil {
		t.Fatal(err)
	}
	stages, err := NewSalesRepo(db).Stages()
	if err != nil {
		t.Fatal(err)
	}
	d := model.FindSalesStage(stages, model.SalesStageProposal)
	if d == nil || d.Probability != 18 {
		t.Fatalf("codes 확도 미반영: %+v", d)
	}
}

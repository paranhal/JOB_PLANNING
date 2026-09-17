package repository

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

// 대시보드·통계(LoadStatsKPI)와 주간보고서(weeklyCell)가 같은 주간에 같은 산식을 쓰는지 확인한다.
func TestDashboardStatsWeeklySameFormula(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "formula.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('c1','도서관','도서관',1)`); err != nil {
		t.Fatal(err)
	}
	// 금주(2026-08-03~08-09): app 2건 + import 1건.
	// app1 예정일 당일 완료, app2 다음날 완료, import는 당일 완료.
	_, err = db.Exec(`
		INSERT INTO as_receipts (
			as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date,
			start_datetime, complete_datetime, status, assigned_to, data_origin
		) VALUES
		('app1','R-APP1','c1','2026-08-04 09:00:00','2026-08-05',
		 '2026-08-05 10:00:00','2026-08-05 16:00:00','completed','양기헌','app'),
		('app2','R-APP2','c1','2026-08-04 09:00:00','2026-08-06',
		 '2026-08-06 10:00:00','2026-08-07 16:00:00','completed','양기헌','app'),
		('imp1','R-IMP1','c1','2026-08-04 09:00:00','2026-08-05',
		 '2026-08-05 10:00:00','2026-08-05 16:00:00','completed','양기헌','import')
	`)
	if err != nil {
		t.Fatal(err)
	}

	repo := NewStatsRepo(db)
	anchor := time.Date(2026, 8, 7, 0, 0, 0, 0, time.Local)
	f := ParseMeetingFilter(model.StatsScopeTeam, "", "")
	if f.IncludeImport {
		t.Fatal("default must exclude import")
	}

	cols := BuildStatsPeriodColumns(model.StatsViewWeek, anchor)
	if err := repo.FillPeriodOverview(cols, f); err != nil {
		t.Fatal(err)
	}
	kpi, err := repo.LoadStatsKPI(model.StatsViewWeek, cols, f)
	if err != nil {
		t.Fatal(err)
	}

	rep, err := repo.BuildWeeklyReport(anchor)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.PersonRows) == 0 {
		t.Fatal("주간보고서 팀 행 없음")
	}
	team := rep.PersonRows[0]
	if !team.IsTeam {
		t.Fatal("첫 행은 사업팀 전체여야 함")
	}

	cur := cols[1]
	if cur.From != "2026-08-03" || cur.ToExclusive != "2026-08-10" {
		t.Fatalf("금주 열 %s~%s", cur.From, cur.ToExclusive)
	}
	if cur.Counts.AS.Planned != 2 {
		t.Fatalf("이관 제외 예정=%d want 2 (app만)", cur.Counts.AS.Planned)
	}

	if kpi.VisitAvgDays != team.VisitDisplay.Value || kpi.VisitSample != team.VisitDisplay.Sample {
		t.Fatalf("방문 KPI=%.1f/%d 주간=%.1f/%d", kpi.VisitAvgDays, kpi.VisitSample, team.VisitDisplay.Value, team.VisitDisplay.Sample)
	}
	if kpi.CompleteAvgDays != team.CompleteDisplay.Value || kpi.CompleteSample != team.CompleteDisplay.Sample {
		t.Fatalf("완료 KPI=%.1f/%d 주간=%.1f/%d", kpi.CompleteAvgDays, kpi.CompleteSample, team.CompleteDisplay.Value, team.CompleteDisplay.Sample)
	}

	// 토글 켜면 import 1건이 합산된다.
	fIn := f
	fIn.IncludeImport = true
	colsIn := BuildStatsPeriodColumns(model.StatsViewWeek, anchor)
	if err := repo.FillPeriodOverview(colsIn, fIn); err != nil {
		t.Fatal(err)
	}
	if colsIn[1].Counts.AS.Planned != 3 {
		t.Fatalf("이관 포함 예정=%d want 3", colsIn[1].Counts.AS.Planned)
	}
	kpiIn, err := repo.LoadStatsKPI(model.StatsViewWeek, colsIn, fIn)
	if err != nil {
		t.Fatal(err)
	}
	if kpiIn.CompleteSample != 3 {
		t.Fatalf("이관 포함 완료 표본=%d want 3", kpiIn.CompleteSample)
	}
}

func TestIncludeImportToggleDoesNotAffectWeeklyByDefault(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "import.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('c1','도서관','도서관',1)`); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		INSERT INTO as_receipts (
			as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date,
			complete_datetime, status, assigned_to, data_origin
		) VALUES
		('imp1','R-IMP','c1','2026-08-04','2026-08-05','2026-08-05','completed','양기헌','import')`)
	if err != nil {
		t.Fatal(err)
	}

	repo := NewStatsRepo(db)
	anchor := time.Date(2026, 8, 7, 0, 0, 0, 0, time.Local)
	rep, err := repo.BuildWeeklyReport(anchor)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.PersonRows) == 0 || !rep.PersonRows[0].IsTeam {
		t.Fatal("팀 행 없음")
	}
	team := rep.PersonRows[0]
	if team.Receipt != 0 {
		t.Fatalf("이관 접수 포함됨: %d", team.Receipt)
	}
	for _, ev := range rep.Events {
		if ev.WorkNo == "R-IMP" {
			t.Fatal("이관 건이 전체 리스트에 포함됨")
		}
	}
}

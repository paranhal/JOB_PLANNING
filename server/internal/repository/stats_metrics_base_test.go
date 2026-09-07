package repository

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func setMetricsPolicy(t *testing.T, db *sql.DB, base, scope string) {
	t.Helper()
	s := NewSettingsRepo(db)
	if err := s.Set(SettingMetricsBaseDate, base); err != nil {
		t.Fatal(err)
	}
	if err := s.Set(SettingProgressScope, scope); err != nil {
		t.Fatal(err)
	}
}

func monthKPI(t *testing.T, repo *StatsRepo, f model.StatsMeetingFilter) (model.StatsKPICard, model.StatsBucketCounts) {
	t.Helper()
	anchor := time.Date(2026, 8, 7, 0, 0, 0, 0, time.Local)
	cols := BuildStatsPeriodColumns(model.StatsViewMonth, anchor)
	if err := repo.FillPeriodOverview(cols, f); err != nil {
		t.Fatal(err)
	}
	kpi, err := repo.LoadStatsKPI(model.StatsViewMonth, cols, f)
	if err != nil {
		t.Fatal(err)
	}
	return kpi, cols[1].Counts
}

func TestMetricsSettingsSeededNotHardcoded(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "seed.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	p := NewStatsRepo(db).MetricsPolicy()
	if p.BaseDate != "2026-08-03" {
		t.Fatalf("기준일=%q want 2026-08-03 (app_settings)", p.BaseDate)
	}
	if p.ProgressScope != "as,maintenance" {
		t.Fatalf("progress_scope=%q want as,maintenance", p.ProgressScope)
	}
}

func TestMetricsBaseDateAndImportAreSeparateAxes(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "axes.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('c1','도서관','도서관',1)`); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		INSERT INTO as_receipts (
			as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date,
			start_datetime, complete_datetime, status, assigned_to, data_origin
		) VALUES
		('old','R-OLD','c1','2026-07-20 09:00:00','2026-07-21',
		 '2026-07-21 10:00:00','2026-07-21 16:00:00','completed','양기헌','app'),
		('aug1','R-AUG1','c1','2026-08-01 09:00:00','2026-08-02',
		 '2026-08-02 10:00:00','2026-08-02 16:00:00','completed','양기헌','app'),
		('app1','R-APP1','c1','2026-08-04 09:00:00','2026-08-05',
		 '2026-08-05 10:00:00','2026-08-05 16:00:00','completed','양기헌','app'),
		('app2','R-APP2','c1','2026-08-04 09:00:00','2026-08-06',
		 NULL,NULL,'in_progress','양기헌','app'),
		('imp','R-IMP','c1','2026-08-04 09:00:00','2026-08-05',
		 '2026-08-05 10:00:00','2026-08-05 16:00:00','completed','양기헌','import')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		INSERT INTO work_tasks (task_id, work_type, title, due_date, work_date, receipt_date, status, source_type, assignee)
		VALUES
		('t-admin','admin','행정','2026-08-05','2026-08-05','2026-08-05','waiting','','양기헌'),
		('t-sup','support','지원','2026-08-05','2026-08-05','2026-08-05','waiting','','양기헌')`)
	if err != nil {
		t.Fatal(err)
	}

	repo := NewStatsRepo(db)
	f := ParseMeetingFilter(model.StatsScopeTeam, "", "")

	// --- 적용 전: 하한 없음 · 실행률 전 유형 ---
	setMetricsPolicy(t, db, "", "")
	beforeKPI, before := monthKPI(t, repo, f)

	// --- 적용 후: 기준일 + 이관 제외 + 실행률 AS·정기점검 ---
	setMetricsPolicy(t, db, "2026-08-03", "as,maintenance")
	afterKPI, after := monthKPI(t, repo, f)

	fIn := f
	fIn.IncludeImport = true
	importKPI, withImport := monthKPI(t, repo, fIn)

	t.Logf("적용 전후 KPI 비교 (8월 열, 픽스처)")
	t.Logf("| 항목 | 적용 전 | 적용 후 | 이관 포함 |")
	t.Logf("| AS 접수 | %d | %d | %d |", before.AS.Receipt, after.AS.Receipt, withImport.AS.Receipt)
	t.Logf("| AS 예정 | %d | %d | %d |", before.AS.Planned, after.AS.Planned, withImport.AS.Planned)
	t.Logf("| 행정 예정 | %d | %d | %d |", before.Admin.Planned, after.Admin.Planned, withImport.Admin.Planned)
	t.Logf("| 실행률 분모 | %d | %d | %d |", before.ExecPlanned(), after.ExecPlanned(), withImport.ExecPlanned())
	t.Logf("| 방문 표본 | %d | %d | %d |", beforeKPI.VisitSample, afterKPI.VisitSample, importKPI.VisitSample)

	if after.AS.Receipt != 2 {
		t.Fatalf("기준일+이관제외 AS 접수=%d want 2", after.AS.Receipt)
	}
	if before.AS.Receipt != 3 {
		t.Fatalf("하한 없을 때 8월 AS 접수=%d want 3 (08-01 포함)", before.AS.Receipt)
	}
	if withImport.AS.Receipt != 3 {
		t.Fatalf("이관 포함 AS 접수=%d want 3 (기준일은 유지, 08-01 이관축만 해제)", withImport.AS.Receipt)
	}
	if afterKPI.ExecutionSample != after.ExecPlanned() {
		t.Fatalf("KPI 표본=%d ExecPlanned=%d", afterKPI.ExecutionSample, after.ExecPlanned())
	}
	if after.ExecPlanned() != 2 {
		t.Fatalf("실행률 분모=%d want 2 (AS 예정만, 행정 제외)", after.ExecPlanned())
	}
	if before.ExecPlanned() != before.AS.Planned+before.Admin.Planned {
		t.Fatalf("적용 전 실행률 분모=%d want AS+행정 %d", before.ExecPlanned(), before.AS.Planned+before.Admin.Planned)
	}
	if after.Admin.Planned != 2 {
		t.Fatalf("행정·지원 예정=%d want 2 (건수에는 남는다)", after.Admin.Planned)
	}
	if after.ExecPlanned() == after.PlannedTotal() {
		t.Fatal("실행률 분모에 행정이 들어가면 안 됨")
	}
	if withImport.ExecPlanned() != 3 {
		t.Fatalf("이관 포함 실행률 분모=%d want 3", withImport.ExecPlanned())
	}
	if repo.MetricsPolicy().BaseDate != "2026-08-03" {
		t.Fatal("이관 토글이 기준일을 풀면 안 됨")
	}

	// 기준일을 바꾸면 모든 지표가 같이 움직인다.
	setMetricsPolicy(t, db, "2026-09-01", "as,maintenance")
	shiftedKPI, shifted := monthKPI(t, repo, f)
	if shifted.AS.Receipt != 0 || shifted.AS.Planned != 0 || shiftedKPI.VisitSample != 0 {
		t.Fatalf("기준일을 미래로 밀었는데 숫자가 남음 receipt=%d planned=%d visit=%d",
			shifted.AS.Receipt, shifted.AS.Planned, shiftedKPI.VisitSample)
	}
}

func TestASListIgnoresMetricsBaseDate(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "list.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('c1','도서관','도서관',1)`); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, symptom, status, assigned_to, data_origin)
		VALUES
		('old','R-OLD','c1','2026-07-01 09:00:00','옛 증상','received','양기헌','app'),
		('new','R-NEW','c1','2026-08-10 09:00:00','새 증상','received','양기헌','app')`)
	if err != nil {
		t.Fatal(err)
	}

	items, total, err := NewASRepo(db).List("", "", 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("AS 목록 total=%d want 2 (기준일 이전 건도 보여야 함)", total)
	}
	seenOld := false
	for _, it := range items {
		if it.ASID == "old" || it.ASNumber == "R-OLD" {
			seenOld = true
		}
	}
	if !seenOld {
		t.Fatal("AS 목록에 기준일 이전 건이 없다")
	}

	details, err := NewStatsRepo(db).ListDetail(model.StatsQuery{
		Metric: model.StatsMetricReceived,
		Period: model.StatsPeriodRange,
		From:   "2026-01-01",
		To:     "2026-12-31",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range details {
		if d.ASID == "old" || d.ASNumber == "R-OLD" {
			t.Fatal("통계 상세에 기준일 이전 건이 들어갔다")
		}
	}
	if len(details) != 1 {
		t.Fatalf("통계 상세=%d want 1 (기준일 이후 app만)", len(details))
	}
}

func TestAppendixCUsesMetricsBaseDate(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('c1','도서관','도서관',1)`); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, status, visit_scheduled_date, schedule_no_date_reason, data_origin)
		VALUES
		('old','R-OLD','c1','2026-07-01','received','','','app'),
		('new','R-NEW','c1','2026-08-10','received','','','app')`)
	if err != nil {
		t.Fatal(err)
	}
	checks := RunAppendixC(db, true)
	var v2 *AppendixCCheck
	for i := range checks {
		if checks[i].ID == "V-2" {
			v2 = &checks[i]
		}
	}
	if v2 == nil {
		t.Fatal("V-2 없음")
	}
	if v2.Count != 1 {
		t.Fatalf("V-2=%d want 1 (기준일 이전 미계획은 지표 점검에서 뺀다)", v2.Count)
	}
}

package repository

import (
	"database/sql"
	"path/filepath"
	"strings"
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
	_ = scope
}

func TestBuildStatsChartBuckets(t *testing.T) {
	anchor := time.Date(2026, 8, 7, 0, 0, 0, 0, time.Local)

	_, _, dayB := BuildStatsChartBuckets(model.StatsViewDay, anchor)
	if len(dayB) != 14 {
		t.Fatalf("day buckets=%d want 14", len(dayB))
	}
	if dayB[0].Label != "07/25" || dayB[13].Label != "08/07" {
		t.Fatalf("day labels %s .. %s", dayB[0].Label, dayB[13].Label)
	}

	from, toEx, weekB := BuildStatsChartBuckets(model.StatsViewWeek, anchor)
	// 이번주 월=8/3 → 2주전 7/20, 전주 7/27, 이번주 8/3~8/10
	if from != "2026-07-20" || toEx != "2026-08-10" || len(weekB) != 3 {
		t.Fatalf("week %s~%s n=%d", from, toEx, len(weekB))
	}
	if weekB[0].Label != "전전주 07/20" || weekB[1].Label != "전주 07/27" || weekB[2].Label != "이번주 08/03" {
		t.Fatalf("week labels %+v", weekB)
	}

	from, toEx, monthB := BuildStatsChartBuckets(model.StatsViewMonth, anchor)
	if from != "2026-06-01" || toEx != "2026-09-01" || len(monthB) != 3 {
		t.Fatalf("month %s~%s n=%d", from, toEx, len(monthB))
	}
	if monthB[0].Label != "전전월" || monthB[1].Label != "전월" || monthB[2].Label != "이번달" {
		t.Fatalf("month labels %+v", monthB)
	}
}

func TestLoadStatsKPIAndSeries(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "t.db"))
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
		('a1','R1','c1','2026-08-01','2026-08-03','2026-08-03','completed','양기헌','2026-08-05','visit','2026-08-03'),
		('a2','R2','c1','2026-08-02','2026-08-06',NULL,'in_progress','이해진',NULL,NULL,NULL)`)
	if err != nil {
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
	if !kpi.HasVisit || kpi.VisitAvgDays <= 0 {
		t.Fatalf("visit kpi: %+v", kpi)
	}
	if !kpi.HasComplete || kpi.CompleteAvgDays <= 0 {
		t.Fatalf("complete kpi: %+v", kpi)
	}

	series, err := repo.LoadStatsChartSeries(model.StatsViewWeek, anchor, f)
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 3 {
		t.Fatalf("week series=%d want 3", len(series))
	}
	if !strings.HasPrefix(series[0].Label, "전전주") || !strings.HasPrefix(series[2].Label, "이번주") {
		t.Fatalf("week series labels %s .. %s", series[0].Label, series[2].Label)
	}
	if !strings.Contains(series[0].Label, "07/20") || !strings.Contains(series[1].Label, "07/27") {
		t.Fatalf("week monday labels %s / %s", series[0].Label, series[1].Label)
	}
	if series[0].RangeLabel == "" || !strings.Contains(series[0].RangeLabel, "~") {
		t.Fatalf("week range_label=%q", series[0].RangeLabel)
	}

	// 담당자 필터가 실제 집계를 바꿔야 함
	teamCols := BuildStatsPeriodColumns(model.StatsViewMonth, anchor)
	if err := repo.FillPeriodOverview(teamCols, ParseMeetingFilter(model.StatsScopeTeam, "", "")); err != nil {
		t.Fatal(err)
	}
	asgCols := BuildStatsPeriodColumns(model.StatsViewMonth, anchor)
	if err := repo.FillPeriodOverview(asgCols, ParseMeetingFilter(model.StatsScopeAssignee, "양기헌", "")); err != nil {
		t.Fatal(err)
	}
	if teamCols[1].Counts.AS.Planned == asgCols[1].Counts.AS.Planned &&
		teamCols[1].Counts.AS.Process == asgCols[1].Counts.AS.Process {
		// 양기헌 1건·이해진 1건이므로 담당자 필터 시 예정/처리 수가 줄어야 함
		if asgCols[1].Counts.AS.Process >= teamCols[1].Counts.AS.Process && teamCols[1].Counts.AS.Process > 0 {
			t.Fatalf("assignee filter not applied: team=%+v asg=%+v", teamCols[1].Counts.AS, asgCols[1].Counts.AS)
		}
	}
	if asgCols[1].Counts.AS.Process != 1 {
		t.Fatalf("양기헌 process want 1 got %d", asgCols[1].Counts.AS.Process)
	}
}

func TestAvgASLeadTimesSamePopulation(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "lead.db"))
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
			start_datetime, complete_datetime, status, assigned_to, data_origin, updated_at,
			process_type, visit_date
		) VALUES
		('ok','R-OK','c1','2026-08-01','2026-08-10',
		 '2026-08-03','2026-08-05','completed','양기헌','app','2026-08-20',
		 'visit','2026-08-03'),
		('sched','R-SCHED','c1','2026-08-01','2026-08-06',
		 NULL,'2026-08-06','completed','양기헌','app','2026-08-06',
		 'visit',''),
		('open','R-OPEN','c1','2026-08-01','2026-08-04',
		 '2026-08-04',NULL,'in_progress','양기헌','app','2026-08-04',
		 'visit','2026-08-04'),
		('late','R-LATE','c1','2026-08-01','2026-08-02',
		 '2026-08-10 18:00:00','2026-08-05 12:00:00','completed','양기헌','app','2026-08-05',
		 '','')`)
	if err != nil {
		t.Fatal(err)
	}

	repo := NewStatsRepo(db)
	f := ParseMeetingFilter(model.StatsScopeTeam, "", "")
	visit, nV, err := repo.avgASDays("2026-08-01", "2026-09-01", f, "visit")
	if err != nil {
		t.Fatal(err)
	}
	comp, nC, err := repo.avgASDays("2026-08-01", "2026-09-01", f, "complete")
	if err != nil {
		t.Fatal(err)
	}
	if nV != 1 {
		t.Fatalf("방문 표본=%d want 1 (visit_date 있는 visit 완료만)", nV)
	}
	if nC != 2 {
		t.Fatalf("완료 표본=%d want 2 (착수·완료 있는 완료 건)", nC)
	}
	if visit < 1.9 || visit > 2.1 {
		t.Fatalf("방문 평균=%.2f want 2 (visit_date 8/3 − 접수 8/1)", visit)
	}
	if comp < 3.9 || comp > 4.1 {
		t.Fatalf("완료 평균=%.2f want 4", comp)
	}

	anchor := time.Date(2026, 8, 7, 0, 0, 0, 0, time.Local)
	cols := BuildStatsPeriodColumns(model.StatsViewMonth, anchor)
	if err := repo.FillPeriodOverview(cols, f); err != nil {
		t.Fatal(err)
	}
	kpi, err := repo.LoadStatsKPI(model.StatsViewMonth, cols, f)
	if err != nil {
		t.Fatal(err)
	}
	if kpi.VisitSample != 1 || kpi.CompleteSample != 2 {
		t.Fatalf("KPI n visit=%d complete=%d (산식이 달라 n이 갈라지는 것이 정직)", kpi.VisitSample, kpi.CompleteSample)
	}
	if kpi.LeadTimeWarn != "" {
		t.Fatalf("n이 다른데 방문>완료 경고가 나왔다: %q", kpi.LeadTimeWarn)
	}
	if kpi.VisitDisplay.GradeLabel != "판단 보류" || kpi.VisitDisplay.GradeMark != "⚪" {
		t.Fatalf("방문 표본 1건은 판단 보류여야 한다: %+v", kpi.VisitDisplay)
	}
	if kpi.CompleteDisplay.ShowValue {
		t.Fatal("완료 표본 2건은 측정불가여야 한다")
	}

	checks := RunAppendixC(db, false)
	var v12 *AppendixCCheck
	for i := range checks {
		if checks[i].ID == "V-12" {
			v12 = &checks[i]
		}
	}
	if v12 == nil || v12.Count != 1 || v12.OK {
		t.Fatalf("V-12=%+v", v12)
	}
}

func TestCompleteDateIgnoresUpdatedAt(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "cdate.db"))
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
			start_datetime, complete_datetime, status, assigned_to, data_origin, updated_at
		) VALUES
		('miss','R-MISS','c1','2026-08-01','2026-08-03',
		 '2026-08-03',NULL,'completed','양기헌','app','2026-08-20'),
		('ok','R-OK','c1','2026-08-01','2026-08-03',
		 '2026-08-03','2026-08-05','completed','양기헌','app','2026-08-20')`)
	if err != nil {
		t.Fatal(err)
	}

	repo := NewStatsRepo(db)
	f := ParseMeetingFilter(model.StatsScopeTeam, "", "")
	anchor := time.Date(2026, 8, 7, 0, 0, 0, 0, time.Local)
	cols := BuildStatsPeriodColumns(model.StatsViewMonth, anchor)
	if err := repo.FillPeriodOverview(cols, f); err != nil {
		t.Fatal(err)
	}
	if cols[1].Counts.AS.Process != 1 {
		t.Fatalf("처리=%d want 1 (updated_at 으로 빈 완료일을 채우면 안 됨)", cols[1].Counts.AS.Process)
	}
	_, n, err := repo.avgASDays("2026-08-01", "2026-09-01", f, "complete")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("리드타임 표본=%d want 1 (처리 건수와 같은 완료일 축)", n)
	}
	miss, err := NewWBRepo(db).CountMissingCompleteDates()
	if err != nil {
		t.Fatal(err)
	}
	if miss.AS != 1 {
		t.Fatalf("완료일 미기록=%d want 1", miss.AS)
	}
}

func TestPartialCompleteWorkInChartReceivedOpen(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "partial.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('c1','도서관','도서관',1)`); err != nil {
		t.Fatal(err)
	}
	setMetricsPolicy(t, db, "2026-08-01", "as,maintenance")
	_, err = db.Exec(`
		INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date, status, assigned_to, complete_datetime)
		VALUES ('a1','R1','c1','2026-08-01','2026-08-10','partial_complete','양기헌','2026-08-05')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		INSERT INTO as_work_items (work_id, work_number, as_id, work_kind, scheduled_date, schedule_confirmed, status, notes, created_at, updated_at)
		VALUES ('w1','R1-W01','a1','revisit','2026-08-10',1,'open','추가방문','2026-08-05 12:00:00','2026-08-05 12:00:00')`)
	if err != nil {
		t.Fatal(err)
	}

	repo := NewStatsRepo(db)
	anchor := time.Date(2026, 8, 7, 0, 0, 0, 0, time.Local)
	f := ParseMeetingFilter(model.StatsScopeTeam, "", "")
	series, err := repo.LoadStatsChartSeries(model.StatsViewWeek, anchor, f)
	if err != nil {
		t.Fatal(err)
	}
	// 이번주(8/3~): 원 접수(8/1)는 전주, 하위업무 생성(8/5)+부분완료(8/5)는 이번주
	var thisWeek *model.StatsChartPoint
	for i := range series {
		if strings.HasPrefix(series[i].Label, "이번주") {
			thisWeek = &series[i]
			break
		}
	}
	if thisWeek == nil {
		t.Fatal("이번주 bucket missing")
	}
	if thisWeek.Completed < 1 {
		t.Fatalf("partial parent should count as completed, got completed=%d", thisWeek.Completed)
	}
	if thisWeek.Received < 1 {
		t.Fatalf("work item should count as received, got received=%d", thisWeek.Received)
	}
	if thisWeek.Open < 1 {
		t.Fatalf("open work item should count as open, got open=%d", thisWeek.Open)
	}
}

func TestAdminWorkCountsInReceiptNotProcess(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "exec-admin.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('c1','도서관','도서관',1)`); err != nil {
		t.Fatal(err)
	}
	setMetricsPolicy(t, db, "2026-08-01", "")
	_, err = db.Exec(`
		INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date, start_datetime, complete_datetime, status, assigned_to, data_origin)
		VALUES ('a1','R1','c1','2026-08-04','2026-08-05','2026-08-05','2026-08-05','completed','양기헌','app')`)
	if err != nil {
		t.Fatal(err)
	}
	repo := NewStatsRepo(db)
	f := ParseMeetingFilter(model.StatsScopeTeam, "", "")
	anchor := time.Date(2026, 8, 7, 0, 0, 0, 0, time.Local)
	cols := BuildStatsPeriodColumns(model.StatsViewWeek, anchor)
	if err := repo.FillPeriodOverview(cols, f); err != nil {
		t.Fatal(err)
	}
	receiptBefore := cols[1].Counts.ReceiptTotal()
	processBefore := cols[1].Counts.ProcessTotal()
	_, err = db.Exec(`
		INSERT INTO work_tasks (task_id, work_type, title, due_date, work_date, receipt_date, status, source_type, assignee)
		VALUES ('t-admin','admin','행정','2026-08-05','2026-08-05','2026-08-05','waiting','','양기헌')`)
	if err != nil {
		t.Fatal(err)
	}
	cols2 := BuildStatsPeriodColumns(model.StatsViewWeek, anchor)
	if err := repo.FillPeriodOverview(cols2, f); err != nil {
		t.Fatal(err)
	}
	if cols2[1].Counts.ReceiptTotal() != receiptBefore+1 {
		t.Fatalf("접수는 행정 포함되어야 함: %d → %d", receiptBefore, cols2[1].Counts.ReceiptTotal())
	}
	if cols2[1].Counts.ProcessTotal() != processBefore {
		t.Fatalf("처리 건수는 미완료 행정으로 늘면 안 됨: %d → %d", processBefore, cols2[1].Counts.ProcessTotal())
	}
	kpi, err := repo.LoadStatsKPI(model.StatsViewWeek, cols2, f)
	if err != nil {
		t.Fatal(err)
	}
	series, err := repo.LoadStatsChartSeries(model.StatsViewWeek, anchor, f)
	if err != nil {
		t.Fatal(err)
	}
	var cur *model.StatsChartPoint
	for i := range series {
		if strings.HasPrefix(series[i].Label, "이번주") {
			cur = &series[i]
			break
		}
	}
	if cur == nil {
		t.Fatal("이번주 차트 없음")
	}
	if cur.Received != cols2[1].Counts.ReceiptTotal() {
		t.Fatalf("차트 접수 %d 열 %d", cur.Received, cols2[1].Counts.ReceiptTotal())
	}
	_ = kpi
}

package repository

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestListUnplannedFiveKindsAndMaintenanceSlot(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "unplanned.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now()
	today := now.Format("2006-01-02")
	past := now.AddDate(0, 0, -5).Format("2006-01-02")
	ts := now.Format("2006-01-02 15:04:05")
	expiredAt := now.AddDate(0, 0, -15).Format("2006-01-02")

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('C1','기관A','기관A',1)`); err != nil {
		t.Fatal(err)
	}

	insertAS := func(id, num, visit, confirmed, assigned, status, reason, reasonAt string) {
		t.Helper()
		_, err := db.Exec(`INSERT INTO as_receipts (
			as_id, as_number, receipt_datetime, customer_id, symptom, urgency, status,
			visit_scheduled_date, schedule_confirmed, assigned_to,
			schedule_no_date_reason, schedule_no_date_at, created_at, updated_at
		) VALUES (?,?,?,'C1','증상','중',?,?,?,?,?,?,?,?)`,
			id, num, ts, status, visit, confirmed, assigned, reason, reasonAt, ts, ts)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
	}

	insertAS("AS-NODATE", "R-ND", "", "0", "테크", "in_progress", "", "")
	insertAS("AS-DELAY", "R-DL", past, "1", "테크", "in_progress", "", "")
	insertAS("AS-NEXT", "R-NX", past, "1", "테크", "in_progress", "", "")
	insertAS("AS-UNAS", "R-UA", today, "1", "", "received", "", "")
	insertAS("AS-REV", "R-RV", "", "0", "테크", "in_progress", "부품 대기", expiredAt)
	insertAS("AS-STARTED", "R-ST", past, "1", "테크", "in_progress", "", "")
	if _, err := db.Exec(`UPDATE as_receipts SET start_datetime=? WHERE as_id='AS-STARTED'`, past+" 10:00:00"); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(`INSERT INTO as_processes (process_id, process_number, as_id, process_datetime, worker, work_content, time_spent)
		VALUES ('P1','P1','AS-NEXT',?,'테크','방문',30)`, past+" 14:00:00"); err != nil {
		t.Fatal(err)
	}

	mnt := NewMaintenanceRepo(db)
	if err := mnt.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "C1", ShortName: "기관A", HasKlas: true, HasRfid: false, InspectionCycle: "monthly",
	}); err != nil {
		t.Fatal(err)
	}
	plan, err := mnt.CreatePlan(now.Year(), "t")
	if err != nil {
		t.Fatal(err)
	}

	wb := NewWorkBoardRepo(db)
	items, counts, err := wb.ListUnplanned("", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if counts.Total < 6 {
		t.Fatalf("고유 건수=%d want >=6 (AS 5 + 점검슬롯) items=%d", counts.Total, len(items))
	}

	has := map[string]bool{}
	for _, it := range items {
		for _, k := range it.Kinds {
			has[it.RefNumber+"|"+k] = true
		}
	}
	if !has["R-ND|"+model.UnplannedNoDate] {
		t.Fatal("예정없음 누락")
	}
	if !has["R-DL|"+model.UnplannedDelayed] {
		t.Fatal("예정일경과 누락 — collectDelayed 재사용")
	}
	if !has["R-NX|"+model.UnplannedNext] {
		t.Fatal("다음일정미정 누락 — collectSchedulePending 방문함 분기")
	}
	if has["R-NX|"+model.UnplannedDelayed] {
		t.Fatal("방문한 건을 예정일경과로 세면 안 된다")
	}
	if !has["R-UA|"+model.UnplannedUnassigned] {
		t.Fatal("담당자미배정 누락 — collectUnassigned 재사용")
	}
	if !has["R-RV|"+model.UnplannedReview] {
		t.Fatal("미정사유만료 재검토 누락")
	}
	if !has["R-ST|"+model.UnplannedDelayed] {
		t.Fatal("착수한 경과 건이 예정일경과에 있어야 한다")
	}

	if counts.NeedPlan < 2 {
		t.Fatalf("계획을 세워야 할 건=%d", counts.NeedPlan)
	}
	if counts.Overdue < 3 {
		t.Fatalf("일정이 지난 건=%d", counts.Overdue)
	}

	var delayStarted, delayNot bool
	for _, it := range items {
		if it.RefNumber == "R-DL" && it.HasKind(model.UnplannedDelayed) {
			if it.Started {
				t.Fatal("착수 없는 경과는 Started=false 여야 한다")
			}
			if len(it.Badges) == 0 || !strings.HasPrefix(it.Badges[0].Label, "D+") || !strings.Contains(it.Badges[0].Class, "red") {
				t.Fatalf("미착수 뱃지=%+v", it.Badges)
			}
			delayNot = true
		}
		if it.RefNumber == "R-ST" && it.HasKind(model.UnplannedDelayed) {
			if !it.Started {
				t.Fatal("start_datetime 있는 경과는 Started=true")
			}
			found := false
			for _, b := range it.Badges {
				if strings.HasPrefix(b.Label, "진행중 D+") && strings.Contains(b.Class, "amber") {
					found = true
				}
			}
			if !found {
				t.Fatalf("착수 뱃지=%+v", it.Badges)
			}
			delayStarted = true
		}
	}
	if !delayNot || !delayStarted {
		t.Fatalf("경과 착수 구분 누락 not=%v started=%v", delayNot, delayStarted)
	}
	rankNot, rankStarted := -1, -1
	for i, it := range items {
		if it.RefNumber == "R-DL" && rankNot < 0 {
			rankNot = i
		}
		if it.RefNumber == "R-ST" && rankStarted < 0 {
			rankStarted = i
		}
	}
	if rankNot < 0 || rankStarted < 0 || rankNot > rankStarted {
		t.Fatalf("빨강이 노랑보다 앞이어야 한다: 미착수=%d 착수=%d", rankNot, rankStarted)
	}

	slots := 0
	for _, it := range items {
		if it.HasKind(model.UnplannedNoDate) && it.Prefix == model.WorkPrefixMaintenance {
			slots++
		}
	}
	if slots < 1 {
		t.Fatal("정기점검 배정안됨이 미계획에 합산되어야 한다")
	}

	stats, err := wb.DashStats("", nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Unplanned != counts.Total {
		t.Fatalf("대시보드 미계획=%d 목록=%d", stats.Unplanned, counts.Total)
	}

	pc, err := wb.CountPlanning()
	if err != nil {
		t.Fatal(err)
	}
	if !pc.HasPlanningRate() || pc.Open < counts.Total {
		t.Fatalf("계획수립률 분모=%d 미계획=%d", pc.Open, counts.Total)
	}
	// 미정 사유 건(AS-REV)은 분자에 포함. 밀린 건(AS-DELAY)도 계획이 있으므로 분자.
	if pc.Planned < 3 {
		t.Fatalf("예정일·미정사유가 있는 건은 분자: %+v", pc)
	}

	if err := wb.AssignUnplannedDate("as:AS-NODATE", today, "테크", ""); err != nil {
		t.Fatal(err)
	}
	items2, _, err := wb.ListUnplanned("", nil, model.UnplannedNoDate)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items2 {
		if it.RefID == "AS-NODATE" {
			t.Fatal("날짜 배정 후에도 예정없음에 남아 있다")
		}
	}

	if err := NewASRepo(db).SetScheduleNoDate("AS-DELAY", model.FormatNoDateReason(model.NoDateReasonParts, "")); err != nil {
		t.Fatal(err)
	}
	var reason, at string
	if err := db.QueryRow(`SELECT COALESCE(schedule_no_date_reason,''), COALESCE(schedule_no_date_at,'') FROM as_receipts WHERE as_id='AS-DELAY'`).
		Scan(&reason, &at); err != nil {
		t.Fatal(err)
	}
	if reason == "" || at != today {
		t.Fatalf("미정 등록 reason=%q at=%q", reason, at)
	}
	_ = plan
}

func TestPlanningRateIncludesNoDateReason(t *testing.T) {
	if model.PlanningRatePct(0, 0) != 0 || model.HasPlanningRate(0) {
		t.Fatal("분모 0")
	}
	if got := model.PlanningRatePct(19, 20); got != 95 {
		t.Fatalf("95%%: %v", got)
	}
	pc := model.PlanningCounts{Open: 10, Planned: 9}
	if pc.Rate() != 90 {
		t.Fatalf("rate=%v", pc.Rate())
	}

	ok := model.StatsReliability(20, false, 91)
	capped := ok.CapIfLowPlanning(true, 80)
	if capped.ShowValue || capped.Grade != model.StatsGradeNA || capped.GradeMark != "⚪" {
		t.Fatalf("계획 수립률 부족 시 실행율은 측정불가: %+v", capped)
	}
	if ok.CapIfLowPlanning(true, 95).Grade != model.StatsGradeTrusted {
		t.Fatal("95% 이상이면 실행율 등급을 유지")
	}
	if ok.CapIfLowPlanning(false, 0).Grade != model.StatsGradeTrusted {
		t.Fatal("미완료 없으면 실행율 그대로")
	}
}

func TestOverdueWaitingActionInUnplannedAndDelayed(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "wa_delay.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	wb := NewWBRepo(db)
	today := time.Now().Format("2006-01-02")
	past := time.Now().AddDate(0, 0, -2).Format("2006-01-02")
	task := &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "IRM 평가지표", Status: model.WBTaskWaitingFor,
		DueDate: today, WaitParty: "□□정보 박OO", WaitRequest: "전자도서관 부문",
	}
	if err := wb.CreateTask(task); err != nil {
		t.Fatal(err)
	}
	if err := wb.CreateAction(&model.WorkAction{
		TaskID: task.TaskID, Title: "전자도서관 부문 요청", Status: model.WBActionWaiting, Required: true,
		WaitParty: "□□정보 박OO", WaitRequest: "지표 회신", ReplyDueDate: past, NextCheckDate: today,
	}); err != nil {
		t.Fatal(err)
	}

	board := NewWorkBoardRepo(db)
	delayed, err := board.ListBucket(model.WorkBucketDelayed, "", nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, it := range delayed {
		if it.Title == "전자도서관 부문 요청" && it.SubLabel == "회신 대기" {
			found = true
			if it.DaysOverdue < 1 {
				t.Fatalf("경과일=%d", it.DaysOverdue)
			}
		}
	}
	if !found {
		t.Fatalf("지연 목록에 회신 경과 행동이 없음 %+v", delayed)
	}

	items, counts, err := board.ListUnplanned("", nil, model.UnplannedDelayed)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Delayed < 1 {
		t.Fatalf("미계획 예정일경과=%d", counts.Delayed)
	}
	found = false
	for _, it := range items {
		if it.Title == "전자도서관 부문 요청" && it.HasKind(model.UnplannedDelayed) {
			found = true
		}
	}
	if !found {
		t.Fatal("미계획 업무함에 회신 경과 행동이 없음")
	}
}

func TestCountPlanningTreatsDelayedAsPlannedNotNeedPlan(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "plan_rate.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ts := time.Now().Format("2006-01-02 15:04:05")
	past := time.Now().AddDate(0, 0, -3).Format("2006-01-02")
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('C1','기관','기관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, receipt_datetime, customer_id, symptom, urgency, status,
		visit_scheduled_date, schedule_confirmed, assigned_to, created_at, updated_at)
		VALUES ('AS1','R1',?,'C1','s','중','in_progress',?,1,'테크',?,?),
		       ('AS2','R2',?,'C1','s','중','in_progress','',0,'테크',?,?)`,
		ts, past, ts, ts, ts, ts, ts); err != nil {
		t.Fatal(err)
	}
	pc, err := NewWorkBoardRepo(db).CountPlanning()
	if err != nil {
		t.Fatal(err)
	}
	if pc.Open < 2 || pc.Planned < 1 {
		t.Fatalf("분모=%d 분자=%d", pc.Open, pc.Planned)
	}
	// 밀린 건을 분자에서 빼면 계획을 세워도 지표가 떨어진다.
	if pc.Planned < 1 {
		t.Fatal("예정일 경과 건은 계획 수립률 분자에 남아야 한다")
	}
}

func TestAssignUnplannedDateRejectsYear0206(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "date_year.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ts := time.Now().Format("2006-01-02 15:04:05")
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('C1','기관','기관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, receipt_datetime, customer_id, symptom, urgency, status,
		assigned_to, created_at, updated_at)
		VALUES ('AS1','R1',?,'C1','s','중','received','테크',?,?)`, ts, ts, ts); err != nil {
		t.Fatal(err)
	}
	err = NewWorkBoardRepo(db).AssignUnplannedDate("as:AS1", "0206-08-12", "테크", "")
	if !errors.Is(err, model.ErrAppDateYear) {
		t.Fatalf("0206 거절: %v", err)
	}
	var visit string
	if err := db.QueryRow(`SELECT COALESCE(visit_scheduled_date,'') FROM as_receipts WHERE as_id='AS1'`).Scan(&visit); err != nil {
		t.Fatal(err)
	}
	if visit != "" {
		t.Fatalf("거절 후에도 저장됨: %q", visit)
	}
}

func TestAppendixCListsYearOutOfRangeDates(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "appendix_c.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ts := time.Now().Format("2006-01-02 15:04:05")
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('C1','기관','기관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, receipt_datetime, customer_id, symptom, urgency, status,
		visit_scheduled_date, schedule_confirmed, assigned_to, created_at, updated_at)
		VALUES ('AS-BAD','R2608-039',?,'C1','s','중','in_progress','0206-08-12',1,'테크',?,?)`, ts, ts, ts); err != nil {
		t.Fatal(err)
	}
	checks := RunAppendixC(db, false)
	var v11 *AppendixCCheck
	for i := range checks {
		if checks[i].ID == "V-11" {
			v11 = &checks[i]
		}
	}
	if v11 == nil || v11.Count < 1 {
		t.Fatalf("V-11 건수=%v", v11)
	}
	found := false
	for _, r := range v11.Rows {
		if r.Number == "R2608-039" && r.Column == "visit_scheduled_date" && r.Value == "0206-08-12" {
			found = true
		}
	}
	if !found {
		t.Fatalf("0206 행이 목록에 없다: %+v", v11.Rows)
	}
	var still string
	_ = db.QueryRow(`SELECT visit_scheduled_date FROM as_receipts WHERE as_id='AS-BAD'`).Scan(&still)
	if still != "0206-08-12" {
		t.Fatalf("자동으로 고치면 안 된다: %q", still)
	}
}

func TestSalesUnplannedFollowupReviewAndAssign(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "sales_unplanned.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewProjectRepo(db)
	open := &model.WorkProject{
		Name: "후속없는건", ProjectKind: model.ProjectKindBuild, SalesStage: model.SalesStageLead,
		ExpectedYM: "2027-03", SalesOwner: "관리자", ProspectName: "후속기관",
		Status: model.WBProjectActive, Color: "#3B82F6",
	}
	if err := repo.Create(open); err != nil {
		t.Fatal(err)
	}
	over := &model.WorkProject{
		Name: "지난예정", ProjectKind: model.ProjectKindSupply, SalesStage: model.SalesStageQuote,
		ExpectedYM: "2020-01", SalesOwner: "관리자", ProspectName: "지난기관",
		Status: model.WBProjectActive, Color: "#3B82F6",
	}
	if err := repo.Create(over); err != nil {
		t.Fatal(err)
	}
	board := NewWorkBoardRepo(db)
	items, counts, err := board.ListUnplanned("", nil, model.UnplannedSalesFollowup)
	if err != nil {
		t.Fatal(err)
	}
	if counts.SalesFollowup < 2 {
		t.Fatalf("후속없음 counts=%+v n=%d", counts, len(items))
	}
	rev, _, err := board.ListUnplanned("", nil, model.UnplannedReview)
	if err != nil {
		t.Fatal(err)
	}
	foundRev := false
	for _, it := range rev {
		if it.RefID == over.ProjectID {
			foundRev = true
		}
	}
	if !foundRev {
		t.Fatal("예정월 경과 건이 재검토에 없다")
	}
	pc, err := board.CountPlanning()
	if err != nil {
		t.Fatal(err)
	}
	if pc.Open < 2 || pc.NeedPlan < 2 {
		t.Fatalf("영업 미계획 분모=%d 미계획=%d", pc.Open, pc.NeedPlan)
	}
	if err := board.AssignUnplannedDate("proj:"+open.ProjectID, "2026-09-01", "관리자", ""); err != nil {
		t.Fatal(err)
	}
	after, counts2, err := board.ListUnplanned("", nil, model.UnplannedSalesFollowup)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range after {
		if it.RefID == open.ProjectID {
			t.Fatal("날짜를 배정했는데 후속없음에 남았다")
		}
	}
	if counts2.SalesFollowup >= counts.SalesFollowup {
		t.Fatalf("후속없음이 줄어야 한다 before=%d after=%d", counts.SalesFollowup, counts2.SalesFollowup)
	}
	task, err := NewWBRepo(db).GetTaskBySource(model.WBSourceProject, open.ProjectID)
	if err != nil || task == nil || task.SourceType != model.WBSourceProject {
		t.Fatalf("source_type=project 없음: %+v err=%v", task, err)
	}
}

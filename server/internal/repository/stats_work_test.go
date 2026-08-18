package repository

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestLoadStatsWorkAnalysisAndSpotlight(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "work.db"))
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
			start_datetime, complete_datetime, status, assigned_to, urgency, priority
		) VALUES
		('a1','R1','c1','2026-08-04 09:00:00','2026-08-04','2026-08-04 10:00:00','2026-08-04 12:30:00','completed','양기헌','high','high'),
		('a2','R2','c1','2026-07-30 09:00:00','2026-08-05',NULL,NULL,'in_progress','이해진','normal','normal'),
		('a3','R3','c1','2026-08-05 09:00:00','2026-08-06','2026-08-06 09:00:00',NULL,'in_progress','양기헌','high','normal'),
		('a4','R4','c1','2026-07-20 09:00:00','2026-07-21','2026-07-21 09:00:00','2026-07-22 09:00:00','completed','양기헌','high','high')
	`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		INSERT INTO as_processes (process_id, as_id, process_datetime, worker, time_spent)
		VALUES
		('p1','a1','2026-08-04 10:00:00','양기헌',90),
		('p2','a1','2026-08-04 11:30:00','양기헌',60),
		('p3','a3','2026-08-06 09:00:00','양기헌',20)`)
	if err != nil {
		t.Fatal(err)
	}

	repo := NewStatsRepo(db)
	f := ParseMeetingFilter(model.StatsScopeTeam, "", "")
	from, toEx := "2026-08-03", "2026-08-10"

	an, err := repo.LoadStatsWorkAnalysis(from, toEx, f)
	if err != nil {
		t.Fatal(err)
	}
	if an.Receipt != 2 { // a1 8/4, a3 8/5
		t.Fatalf("receipt=%d want 2", an.Receipt)
	}
	if an.Visit != 2 { // a1 start 8/4, a3 start 8/6
		t.Fatalf("visit=%d want 2", an.Visit)
	}
	if an.Completed != 1 { // a1
		t.Fatalf("completed=%d want 1", an.Completed)
	}
	if an.CarryIn != 1 { // a2 received 7/30 still open
		t.Fatalf("carryIn=%d want 1 %+v", an.CarryIn, an)
	}
	if an.CarryOut != 2 { // a2 + a3 still open at 8/10
		t.Fatalf("carryOut=%d want 2 %+v", an.CarryOut, an)
	}

	sp, err := repo.LoadStatsSpotlight(from, toEx, f, an.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	if sp.TopN != 1 {
		t.Fatalf("topN=%d want 1 (receipt=2)", sp.TopN)
	}
	if len(sp.Longest) != 1 || sp.Longest[0].ASNumber != "R1" {
		t.Fatalf("longest %+v", sp.Longest)
	}
	if sp.Longest[0].DurationMin != 150 {
		t.Fatalf("duration=%d want 150 (90+60)", sp.Longest[0].DurationMin)
	}
	if len(sp.OverHour) != 1 || sp.OverHour[0].ASNumber != "R1" {
		t.Fatalf("overHour %+v", sp.OverHour)
	}
	if len(sp.MostVisits) < 1 || sp.MostVisits[0].ASNumber != "R1" || sp.MostVisits[0].VisitRounds != 2 {
		t.Fatalf("mostVisits %+v", sp.MostVisits)
	}
	if len(sp.HighPriority) != 1 || sp.HighPriority[0].ASNumber != "R1" {
		t.Fatalf("highPriority %+v (a4 is outside period)", sp.HighPriority)
	}
	if len(sp.Timeline) < 2 {
		t.Fatalf("timeline n=%d", len(sp.Timeline))
	}
}

func TestStatsProjectFilterOnAS(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "proj.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('c1','도서관','도서관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO work_projects (project_id, name, short_name, plan_year, is_paid, sort_order, status)
		VALUES ('WP1','테스트사업','테스트',2026,1,1,'active')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO assets (asset_id, customer_id, product_name, project_id) VALUES ('A1','c1','KLAS','WP1')`); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		INSERT INTO as_receipts (as_id, as_number, customer_id, asset_id, receipt_datetime, status, assigned_to)
		VALUES
		('a1','R1','c1','A1','2026-08-04','in_progress','양기헌'),
		('a2','R2','c1',NULL,'2026-08-04','in_progress','양기헌')`)
	if err != nil {
		t.Fatal(err)
	}

	repo := NewStatsRepo(db)
	from, toEx := "2026-08-03", "2026-08-10"
	team, err := repo.LoadStatsWorkAnalysis(from, toEx, ParseMeetingFilter(model.StatsScopeTeam, "", ""))
	if err != nil {
		t.Fatal(err)
	}
	proj, err := repo.LoadStatsWorkAnalysis(from, toEx, ParseMeetingFilter(model.StatsScopeTeam, "", "WP1"))
	if err != nil {
		t.Fatal(err)
	}
	if team.Receipt != 2 {
		t.Fatalf("team receipt=%d want 2", team.Receipt)
	}
	if proj.Receipt != 1 {
		t.Fatalf("project receipt=%d want 1", proj.Receipt)
	}
}

func TestLoadStatsWorkAnalysisIncludesMntAdmin(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "mix.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('c1','도서관','도서관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, start_datetime, complete_datetime, status, assigned_to)
		VALUES ('a1','R1','c1','2026-08-04','2026-08-04 10:00:00','2026-08-04 12:00:00','completed','양기헌')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_plans (plan_id, plan_year, title, status) VALUES ('P1',2026,'2026','approved')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits (visit_id, plan_id, visit_date, customer_id, completed, completed_date, assignee)
		VALUES ('V1','P1','2026-08-05','c1',1,'2026-08-05','양기헌')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks (task_id, work_type, title, work_date, due_date, duration_min, status, assignee, complete_date, receipt_date)
		VALUES ('T1','admin','행정회의','2026-08-06','2026-08-06',90,'complete','양기헌','2026-08-06','2026-08-06')`); err != nil {
		t.Fatal(err)
	}

	repo := NewStatsRepo(db)
	an, err := repo.LoadStatsWorkAnalysis("2026-08-03", "2026-08-10", ParseMeetingFilter(model.StatsScopeTeam, "", ""))
	if err != nil {
		t.Fatal(err)
	}
	if an.AS.Completed != 1 || an.Mnt.Completed != 1 || an.Admin.Completed != 1 {
		t.Fatalf("type completed AS=%d mnt=%d admin=%d", an.AS.Completed, an.Mnt.Completed, an.Admin.Completed)
	}
	if an.Completed != 3 {
		t.Fatalf("completed total=%d want 3", an.Completed)
	}
	if an.Receipt < 3 {
		t.Fatalf("receipt=%d want >=3 (AS+점검+행정)", an.Receipt)
	}

	done, err := repo.ListCompletedDetail("2026-08-03", "2026-08-10", ParseMeetingFilter(model.StatsScopeTeam, "", ""))
	if err != nil {
		t.Fatal(err)
	}
	if len(done) < 3 {
		t.Fatalf("completed list=%d want >=3", len(done))
	}

	rep, err := repo.BuildWeeklyReport(time.Date(2026, 8, 7, 0, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.PersonRows) < 1 {
		t.Fatalf("weekly rows=%d", len(rep.PersonRows))
	}
	team := rep.PersonRows[0]
	if !team.IsTeam || team.Completed < 3 {
		t.Fatalf("이번주 팀 완료=%d %+v", team.Completed, team)
	}
}

func TestAdminLeadBreakdownOverlapAndUnplannedSheet(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "lead.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	wb := NewWBRepo(db)
	task := &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "IRM 평가지표", Status: model.WBTaskComplete,
		DueDate: "2026-08-14", WorkDate: "2026-08-13",
		ReceiptDate: "2026-08-13", CompleteDate: "2026-08-14", Assignee: "관리자",
	}
	if err := wb.CreateTask(task); err != nil {
		t.Fatal(err)
	}
	vendors := []string{"OO소프트", "△△시스템", "□□정보"}
	for _, v := range vendors {
		a := &model.WorkAction{
			TaskID: task.TaskID, Title: v + " 요청", Status: model.WBActionComplete,
			Required: true, WaitParty: v, WaitRequest: "지표", ReplyDueDate: "2026-08-13",
		}
		if err := wb.CreateAction(a); err != nil {
			t.Fatal(err)
		}
		send := &model.WorkActivity{
			TaskID: task.TaskID, ActionID: a.ActionID, ActivityType: model.WBActivitySend,
			Content: "요청", Actor: "관리자", SpentMinutes: 10,
		}
		if err := wb.CreateActivity(send); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE work_activities SET created_at='2026-08-13 09:00:00' WHERE activity_id=?`, send.ActivityID); err != nil {
			t.Fatal(err)
		}
		reply := &model.WorkActivity{
			TaskID: task.TaskID, ActionID: a.ActionID, ActivityType: model.WBActivityReply,
			Content: "회신", Actor: "업체", SpentMinutes: 5,
		}
		if err := wb.CreateActivity(reply); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE work_activities SET created_at='2026-08-14 09:00:00' WHERE activity_id=?`, reply.ActivityID); err != nil {
			t.Fatal(err)
		}
	}

	repo := NewStatsRepo(db)
	an, err := repo.LoadStatsWorkAnalysis("2026-08-10", "2026-08-16", ParseMeetingFilter(model.StatsScopeTeam, "", ""))
	if err != nil {
		t.Fatal(err)
	}
	if an.AdminLead.Sample != 1 {
		t.Fatalf("sample=%d", an.AdminLead.Sample)
	}
	if an.AdminLead.LeadDaysAvg != 1 {
		t.Fatalf("lead avg=%v want 1", an.AdminLead.LeadDaysAvg)
	}
	if an.AdminLead.WorkMinutes != 45 { // 3*(10+5)
		t.Fatalf("work=%d want 45", an.AdminLead.WorkMinutes)
	}
	if an.AdminLead.WaitDaysSum < 0.9 || an.AdminLead.WaitDaysSum > 1.1 {
		t.Fatalf("wait days=%v want ~1 (3개사 겹침 1일)", an.AdminLead.WaitDaysSum)
	}
	pct, ok := model.ExternalWaitSharePct(an.AdminLead.WaitDaysSum, an.AdminLead.LeadDaysSum)
	if !ok || pct < 90 {
		t.Fatalf("wait share=%v ok=%v", pct, ok)
	}

	open := &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "전자도서관 대기중", Status: model.WBTaskWaitingFor,
		DueDate: "2026-08-20", WaitParty: "□□정보 박OO", WaitRequest: "수정본", ReplyDueDate: "2026-08-15",
	}
	if err := wb.CreateTask(open); err != nil {
		t.Fatal(err)
	}
	rep, err := repo.BuildWeeklyReport(time.Date(2026, 8, 14, 0, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.PersonRows) < 1 {
		t.Fatal("팀 행 없음")
	}
	if rep.AdminLead.Sample != 1 {
		t.Fatalf("주간 리드타임 표본=%d want 1", rep.AdminLead.Sample)
	}
	if rep.AdminLead.WaitDaysSum < 0.9 || rep.AdminLead.WaitDaysSum > 1.1 {
		t.Fatalf("주간 외부대기=%v want ~1", rep.AdminLead.WaitDaysSum)
	}
	foundWait := false
	for _, e := range rep.Events {
		if strings.Contains(e.Content, "□□정보 박OO") && strings.Contains(e.Content, "2026-08-15") && e.Result == "회신 대기" {
			foundWait = true
			break
		}
	}
	if !foundWait {
		t.Fatalf("회신 대기 행(대기 대상·회신예정) 없음: %+v", rep.Events)
	}
}

func TestBuildStatsChartBucketsForRange(t *testing.T) {
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.Local)
	to := time.Date(2026, 8, 7, 0, 0, 0, 0, time.Local)
	_, _, buckets := BuildStatsChartBucketsForRange(from, to)
	if len(buckets) != 7 {
		t.Fatalf("days=%d want 7", len(buckets))
	}
	from2 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	to2 := time.Date(2026, 8, 14, 0, 0, 0, 0, time.Local)
	_, _, monthB := BuildStatsChartBucketsForRange(from2, to2)
	if len(monthB) != 8 {
		t.Fatalf("months=%d want 8", len(monthB))
	}
}

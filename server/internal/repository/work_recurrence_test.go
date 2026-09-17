package repository

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

var septEvery3 = []string{
	"2026-09-01", "2026-09-04", "2026-09-07", "2026-09-10", "2026-09-13",
	"2026-09-16", "2026-09-19", "2026-09-22", "2026-09-25", "2026-09-28",
}

func TestGenerateOccurrencesCopiesParentAndDates(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "occ.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	wb := NewWBRepo(db)
	proj := &model.WorkProject{Name: "반복실행 사업", ContractType: "private", BillingType: "monthly", StartDate: "2026-01-01", EndDate: "2026-12-31"}
	if err := wb.CreateProject(proj); err != nil {
		t.Fatal(err)
	}
	parent := &model.WorkTask{
		WorkType: model.WBWorkSupport, ProjectID: proj.ProjectID, Title: "3일마다 확인",
		DueDate: "2026-09-30", DurationMin: 45, Assignee: "양기헌", Tags: "정기",
		Status: model.WBTaskWaiting,
	}
	if err := wb.CreateTask(parent); err != nil {
		t.Fatal(err)
	}
	if err := wb.ReplaceSupportMembers(parent.TaskID, []model.WorkTaskMember{{Assignee: "이해진", Role: model.WBMemberSupport}}); err != nil {
		t.Fatal(err)
	}
	parent, _ = wb.GetTask(parent.TaskID)

	res, err := wb.GenerateOccurrences(parent, model.WorkRecurrence{
		StartDate: "2026-09-01", EndDate: "2026-09-30",
		RuleType: model.RecurrenceEveryNDays, IntervalN: 3,
		HolidayPolicy: model.HolidayPolicyAsIs, CompletePolicy: model.CompletePolicyManual,
	}, septEvery3)
	if err != nil || res.Created != 10 {
		t.Fatalf("gen created=%d err=%v", res.Created, err)
	}
	children, err := wb.ListChildren(parent.TaskID)
	if err != nil || len(children) != 10 {
		t.Fatalf("children=%d err=%v", len(children), err)
	}
	for i, ch := range children {
		if ch.RecurrenceRole != model.RecurrenceRoleOccurrence {
			t.Fatalf("role=%s", ch.RecurrenceRole)
		}
		if ch.WorkDate != septEvery3[i] {
			t.Fatalf("work_date=%s want %s", ch.WorkDate, septEvery3[i])
		}
		if ch.DueDate != "2026-09-30" {
			t.Fatalf("실행 작업 due_date=%s (최종 마감을 복사해야 함)", ch.DueDate)
		}
		if ch.WorkType != model.WBWorkSupport || ch.ProjectID != proj.ProjectID || ch.Assignee != "양기헌" {
			t.Fatalf("복사 누락: %+v", ch)
		}
		if ch.ParentTaskID != parent.TaskID {
			t.Fatalf("parent=%s", ch.ParentTaskID)
		}
		mem, _ := wb.ListMembers(ch.TaskID)
		support := 0
		for _, m := range mem {
			if m.IsSupport() && m.Assignee == "이해진" {
				support++
			}
		}
		if support != 1 {
			t.Fatalf("지원 담당 미복사 task=%s mem=%+v", ch.TaskID, mem)
		}
	}
	got, _ := wb.GetTask(parent.TaskID)
	if got.RecurrenceRole != model.RecurrenceRoleParent {
		t.Fatalf("parent role=%s", got.RecurrenceRole)
	}
	if _, err := wb.GenerateOccurrences(parent, model.WorkRecurrence{}, septEvery3); err == nil {
		t.Fatal("중복 생성은 거부해야 한다")
	}

	children[0].Status = model.WBTaskComplete
	children[0].OccurrenceStatus = model.OccurrenceComplete
	if err := wb.UpdateTask(&children[0]); err != nil {
		t.Fatal(err)
	}
	keepID := children[0].TaskID
	res, err = wb.RegenerateOccurrences(parent, model.WorkRecurrence{
		StartDate: "2026-09-01", EndDate: "2026-09-30",
		RuleType: model.RecurrenceEveryNDays, IntervalN: 3,
		HolidayPolicy: model.HolidayPolicyAsIs,
	}, septEvery3)
	if err != nil {
		t.Fatal(err)
	}
	if res.KeptComplete != 1 || res.Created != 9 {
		t.Fatalf("regen %+v report=%s", res, res.Report())
	}
	still, err := wb.GetTask(keepID)
	if err != nil || still == nil || still.Status != model.WBTaskComplete {
		t.Fatalf("완료 회차가 지워졌다: %+v err=%v", still, err)
	}
}

func TestOccurrenceKPIUnchangedForAugustView(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "kpi.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('c1','도서관','도서관',1)`); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date, status, assigned_to, complete_datetime)
		VALUES ('a1','R1','c1','2026-08-01','2026-08-03','completed','양기헌','2026-08-05')`)
	if err != nil {
		t.Fatal(err)
	}
	stats := NewStatsRepo(db)
	anchor := time.Date(2026, 8, 7, 0, 0, 0, 0, time.Local)
	cols := BuildStatsPeriodColumns(model.StatsViewMonth, anchor)
	f := ParseMeetingFilter(model.StatsScopeTeam, "", "")
	if err := stats.FillPeriodOverview(cols, f); err != nil {
		t.Fatal(err)
	}
	before, err := stats.LoadStatsKPI(model.StatsViewMonth, cols, f)
	if err != nil {
		t.Fatal(err)
	}

	wb := NewWBRepo(db)
	parent := &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "9월 반복", DueDate: "2026-09-30",
		Assignee: "양기헌", Status: model.WBTaskWaiting,
	}
	if err := wb.CreateTask(parent); err != nil {
		t.Fatal(err)
	}
	if _, err := wb.GenerateOccurrences(parent, model.WorkRecurrence{
		StartDate: "2026-09-01", EndDate: "2026-09-30",
		RuleType: model.RecurrenceEveryNDays, IntervalN: 3,
		HolidayPolicy: model.HolidayPolicyAsIs,
	}, septEvery3); err != nil {
		t.Fatal(err)
	}

	cols2 := BuildStatsPeriodColumns(model.StatsViewMonth, anchor)
	if err := stats.FillPeriodOverview(cols2, f); err != nil {
		t.Fatal(err)
	}
	after, err := stats.LoadStatsKPI(model.StatsViewMonth, cols2, f)
	if err != nil {
		t.Fatal(err)
	}
	if before.VisitAvgDays != after.VisitAvgDays || before.CompleteAvgDays != after.CompleteAvgDays {
		t.Fatalf("8월 KPI가 바뀌었다 before=%+v after=%+v", before, after)
	}
}

func TestMarkPastOccurrencesOverdueLeavesFuture(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "overdue.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	wb := NewWBRepo(db)
	today := time.Now().Format("2006-01-02")
	past := time.Now().AddDate(0, 0, -5).Format("2006-01-02")
	future := time.Now().AddDate(0, 0, 7).Format("2006-01-02")
	parent := &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "반복 확인", DueDate: future,
		Assignee: "양기헌", Status: model.WBTaskWaiting,
	}
	if err := wb.CreateTask(parent); err != nil {
		t.Fatal(err)
	}
	if _, err := wb.GenerateOccurrences(parent, model.WorkRecurrence{
		StartDate: past, EndDate: future, RuleType: model.RecurrenceManual,
		HolidayPolicy: model.HolidayPolicyAsIs, CompletePolicy: model.CompletePolicyManual,
	}, []string{past, today, future}); err != nil {
		t.Fatal(err)
	}
	n, err := wb.MarkPastOccurrencesOverdue(today)
	if err != nil || n < 1 {
		t.Fatalf("marked=%d err=%v", n, err)
	}
	children, _ := wb.ListChildren(parent.TaskID)
	if len(children) != 3 {
		t.Fatalf("다음 회차가 지워졌다 children=%d", len(children))
	}
	byDate := map[string]model.WorkTask{}
	for _, ch := range children {
		byDate[ch.WorkDate] = ch
	}
	if byDate[past].OccurrenceStatus != model.OccurrenceOverdue {
		t.Fatalf("과거 상태=%s", byDate[past].OccurrenceStatus)
	}
	if byDate[today].OccurrenceStatus != model.OccurrenceScheduled {
		t.Fatalf("당일까지 미완료가 되면 안 됨 status=%s", byDate[today].OccurrenceStatus)
	}
	if byDate[future].OccurrenceStatus != model.OccurrenceScheduled || byDate[future].WorkDate != future {
		t.Fatalf("미래 회차가 바뀌었다 %+v", byDate[future])
	}

	board := NewWorkBoardRepo(db)
	items, counts, err := board.ListUnplanned("", nil, model.UnplannedDelayed)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Delayed < 1 {
		t.Fatalf("미계획 예정일경과=%d", counts.Delayed)
	}
	found := false
	for _, it := range items {
		if it.RefID == byDate[past].TaskID && it.HasKind(model.UnplannedDelayed) {
			found = true
			if it.CanAssignDate {
				t.Fatal("실행 작업은 미계획함에서 일자를 바꾸면 안 된다")
			}
			if it.SubLabel != "미완료" {
				t.Fatalf("SubLabel=%s", it.SubLabel)
			}
		}
		if it.RefID == byDate[future].TaskID {
			t.Fatal("미래 실행 작업이 일정이 지난 건에 올랐다")
		}
	}
	if !found {
		t.Fatal("미완료 실행 작업이 미계획 업무함에 없다")
	}
}

func TestCompletePolicyAutoManualRequireResult(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "policy.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	wb := NewWBRepo(db)
	dates := []string{"2026-09-01", "2026-09-04"}

	autoParent := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "자동완료", DueDate: "2026-09-30", Status: model.WBTaskWaiting, Assignee: "양기헌"}
	if err := wb.CreateTask(autoParent); err != nil {
		t.Fatal(err)
	}
	if _, err := wb.GenerateOccurrences(autoParent, model.WorkRecurrence{
		StartDate: "2026-09-01", EndDate: "2026-09-30", RuleType: model.RecurrenceManual,
		CompletePolicy: model.CompletePolicyAuto,
	}, dates); err != nil {
		t.Fatal(err)
	}
	chs, _ := wb.ListChildren(autoParent.TaskID)
	if err := wb.UpdateOccurrenceFields(chs[0].TaskID, model.OccurrenceComplete, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := wb.UpdateOccurrenceFields(chs[1].TaskID, model.OccurrenceSkipped, "휴일 제외", ""); err != nil {
		t.Fatal(err)
	}
	if err := wb.MaybeAutoCompleteParent(autoParent.TaskID); err != nil {
		t.Fatal(err)
	}
	got, _ := wb.GetTask(autoParent.TaskID)
	if got.Status != model.WBTaskComplete {
		t.Fatalf("auto 상위 상태=%s", got.Status)
	}
	still, _ := wb.GetTask(chs[1].TaskID)
	if still == nil || still.OccurrenceStatus != model.OccurrenceSkipped {
		t.Fatalf("제외 기록이 지워졌다 %+v", still)
	}

	manualParent := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "수동완료", DueDate: "2026-09-30", Status: model.WBTaskWaiting, Assignee: "양기헌"}
	if err := wb.CreateTask(manualParent); err != nil {
		t.Fatal(err)
	}
	if _, err := wb.GenerateOccurrences(manualParent, model.WorkRecurrence{
		StartDate: "2026-09-01", EndDate: "2026-09-30", RuleType: model.RecurrenceManual,
		CompletePolicy: model.CompletePolicyManual,
	}, dates); err != nil {
		t.Fatal(err)
	}
	mchs, _ := wb.ListChildren(manualParent.TaskID)
	_ = wb.UpdateOccurrenceFields(mchs[0].TaskID, model.OccurrenceComplete, "", "")
	_ = wb.UpdateOccurrenceFields(mchs[1].TaskID, model.OccurrenceComplete, "", "")
	_ = wb.MaybeAutoCompleteParent(manualParent.TaskID)
	got, _ = wb.GetTask(manualParent.TaskID)
	if got.Status == model.WBTaskComplete {
		t.Fatal("manual은 담당자가 완료하기 전에 상위가 끝나면 안 된다")
	}

	reqParent := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "결과필수", DueDate: "2026-09-30", Status: model.WBTaskWaiting, Assignee: "양기헌"}
	if err := wb.CreateTask(reqParent); err != nil {
		t.Fatal(err)
	}
	if _, err := wb.GenerateOccurrences(reqParent, model.WorkRecurrence{
		StartDate: "2026-09-01", EndDate: "2026-09-30", RuleType: model.RecurrenceManual,
		CompletePolicy: model.CompletePolicyRequireResult,
	}, dates); err != nil {
		t.Fatal(err)
	}
	rchs, _ := wb.ListChildren(reqParent.TaskID)
	_ = wb.UpdateOccurrenceFields(rchs[0].TaskID, model.OccurrenceComplete, "", "")
	_ = wb.UpdateOccurrenceFields(rchs[1].TaskID, model.OccurrenceComplete, "", "")
	_ = wb.MaybeAutoCompleteParent(reqParent.TaskID)
	got, _ = wb.GetTask(reqParent.TaskID)
	if got.Status == model.WBTaskComplete {
		t.Fatal("최종 결과 없이 require_result가 완료되면 안 된다")
	}
	if err := wb.SetRecurrenceFinalResult(reqParent.TaskID, "9월 확인 종료"); err != nil {
		t.Fatal(err)
	}
	if err := wb.MaybeAutoCompleteParent(reqParent.TaskID); err != nil {
		t.Fatal(err)
	}
	got, _ = wb.GetTask(reqParent.TaskID)
	if got.Status != model.WBTaskComplete {
		t.Fatalf("최종 결과 후 상위 상태=%s", got.Status)
	}
}

func TestParentCompleteKeepsOpenOccurrenceRows(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "keep.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	wb := NewWBRepo(db)
	parent := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "기록유지", DueDate: "2026-09-30", Status: model.WBTaskWaiting, Assignee: "양기헌"}
	if err := wb.CreateTask(parent); err != nil {
		t.Fatal(err)
	}
	if _, err := wb.GenerateOccurrences(parent, model.WorkRecurrence{
		StartDate: "2026-09-01", EndDate: "2026-09-30", RuleType: model.RecurrenceManual,
		CompletePolicy: model.CompletePolicyManual,
	}, []string{"2026-09-01", "2026-09-04"}); err != nil {
		t.Fatal(err)
	}
	chs, _ := wb.ListChildren(parent.TaskID)
	openID := chs[0].TaskID
	if err := wb.UpdateOccurrenceFields(openID, model.OccurrenceOverdue, "미처리", ""); err != nil {
		t.Fatal(err)
	}
	parent.Status = model.WBTaskComplete
	parent.CompleteNote = "상위로 종결"
	if err := wb.UpdateTask(parent); err != nil {
		t.Fatal(err)
	}
	left, err := wb.GetTask(openID)
	if err != nil || left == nil {
		t.Fatalf("미완료 실행 작업이 지워졌다 err=%v", err)
	}
	if left.OccurrenceStatus != model.OccurrenceOverdue {
		t.Fatalf("미완료 기록이 바뀌었다 %+v", left)
	}
	n, _ := wb.CountOccurrences(parent.TaskID)
	if n != 2 {
		t.Fatalf("실행 작업 수=%d", n)
	}
}

func TestOccurrenceChangeModesAndDeleteProtect(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "chg.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	wb := NewWBRepo(db)
	today := time.Now().Format("2006-01-02")
	past := time.Now().AddDate(0, 0, -5).Format("2006-01-02")
	future := time.Now().AddDate(0, 0, 10).Format("2006-01-02")
	future2 := time.Now().AddDate(0, 0, 17).Format("2006-01-02")
	parent := &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "일정변경", DueDate: future2,
		Assignee: "양기헌", Status: model.WBTaskWaiting,
	}
	if err := wb.CreateTask(parent); err != nil {
		t.Fatal(err)
	}
	if _, err := wb.GenerateOccurrences(parent, model.WorkRecurrence{
		StartDate: past, EndDate: future, RuleType: model.RecurrenceManual,
		CompletePolicy: model.CompletePolicyManual,
	}, []string{past, future}); err != nil {
		t.Fatal(err)
	}
	chs, _ := wb.ListChildren(parent.TaskID)
	var pastID, futureID string
	for _, ch := range chs {
		switch ch.WorkDate {
		case past:
			pastID = ch.TaskID
		case future:
			futureID = ch.TaskID
		}
	}
	if err := wb.UpdateOccurrenceFields(pastID, model.OccurrenceComplete, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := wb.CreateActivity(&model.WorkActivity{
		TaskID: pastID, Content: "8월 회차 확인 완료", Actor: "양기헌",
	}); err != nil {
		t.Fatal(err)
	}

	keepAdd, err := wb.ApplyOccurrenceChange(parent, model.WorkRecurrence{
		StartDate: past, EndDate: future2, RuleType: model.RecurrenceManual,
		CompletePolicy: model.CompletePolicyManual,
	}, []string{past, future, future2}, model.RecurrenceChangeKeepAdd, today)
	if err != nil {
		t.Fatal(err)
	}
	if keepAdd.Deleted != 0 || keepAdd.Created != 1 {
		t.Fatalf("keep_add %+v", keepAdd)
	}
	if got, _ := wb.GetTask(futureID); got == nil {
		t.Fatal("keep_add가 기존 미래를 지웠다")
	}

	res, err := wb.ApplyOccurrenceChange(parent, model.WorkRecurrence{
		StartDate: past, EndDate: future2, RuleType: model.RecurrenceManual,
		CompletePolicy: model.CompletePolicyManual,
	}, []string{past, future, future2}, model.RecurrenceChangeFuture, today)
	if err != nil {
		t.Fatal(err)
	}
	if res.KeptComplete != 1 {
		t.Fatalf("완료 유지 %+v", res)
	}
	still, _ := wb.GetTask(pastID)
	if still == nil || still.OccurrenceStatus != model.OccurrenceComplete {
		t.Fatalf("완료 회차가 바뀌었다 %+v", still)
	}
	acts, _ := wb.ListActivities(pastID)
	if len(acts) != 1 || acts[0].Content != "8월 회차 확인 완료" {
		t.Fatalf("처리 기록이 지워졌다 %+v", acts)
	}
	if got, _ := wb.GetTask(futureID); got != nil {
		t.Fatalf("미래 미완료가 남아 있다 %+v deleted=%d", got, res.Deleted)
	}

	if err := wb.DeleteParentTask(parent.TaskID); err == nil {
		t.Fatal("완료 회차가 있는 상위는 삭제가 거부돼야 한다")
	} else if !strings.Contains(err.Error(), "보관") {
		t.Fatalf("보관 안내 없음: %v", err)
	}
	if got, _ := wb.GetTask(parent.TaskID); got == nil {
		t.Fatal("거부했는데 상위가 지워졌다")
	}
	if got, _ := wb.GetTask(pastID); got == nil {
		t.Fatal("삭제 거부 시 완료 회차도 남아야 한다")
	}

	if err := wb.SetRecurrenceArchived(parent.TaskID, true); err != nil {
		t.Fatal(err)
	}
	listed, _ := wb.ListTasks()
	for _, it := range listed {
		if it.TaskID == parent.TaskID || it.TaskID == pastID {
			t.Fatalf("보관한 상위·실행이 목록에 남음 %s", it.TaskID)
		}
	}
	if got, _ := wb.GetTask(pastID); got == nil {
		t.Fatal("보관이 완료 회차를 지웠다")
	}
}

func TestRecurrenceDashboardAlerts(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "dash_alert.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	wb := NewWBRepo(db)
	today := time.Now().Format("2006-01-02")
	soon := time.Now().AddDate(0, 0, 2).Format("2006-01-02")
	pastDue := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	parent := &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "마감 반복", DueDate: pastDue,
		Assignee: "양기헌", Status: model.WBTaskWaiting,
	}
	if err := wb.CreateTask(parent); err != nil {
		t.Fatal(err)
	}
	if _, err := wb.GenerateOccurrences(parent, model.WorkRecurrence{
		StartDate: soon, EndDate: soon, RuleType: model.RecurrenceManual,
		HolidayPolicy: model.HolidayPolicyAsIs, CompletePolicy: model.CompletePolicyManual,
	}, []string{soon}); err != nil {
		t.Fatal(err)
	}
	st, err := NewWorkBoardRepo(db).DashStats("", nil)
	if err != nil {
		t.Fatal(err)
	}
	if st.UpcomingOcc != 1 {
		t.Fatalf("다가오는 실행=%d want 1 (today=%s soon=%s)", st.UpcomingOcc, today, soon)
	}
	if st.OverdueDeadline != 1 {
		t.Fatalf("마감 경과=%d want 1", st.OverdueDeadline)
	}
}

func TestUpsertRecurrenceUnitFields(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "rec_units.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	wb := NewWBRepo(db)
	parent := &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "분기 보고",
		DueDate: "2026-12-31", Assignee: "양기헌", Status: model.WBTaskWaiting,
	}
	if err := wb.CreateTask(parent); err != nil {
		t.Fatal(err)
	}
	rule := model.WorkRecurrence{
		TaskID: parent.TaskID, StartDate: "2026-01-01", EndDate: "2026-12-31",
		RuleType: model.RecurrenceQuarterly, MonthN: 1, MonthDay: 1,
		HolidayPolicy: model.HolidayPolicyAsIs, CompletePolicy: model.CompletePolicyManual,
		LastWorkday: false, ManualDates: []string{"2026-03-01"},
	}
	if err := wb.UpsertRecurrence(rule); err != nil {
		t.Fatal(err)
	}
	got, err := wb.GetRecurrence(parent.TaskID)
	if err != nil || got == nil {
		t.Fatalf("get err=%v", err)
	}
	if got.RuleType != model.RecurrenceQuarterly || got.MonthN != 1 || got.MonthDay != 1 {
		t.Fatalf("quarterly fields %+v", got)
	}
	rule.RuleType = model.RecurrenceMonthly
	rule.LastWorkday = true
	rule.MonthDay = 0
	rule.ManualDates = nil
	if err := wb.UpsertRecurrence(rule); err != nil {
		t.Fatal(err)
	}
	got, _ = wb.GetRecurrence(parent.TaskID)
	if got == nil || !got.LastWorkday || got.RuleType != model.RecurrenceMonthly {
		t.Fatalf("monthly last %+v", got)
	}
	rule.RuleType = model.RecurrenceManual
	rule.LastWorkday = false
	rule.ManualDates = []string{"2026-09-24", "2026-10-03"}
	if err := wb.UpsertRecurrence(rule); err != nil {
		t.Fatal(err)
	}
	got, _ = wb.GetRecurrence(parent.TaskID)
	if got == nil || strings.Join(got.ManualDates, ",") != "2026-09-24,2026-10-03" {
		t.Fatalf("manual dates %+v", got)
	}
}

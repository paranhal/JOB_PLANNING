package repository

import (
	"path/filepath"
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
	if before.ExecutionRate != after.ExecutionRate || before.ExecutionSample != after.ExecutionSample ||
		before.VisitAvgDays != after.VisitAvgDays || before.CompleteAvgDays != after.CompleteAvgDays {
		t.Fatalf("8월 KPI가 바뀌었다 before=%+v after=%+v", before, after)
	}
}

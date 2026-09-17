package repository

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

// TestRecurrenceAggregationUnits §13.15.9 · §13.15.14 8·10
//
// recurrence_role 을 넣어야 하는 쿼리(반영 단위는 기획 표 그대로, 임의 산식 없음):
//
// | 지표 | 파일/함수 | 단위 | 조건 |
// | 예정 건수 | countAdminSlice | 실행 작업 | != parent, 예정일=work_date |
// | 주간보고 완료 건수 | countAdminCompletedParents · collapseWeeklyReportEvents | 상위 1건+횟수 | DISTINCT parent |
// | 담당자별 일일업무 | listCompletedByAssigneeDate · ListWeeklyEventRows | 실행 작업 | != parent, 접지 않음 |
// | 일정표 | ListTasksBetween | 실행 작업 | parent 제외 |
// | 미계획함 | queryWorkTasks(미완료 회차) | 실행 작업 | role=occurrence |
// | 전사 F열 | listCompanyAdminDone/Plan | 상위 1건+횟수 | collapse |
func TestRecurrenceAggregationUnits(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "rec_unit.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	wb := NewWBRepo(db)
	parent := &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "정기 데이터 확인", DueDate: "2026-09-30",
		Assignee: "양기헌", Status: model.WBTaskWaiting,
	}
	if err := wb.CreateTask(parent); err != nil {
		t.Fatal(err)
	}
	if _, err := wb.GenerateOccurrences(parent, model.WorkRecurrence{
		StartDate: "2026-09-01", EndDate: "2026-09-30",
		RuleType: model.RecurrenceEveryNDays, IntervalN: 3,
		HolidayPolicy: model.HolidayPolicyAsIs, CompletePolicy: model.CompletePolicyManual,
	}, septEvery3); err != nil {
		t.Fatal(err)
	}
	chs, _ := wb.ListChildren(parent.TaskID)
	if len(chs) != 10 {
		t.Fatalf("실행 작업 %d", len(chs))
	}
	for i := 0; i < 4; i++ {
		if err := wb.UpdateOccurrenceFields(chs[i].TaskID, model.OccurrenceComplete, "", ""); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE work_tasks SET complete_date=? WHERE task_id=?`, chs[i].WorkDate, chs[i].TaskID); err != nil {
			t.Fatal(err)
		}
	}

	from, toEx := "2026-09-01", "2026-10-01"
	var naivePlanned, naiveComplete int
	_ = db.QueryRow(`
		SELECT COUNT(*) FROM work_tasks t
		WHERE t.work_type IN ('admin','support')
		  AND COALESCE(NULLIF(TRIM(t.work_date),''), NULLIF(TRIM(t.due_date),''), '') >= ?
		  AND COALESCE(NULLIF(TRIM(t.work_date),''), NULLIF(TRIM(t.due_date),''), '') < ?`, from, toEx).Scan(&naivePlanned)
	_ = db.QueryRow(`
		SELECT COUNT(*) FROM work_tasks t
		WHERE t.work_type IN ('admin','support') AND t.status='complete'
		  AND date(NULLIF(TRIM(t.complete_date),'')) >= ?
		  AND date(NULLIF(TRIM(t.complete_date),'')) < ?`, from, toEx).Scan(&naiveComplete)

	stats := NewStatsRepo(db)
	f := ParseMeetingFilter(model.StatsScopeTeam, "", "")
	slice, err := stats.countAdminSlice(from, toEx, f)
	if err != nil {
		t.Fatal(err)
	}
	parents, err := stats.countAdminCompletedParents(from, toEx, f)
	if err != nil {
		t.Fatal(err)
	}
	an, err := stats.LoadStatsWorkAnalysis(from, toEx, f)
	if err != nil {
		t.Fatal(err)
	}
	week, err := stats.BuildWeeklyReport(time.Date(2026, 9, 2, 0, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	rawEvents, err := stats.ListWeeklyEventRows(week.WeekFrom, week.WeekToEx)
	if err != nil {
		t.Fatal(err)
	}
	var adminRaw, adminWeek int
	for _, ev := range rawEvents {
		if ev.Kind == model.WeeklyEventAdmin {
			adminRaw++
		}
	}
	for _, ev := range week.Events {
		if ev.Kind == model.WeeklyEventAdmin {
			adminWeek++
		}
	}
	dayItems, _ := wb.ListTasksBetween("2026-09-01", "2026-09-01")
	var dayN int
	for _, it := range dayItems {
		if it.TaskID == parent.TaskID {
			t.Fatal("일정표에 상위가 올라왔다")
		}
		if it.ParentTaskID == parent.TaskID {
			dayN++
		}
	}

	t.Logf("반영 전후 비교표 (9월 3일마다 10회 · 완료 4)")
	t.Logf("| 지표 | 반영 전 | 반영 후 | 단위 |")
	t.Logf("| 9월 예정 | %d | %d | 실행 작업 |", naivePlanned, slice.Planned)
	t.Logf("| 9월 완료 행 | %d | %d | 상위 1건 |", naiveComplete, parents)
	t.Logf("| 주간 완료 | (행 %d) | %d | 상위 1건 |", naiveComplete, an.Admin.Completed)
	t.Logf("| 주간 이벤트 | %d | %d | 1건+횟수 |", adminRaw, adminWeek)
	t.Logf("| 9/1 일정표 | — | %d | 실행 작업 |", dayN)

	if naivePlanned != 11 {
		t.Fatalf("반영 전 예정=%d want 11 (상위+실행 이중)", naivePlanned)
	}
	if slice.Planned != 10 {
		t.Fatalf("예정=%d want 10", slice.Planned)
	}
	if parents != 1 || an.Admin.Completed != 1 {
		t.Fatalf("주간 완료 상위=%d analysis=%d want 1", parents, an.Admin.Completed)
	}
	if adminRaw < 2 {
		t.Fatalf("담당자별 원사건=%d (실행 작업이어야 함)", adminRaw)
	}
	if adminWeek != 1 {
		t.Fatalf("주간 이벤트=%d want 1", adminWeek)
	}
	var note string
	for _, ev := range week.Events {
		if ev.Kind == model.WeeklyEventAdmin {
			note = ev.Content
		}
	}
	if !strings.Contains(note, "10회 중 4회") {
		t.Fatalf("횟수 부기 없음: %s", note)
	}
	if dayN != 1 {
		t.Fatalf("9/1 일정표=%d want 1", dayN)
	}
	for _, ev := range rawEvents {
		if ev.WorkNo == parent.TaskID {
			t.Fatal("주간 이벤트에 상위가 올라와 이중 계상")
		}
	}
	weekAn, err := stats.LoadStatsWorkAnalysis(week.WeekFrom, week.WeekToEx, f)
	if err != nil {
		t.Fatal(err)
	}
	if weekAn.Admin.Completed != 1 {
		t.Fatalf("그 주 완료 상위=%d want 1", weekAn.Admin.Completed)
	}
	if len(week.PersonRows) == 0 || week.PersonRows[0].Completed != weekAn.Completed {
		t.Fatalf("주간 팀 완료 행이 analysis 와 다름")
	}
	fcol, err := stats.listCompanyAdminDone("", from, toEx)
	if err != nil {
		t.Fatal(err)
	}
	if len(fcol) != 1 || !strings.Contains(fcol[0].Title, "4회") {
		t.Fatalf("전사 F열=%v", fcol)
	}
}

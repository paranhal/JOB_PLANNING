package model

import "testing"

func TestCalcOccurrenceProgressExcludesSkippedAndFuture(t *testing.T) {
	today := "2026-08-27"
	items := []WorkTask{
		{RecurrenceRole: RecurrenceRoleOccurrence, WorkDate: "2026-08-01", OccurrenceStatus: OccurrenceComplete, Status: WBTaskComplete},
		{RecurrenceRole: RecurrenceRoleOccurrence, WorkDate: "2026-08-04", OccurrenceStatus: OccurrenceComplete, Status: WBTaskComplete},
		{RecurrenceRole: RecurrenceRoleOccurrence, WorkDate: "2026-08-07", OccurrenceStatus: OccurrenceOverdue},
		{RecurrenceRole: RecurrenceRoleOccurrence, WorkDate: "2026-08-10", OccurrenceStatus: OccurrenceSkipped},
		{RecurrenceRole: RecurrenceRoleOccurrence, WorkDate: "2026-09-01", OccurrenceStatus: OccurrenceScheduled},
		{RecurrenceRole: RecurrenceRoleOccurrence, WorkDate: "2026-09-04", OccurrenceStatus: OccurrenceScheduled},
		{Title: "일반 하위", WorkDate: "2026-08-01"},
	}
	ex := CalcOccurrenceProgress(items, false, today)
	if !ex.Has || ex.Target != 3 || ex.Complete != 2 || ex.Percent != 66 || ex.Open != 3 {
		t.Fatalf("미래 제외: %+v want target=3 complete=2 pct=66 open=3", ex)
	}
	in := CalcOccurrenceProgress(items, true, today)
	if !in.Has || in.Target != 5 || in.Complete != 2 || in.Percent != 40 {
		t.Fatalf("미래 포함: %+v want target=5 complete=2 pct=40", in)
	}
}

func TestOccurrenceOpenIgnoresSkippedAndComplete(t *testing.T) {
	if OccurrenceOpen(WorkTask{OccurrenceStatus: OccurrenceSkipped}) {
		t.Fatal("제외는 미완료가 아니다")
	}
	if OccurrenceOpen(WorkTask{Status: WBTaskComplete, OccurrenceStatus: OccurrenceScheduled}) {
		t.Fatal("업무 완료면 열린 회차가 아니다")
	}
	if !OccurrenceOpen(WorkTask{OccurrenceStatus: OccurrenceOverdue}) {
		t.Fatal("미완료는 열린 회차다")
	}
}

func TestAnnotateOccurrenceChangeKeepAddShowsDuplicates(t *testing.T) {
	prev := OccurrencePreview{Dates: []string{"2026-09-01", "2026-09-08", "2026-09-15"}, Count: 3}
	children := []WorkTask{
		{RecurrenceRole: RecurrenceRoleOccurrence, WorkDate: "2026-09-01", OccurrenceStatus: OccurrenceComplete, Status: WBTaskComplete},
		{RecurrenceRole: RecurrenceRoleOccurrence, WorkDate: "2026-09-04", OccurrenceStatus: OccurrenceScheduled},
	}
	AnnotateOccurrenceChange(&prev, children, RecurrenceChangeKeepAdd, "2026-08-27")
	if prev.WillDelete != 0 || prev.WillCreate != 2 || len(prev.Duplicates) != 1 || prev.Duplicates[0] != "2026-09-01" {
		t.Fatalf("keep_add %+v", prev)
	}
	if prev.KeptComplete != 1 {
		t.Fatalf("완료 유지=%d", prev.KeptComplete)
	}
}

func TestKeepOccurrenceOnChangeFutureLeavesPast(t *testing.T) {
	past := WorkTask{RecurrenceRole: RecurrenceRoleOccurrence, WorkDate: "2026-08-01", OccurrenceStatus: OccurrenceOverdue}
	future := WorkTask{RecurrenceRole: RecurrenceRoleOccurrence, WorkDate: "2026-09-01", OccurrenceStatus: OccurrenceScheduled}
	done := WorkTask{RecurrenceRole: RecurrenceRoleOccurrence, WorkDate: "2026-09-04", Status: WBTaskComplete, OccurrenceStatus: OccurrenceComplete}
	if !KeepOccurrenceOnChange(past, RecurrenceChangeFuture, "2026-08-27") {
		t.Fatal("지난 미완료는 미래만 다시 만들 때 남긴다")
	}
	if KeepOccurrenceOnChange(future, RecurrenceChangeFuture, "2026-08-27") {
		t.Fatal("미래 미완료는 지운다")
	}
	if !KeepOccurrenceOnChange(done, RecurrenceChangeReplace, "2026-08-27") {
		t.Fatal("완료는 어떤 선택에서도 남긴다")
	}
	if KeepOccurrenceOnChange(past, RecurrenceChangeReplace, "2026-08-27") {
		t.Fatal("replace는 지난 미완료도 지운다")
	}
}

func TestRecurrenceDisplayHelpers(t *testing.T) {
	if got := FormatRecurrenceProgress("정기 데이터 확인", 5, 4); got != "정기 데이터 확인 · 5회 중 4회" {
		t.Fatalf("progress=%s", got)
	}
	if got := FormatRecurrenceTimes("정기 데이터 확인", 5); got != "·정기 데이터 확인 5회" {
		t.Fatalf("times=%s", got)
	}
	n, ok := DaysUntil("2026-08-30", "2026-08-27")
	if !ok || n != 3 {
		t.Fatalf("D-n %d ok=%v", n, ok)
	}
	if DueUrgency(3, true) != "soon" || DueUrgency(-1, true) != "over" {
		t.Fatal("urgency")
	}
	if RecurrenceCycleLabel(WorkRecurrence{RuleType: RecurrenceEveryNDays, IntervalN: 3}) != "3일마다" {
		t.Fatal("cycle")
	}
	if RecurrenceCycleLabel(WorkRecurrence{RuleType: RecurrenceMonthly, MonthDay: 15}) != "매월 15일" {
		t.Fatal("monthly day")
	}
	if RecurrenceCycleLabel(WorkRecurrence{RuleType: RecurrenceMonthly, LastWorkday: true}) != "매월 마지막 영업일" {
		t.Fatal("monthly last")
	}
	if RecurrenceCycleLabel(WorkRecurrence{RuleType: RecurrenceQuarterly, MonthN: 1}) != "분기(1월 시작)" {
		t.Fatal("quarterly")
	}
	if RecurrenceCycleLabel(WorkRecurrence{RuleType: RecurrenceYearly, MonthN: 3, MonthDay: 15}) != "매년 3월 15일" {
		t.Fatal("yearly")
	}
	if RecurrenceRuleLabel(RecurrenceManual) != "지정일자" {
		t.Fatal("manual label")
	}
	if got := QuarterMonths(1); len(got) != 4 || got[0] != 1 || got[1] != 4 || got[2] != 7 || got[3] != 10 {
		t.Fatalf("Q1 start=%v", got)
	}
	if got := ClampMonthDay(2026, 2, 31).Format("2006-01-02"); got != "2026-02-28" {
		t.Fatalf("feb clamp=%s", got)
	}
}

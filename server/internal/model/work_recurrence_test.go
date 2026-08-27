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

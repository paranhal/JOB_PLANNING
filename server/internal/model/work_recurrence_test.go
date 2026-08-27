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

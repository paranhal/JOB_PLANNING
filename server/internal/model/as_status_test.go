package model

import (
	"testing"
	"time"
)

func TestApplyActionResult_Revisit(t *testing.T) {
	now := time.Date(2026, 7, 31, 10, 0, 0, 0, time.Local)
	as := &ASReceipt{ResultCode: ResultRevisit, RevisitReason: "부품대기"}
	if _, err := ApplyActionResult(as, ActionApplyInput{}, now); err != ErrRevisitDateRequired {
		t.Fatalf("want date required, got %v", err)
	}
	out, err := ApplyActionResult(as, ActionApplyInput{NextDate: "2026-08-05", ScheduleConfirmed: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	if as.Status != "in_progress" || as.VisitScheduledDate != "2026-08-05" || !as.ScheduleConfirmed {
		t.Fatalf("unexpected: %+v", as)
	}
	if len(out.WorkItems) != 1 || out.WorkItems[0].WorkKind != WorkKindRevisit {
		t.Fatalf("work: %+v", out.WorkItems)
	}
}

func TestApplyActionResult_TransferWaitingAndDone(t *testing.T) {
	now := time.Date(2026, 7, 31, 10, 0, 0, 0, time.Local)
	as := &ASReceipt{ResultCode: ResultTransfer}
	if _, err := ApplyActionResult(as, ActionApplyInput{}, now); err != ErrTransferDetailRequired {
		t.Fatalf("want detail required, got %v", err)
	}
	if _, err := ApplyActionResult(as, ActionApplyInput{TransferDetail: TransferDetailWaiting}, now); err != ErrConfirmDateRequired {
		t.Fatalf("want confirm date, got %v", err)
	}
	out, err := ApplyActionResult(as, ActionApplyInput{
		TransferDetail:    TransferDetailWaiting,
		NextDate:          "2026-08-07",
		ConfirmTarget:     "제조사A",
		ConfirmContact:    "010-1111-2222",
		ScheduleConfirmed: true,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if as.Status != "in_progress" || len(out.WorkItems) != 1 || out.WorkItems[0].WorkKind != WorkKindConfirm {
		t.Fatalf("waiting: status=%s work=%+v", as.Status, out.WorkItems)
	}

	as2 := &ASReceipt{ResultCode: ResultTransfer}
	if _, err := ApplyActionResult(as2, ActionApplyInput{TransferDetail: TransferDetailCompleted}, now); err != nil {
		t.Fatal(err)
	}
	if as2.Status != "completed" || as2.CompleteDatetime == nil {
		t.Fatalf("transfer completed: %+v", as2)
	}

	as3 := &ASReceipt{ResultCode: ResultDone}
	if _, err := ApplyActionResult(as3, ActionApplyInput{}, now); err != nil {
		t.Fatal(err)
	}
	if as3.Status != "completed" {
		t.Fatalf("done: %s", as3.Status)
	}
}

func TestIsOpenIncompleteStatus(t *testing.T) {
	for _, s := range []string{"received", "assigned", "in_progress", "hold", "transfer"} {
		if !IsOpenIncompleteStatus(s) {
			t.Fatalf("%s should be open", s)
		}
	}
	if IsOpenIncompleteStatus("completed") {
		t.Fatal("completed should not be open")
	}
}

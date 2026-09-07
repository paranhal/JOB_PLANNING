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
	if as.Status != StatusPartialComplete || as.VisitScheduledDate != "2026-08-05" || !as.ScheduleConfirmed {
		t.Fatalf("unexpected: %+v", as)
	}
	if as.CompleteDatetime == nil {
		t.Fatal("partial complete must set complete_datetime for stats")
	}
	if len(out.WorkItems) != 1 || out.WorkItems[0].WorkKind != WorkKindRevisit {
		t.Fatalf("work: %+v", out.WorkItems)
	}
	if !IsOpenIncompleteStatus(as.Status) || !IsStatsCompletedStatus(as.Status) {
		t.Fatal("partial_complete should be open ops + stats completed")
	}
}

func TestApplyActionResult_Partial(t *testing.T) {
	now := time.Date(2026, 7, 31, 10, 0, 0, 0, time.Local)
	as := &ASReceipt{ResultCode: ResultPartial, RevisitReason: "추가확인"}
	if _, err := ApplyActionResult(as, ActionApplyInput{}, now); err != ErrRevisitDateRequired {
		t.Fatalf("want date required, got %v", err)
	}
	out, err := ApplyActionResult(as, ActionApplyInput{NextDate: "2026-08-10"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if as.Status != StatusPartialComplete || as.CompleteDatetime == nil {
		t.Fatalf("partial: %+v", as)
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
	if as.Status != StatusPartialComplete || as.CompleteDatetime == nil || len(out.WorkItems) != 1 || out.WorkItems[0].WorkKind != WorkKindConfirm {
		t.Fatalf("waiting: status=%s complete=%v work=%+v", as.Status, as.CompleteDatetime, out.WorkItems)
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

	as4 := &ASReceipt{ResultCode: ResultTransfer}
	out4, err := ApplyActionResult(as4, ActionApplyInput{TransferDetail: TransferDetailFollowup}, now)
	if err != nil {
		t.Fatal(err)
	}
	if as4.Status != "completed" || as4.CompleteDatetime == nil {
		t.Fatalf("followup must complete original: %+v", as4)
	}
	if len(out4.WorkItems) != 0 {
		t.Fatalf("followup must not create work items: %+v", out4.WorkItems)
	}
}

func TestIsOpenIncompleteStatus(t *testing.T) {
	for _, s := range []string{"received", "assigned", "in_progress", "hold", "transfer", StatusPartialComplete} {
		if !IsOpenIncompleteStatus(s) {
			t.Fatalf("%s should be open", s)
		}
	}
	if IsOpenIncompleteStatus("completed") {
		t.Fatal("completed should not be open")
	}
	if CanReopenAS(StatusPartialComplete) {
		t.Fatal("partial_complete is not reopen target")
	}
	if !CanIssueASReport(StatusPartialComplete) || !CanIssueASReport("completed") || !CanIssueASReport("closed") {
		t.Fatal("완료·부분완료는 보고서 발급이 되어야 한다")
	}
	if CanIssueASReport("in_progress") {
		t.Fatal("진행중은 보고서 발급이 되면 안 된다")
	}
	if MapASStatusToWB(StatusPartialComplete) != WBTaskInProgress {
		t.Fatal("partial_complete should map to workboard in_progress")
	}
}

func TestActionResultLabels(t *testing.T) {
	if ActionResultLabel(ResultPartial) != "추가조치 필요" {
		t.Fatal(ActionResultLabel(ResultPartial))
	}
	if ActionResultLabel(ResultHold) != "대기" {
		t.Fatal(ActionResultLabel(ResultHold))
	}
	if TransferDetailLabel(TransferDetailWaiting) != "우리 팀 추가 작업" {
		t.Fatal(TransferDetailLabel(TransferDetailWaiting))
	}
	if TransferDetailLabel(TransferDetailFollowup) != "이관후속" {
		t.Fatal(TransferDetailLabel(TransferDetailFollowup))
	}
	if TransferDetailLabel(TransferDetailCompleted) != "이관 완료" {
		t.Fatal(TransferDetailLabel(TransferDetailCompleted))
	}
	opts := ActionResultOptions()
	if len(opts) != 3 || opts[1].Value != ResultPartial || opts[2].Value != ResultTransfer {
		t.Fatalf("%+v", opts)
	}
	if IsSelectableActionResult(ResultRevisit) || IsSelectableActionResult(ResultHold) {
		t.Fatal("재방문·대기는 조치 결과로 고를 수 없다")
	}
	if !IsSelectableActionResult(ResultDone) {
		t.Fatal("완료는 선택 가능해야 한다")
	}
}

func TestNewTransferFollowupReceipt_NotReopen(t *testing.T) {
	now := time.Date(2026, 8, 31, 10, 0, 0, 0, time.Local)
	src := &ASReceipt{
		ASID: "AS-SRC", CustomerID: "C1", AssetID: "A1",
		ReceiptChannel: "phone", Requester: "홍길동", RequesterType: "고객직접",
		RequesterName: "홍길동", Symptom: "게이트 오작동", AssignedTo: "양기헌",
	}
	child := NewTransferFollowupReceipt(src, "펌웨어 재설정", now)
	if child.IsReopen || !child.IsTransferFollowup() {
		t.Fatalf("이관후속이어야 한다: reopen=%v followup=%v", child.IsReopen, child.IsTransferFollowup())
	}
	if child.ParentASID != src.ASID || child.Symptom != src.Symptom || child.FollowupNote != "펌웨어 재설정" {
		t.Fatalf("%+v", child)
	}
	reopen := NewReopenReceipt(src, "재발", now)
	if !reopen.IsReopen || reopen.IsTransferFollowup() {
		t.Fatal("재접수는 이관후속이 아니다")
	}
}

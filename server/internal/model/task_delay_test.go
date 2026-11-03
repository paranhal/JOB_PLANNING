package model

import "testing"

func TestTaskDelayState_95_80(t *testing.T) {
	today := "2026-11-02"
	if TaskDelayState(WorkTask{DueDate: "2026-11-01", Status: WBTaskWaiting}, today).Days != 1 {
		t.Fatal("어제 종료 = 지연 1일")
	}
	if TaskDelayState(WorkTask{DueDate: today, Status: WBTaskWaiting}, today).Delayed {
		t.Fatal("종료일 오늘은 지연 아님")
	}
	if TaskDelayState(WorkTask{WorkDate: "2026-10-01", DueDate: "2026-11-04", Status: WBTaskWaiting}, today).Delayed {
		t.Fatal("시작 지남+종료 미래 = 진행중")
	}
	if TaskDelayState(WorkTask{WorkDate: "2026-10-01", Status: WBTaskWaiting}, today).Delayed {
		t.Fatal("종료일 없음 = 지연 아님")
	}
	if TaskDelayState(WorkTask{DueDate: "2026-10-01", Status: WBTaskComplete}, today).Delayed {
		t.Fatal("완료는 지연 아님")
	}
	if TaskDelayState(WorkTask{DueDate: "2026-10-01", Status: WBTaskCancelled}, today).Delayed {
		t.Fatal("취소는 지연 아님")
	}
	got := TaskDelayState(WorkTask{DueDate: "2026-11-01", Status: StatusPartialComplete}, today)
	if !got.Delayed || got.Days != 1 {
		t.Fatalf("부분완료+종료 지남 %+v", got)
	}
	as := WorkListTaskDelayState(WorkListItem{Prefix: WorkPrefixAS, ScheduledDate: "2026-11-01", Status: "in_progress"}, today)
	if !as.Delayed {
		t.Fatal("AS 예정일 경과 = 지연")
	}
	wa := WorkListTaskDelayState(WorkListItem{
		Prefix: WorkPrefixGeneral, SubLabel: "회신 대기", Status: WBActionWaiting,
		ScheduledDate: "2026-11-01", DueDate: today,
	}, today)
	if !wa.Delayed {
		t.Fatal("회신 예정 경과 = 지연")
	}
	occ := WorkListTaskDelayState(WorkListItem{
		Prefix: WorkPrefixGeneral, SubLabel: "미완료", Status: WBTaskWaiting,
		WorkDate: "2026-10-01", ScheduledDate: "2026-10-01", DueDate: "2026-11-04",
	}, today)
	if !occ.Delayed {
		t.Fatal("반복 회차일 경과 = 지연")
	}
	adm := WorkListTaskDelayState(WorkListItem{Prefix: WorkPrefixGeneral, WorkDate: "2026-10-01", Status: WBTaskWaiting}, today)
	if adm.Delayed {
		t.Fatal("행정 종료일 없음 = 지연 아님")
	}
	if DelayDueForStats("2026-01-01", "2026-11-04") != "2026-01-01" {
		t.Fatal("통계는 orig_due 우선")
	}
}

func TestTaskSpansDate(t *testing.T) {
	if !TaskSpansDate("2026-11-01", "2026-11-04", "2026-11-02") {
		t.Fatal("기간 중간")
	}
	if TaskSpansDate("2026-11-01", "2026-11-04", "2026-11-05") {
		t.Fatal("기간 밖")
	}
	if TaskIsSpan("2026-11-01", "2026-11-01") {
		t.Fatal("단일일")
	}
}

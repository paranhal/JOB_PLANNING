package model

import "testing"

func TestParseEncodeScheduleShow(t *testing.T) {
	p, a := ParseScheduleShow("")
	if !p || a {
		t.Fatal("기본은 접수만")
	}
	p, a = ParseScheduleShow("act")
	if p || !a {
		t.Fatal("act만")
	}
	p, a = ParseScheduleShow("plan,act")
	if !p || !a {
		t.Fatal("둘 다")
	}
	p, a = ParseScheduleShow("nope")
	if !p || a {
		t.Fatal("잘못된 값은 접수만")
	}
	if EncodeScheduleShow(true, false) != "plan" || EncodeScheduleShow(false, true) != "act" || EncodeScheduleShow(true, true) != "plan,act" {
		t.Fatal(EncodeScheduleShow(true, true))
	}
	if ToggleScheduleShow("plan", "plan") != "plan" {
		t.Fatal("둘 다 끄기 불가")
	}
	if ToggleScheduleShow("plan", "act") != "plan,act" {
		t.Fatal("조치 켜기")
	}
	if ToggleScheduleShow("plan,act", "plan") != "act" {
		t.Fatal("접수 끄기")
	}
}

func TestTaskPlanExecDate(t *testing.T) {
	t1 := WorkTask{WorkDate: "2026-10-10", DueDate: "2026-10-11"}
	if TaskPlanDate(t1) != "2026-10-10" {
		t.Fatal(TaskPlanDate(t1))
	}
	t2 := WorkTask{DueDate: "2026-10-11"}
	if TaskPlanDate(t2) != "2026-10-11" {
		t.Fatal("due fallback")
	}
	t3 := WorkTask{CompleteDate: "2026-10-20 09:00"}
	if TaskExecDate(t3) != "2026-10-20" {
		t.Fatal(TaskExecDate(t3))
	}
	t4 := WorkTask{Status: WBTaskComplete, WorkDate: "2026-10-10"}
	if TaskExecDate(t4) != "2026-10-10" {
		t.Fatal("완료 상태 폴백")
	}
}

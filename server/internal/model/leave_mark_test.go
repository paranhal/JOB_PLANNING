package model

import "testing"

func TestLeaveCellMarkKinds(t *testing.T) {
	if LeaveCellMark("") != "" {
		t.Fatal("빈 종류")
	}
	if LeaveCellMark(LeaveKindAnnual) != "연차" {
		t.Fatalf("연차=%q", LeaveCellMark(LeaveKindAnnual))
	}
	if LeaveCellMark(LeaveKindHalfAM) != "오전반차" {
		t.Fatalf("오전반차=%q", LeaveCellMark(LeaveKindHalfAM))
	}
	if LeaveCellMark(LeaveKindHalfPM) != "오후반차" {
		t.Fatalf("오후반차=%q", LeaveCellMark(LeaveKindHalfPM))
	}
}

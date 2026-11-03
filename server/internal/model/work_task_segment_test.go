package model

import "testing"

func TestSplitPlanAround_OO_XX(t *testing.T) {
	sp, err := SplitPlanAround("09:00", "18:00", "10:00", "14:00", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !sp.HasFront || sp.FrontStart != "09:00" || sp.FrontEnd != "10:00" {
		t.Fatalf("front %+v", sp)
	}
	if !sp.HasBack || sp.BackStart != "14:00" || sp.BackEnd != "18:00" {
		t.Fatalf("back %+v", sp)
	}
}

func TestSplitPlanAround_TravelSnapAndTooSmall(t *testing.T) {
	sp, err := SplitPlanAround("09:00", "12:00", "09:10", "11:50", 0)
	if err == nil && (sp.HasFront || sp.HasBack) {
		if sp.HasFront {
			t.Log(sp.FrontStart, sp.FrontEnd)
		}
	}
	_, err = SplitPlanAround("09:00", "09:20", "09:00", "09:20", 0)
	if err == nil {
		t.Fatal("남는 구간 없음인데 허용")
	}
	sp, err = SplitPlanAround("09:00", "18:00", "10:07", "14:08", 15)
	if err != nil {
		t.Fatal(err)
	}
	if sp.FrontEnd != "09:45" { // 10:07-15=09:52 snap down 09:45
		t.Fatalf("front end snap %s", sp.FrontEnd)
	}
	if sp.BackStart != "14:30" { // 14:08+15=14:23 snap up 14:30
		t.Fatalf("back start snap %s", sp.BackStart)
	}
}

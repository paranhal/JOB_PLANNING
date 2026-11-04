package model

import "testing"

func TestClassifyExecTodayBucketsAndPct(t *testing.T) {
	date := "2026-11-04"
	items := []WorkListItem{
		{Prefix: WorkPrefixGeneral, RefID: "d", Status: WBTaskComplete, OrgID: "O01"},
		{Prefix: WorkPrefixGeneral, RefID: "p", Status: WBTaskInProgress, OrgID: "O01", DueDate: date},
		{Prefix: WorkPrefixGeneral, RefID: "w", Status: WBTaskWaiting, OrgID: "O01", DueDate: date},
		{Prefix: WorkPrefixGeneral, RefID: "late", Status: WBTaskWaiting, OrgID: "O01", DueDate: "2026-11-01"},
		{Prefix: WorkPrefixGeneral, RefID: "nodue", Status: WBTaskWaiting, OrgID: "O01"},
		{Prefix: WorkPrefixGeneral, RefID: "d", Status: WBTaskComplete, OrgID: "O01"},
	}
	c := ClassifyExecToday(items, date)
	if c.Total != c.Done+c.InProgress+c.NotStarted+c.Delayed {
		t.Fatalf("합이 Total 이 아님 %+v", c)
	}
	if c.Done != 1 || c.InProgress != 1 || c.Delayed != 1 || c.NotStarted != 2 {
		t.Fatalf("%+v", c)
	}
	if c.DonePct != 20 {
		t.Fatalf("완료율 %d", c.DonePct)
	}
	z := ClassifyExecToday(nil, date)
	if z.Total != 0 || z.DonePct != 0 {
		t.Fatalf("0건 %+v", z)
	}
	if ExecBarPct(0, 0) != 0 || ExecBarPct(1, 0) != 0 {
		t.Fatal("0건 막대")
	}
}

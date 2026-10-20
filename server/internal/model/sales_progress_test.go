package model

import "testing"

func TestBuildSalesStageProgressContactByMethod(t *testing.T) {
	p := &SalesProject{Stage: SalesStage4Discover}
	got := BuildSalesStageProgress(p, []SalesActivity{
		{ActivityType: SalesActTypeOther, ActivityMethod: SalesActMethodVisit, ActivityDate: "2026-10-01"},
		{ActivityType: SalesActTypeRequirement, ActivityDate: "2026-10-01"},
		{ActivityType: SalesActTypeProposal, ActivityDate: "2026-10-01"},
	}, nil, "2026-10-02")
	if len(got.Discover) != 2 || !got.Discover[1].Done {
		t.Fatalf("담당자 접촉 %+v", got.Discover)
	}
	if len(got.Propose) < 1 || !got.Propose[0].Done || got.Propose[0].Code != "req" {
		t.Fatalf("요구사항이 제안으로 안 옮김 %+v", got.Propose)
	}
	if len(got.Bid) != 1 || !got.Bid[0].Done || got.Bid[0].Label != "제안서 제출" {
		t.Fatalf("입찰 제안서 %+v", got.Bid)
	}
}

package model

import "testing"

func TestFormatSalesPeriod(t *testing.T) {
	if got := FormatSalesPeriod("2027-03", SalesPrecisionMonth); got != "2027-03" {
		t.Fatalf("month: %q", got)
	}
	if got := FormatSalesPeriod("2027-03", SalesPrecisionQuarter); got != "2027년 1분기" {
		t.Fatalf("quarter: %q", got)
	}
	if got := FormatSalesPeriod("2027-08", SalesPrecisionHalf); got != "2027년 하반기" {
		t.Fatalf("half: %q", got)
	}
	if got := FormatSalesPeriod("2027-01", SalesPrecisionYear); got != "2027년" {
		t.Fatalf("year: %q", got)
	}
	if got := FormatSalesPeriod("", SalesPrecisionMonth); got != "" {
		t.Fatalf("empty: %q", got)
	}
}

func TestSalesProjectConfirmationAndDisplay(t *testing.T) {
	p := &SalesProject{Name: "가칭 사업", IsTentativeName: true, Stage: SalesStageSubmit, Probability: 50}
	if n, total := p.ConfirmedCount(); n != 0 || total != 4 {
		t.Fatalf("확정도=%d/%d", n, total)
	}
	if got := p.DisplayStage(&SalesStageDef{Code: SalesStageSubmit, Label: "제안서 제출", Probability: 50}); got != "제안서 제출 · 50%" {
		t.Fatalf("수주 전 표시: %q", got)
	}
	p.HasOverride, p.OverrideValue = true, 40
	if got := p.DisplayStage(&SalesStageDef{Code: SalesStageSubmit, Label: "제안서 제출"}); got != "제안서 제출 · 40%" {
		t.Fatalf("수동 확도: %q", got)
	}
	won := &SalesProject{Stage: SalesStageWon, Probability: 90, Name: "확정사업"}
	if got := won.DisplayStage(&SalesStageDef{Code: SalesStageWon, Label: "수주확정", Probability: 90}); got != "수주확정" {
		t.Fatalf("수주 후 표시: %q", got)
	}
	if err := p.RequireWonConfirmation(); err == nil {
		t.Fatal("미확정인데 수주확정이 통과했다")
	}
	p.IsTentativeName = false
	p.CustomerConfirmed, p.ExpectedYMConfirmed, p.ExpectedAmountConfirmed = true, true, true
	if err := p.RequireWonConfirmation(); err != nil {
		t.Fatal(err)
	}
}

func TestLoadSalesStagesReadsProbabilityFromCodes(t *testing.T) {
	stages := LoadSalesStages(
		[]Code{
			{CodeValue: SalesStageProposal, CodeName: "제안 진행", SortOrder: 3},
			{CodeValue: SalesStageSubmit, CodeName: "제안서 제출", SortOrder: 6},
		},
		[]Code{
			{CodeValue: SalesStageProposal, CodeName: "18"},
			{CodeValue: SalesStageSubmit, CodeName: "50"},
		},
	)
	d := FindSalesStage(stages, SalesStageProposal)
	if d == nil || d.Probability != 18 || d.Label != "제안 진행" {
		t.Fatalf("codes 확도가 반영되지 않음: %+v", d)
	}
}

func TestIsSalesStageBackward(t *testing.T) {
	lead := &SalesStageDef{Code: SalesStageLead, SortOrder: 1}
	submit := &SalesStageDef{Code: SalesStageSubmit, SortOrder: 6}
	lost := &SalesStageDef{Code: SalesStageLost, SortOrder: 9}
	if !IsSalesStageBackward(submit, lead) {
		t.Fatal("뒤로 이동이 후퇴로 안 잡힘")
	}
	if IsSalesStageBackward(lead, submit) {
		t.Fatal("앞으로 이동이 후퇴로 잡힘")
	}
	if !IsSalesStageBackward(lead, lost) {
		t.Fatal("수주실패는 사유가 필요해야 한다")
	}
}

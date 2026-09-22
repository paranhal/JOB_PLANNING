package model

import (
	"strings"
	"testing"
)

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

func TestSalesPeriodFirstYMAndBannerTitle(t *testing.T) {
	if got := SalesPeriodFirstYM("2026-11", SalesPrecisionQuarter); got != "2026-10" {
		t.Fatalf("Q4 first: %q", got)
	}
	if got := SalesPeriodFirstYM("2026-08", SalesPrecisionHalf); got != "2026-07" {
		t.Fatalf("H2 first: %q", got)
	}
	if got := SalesPeriodFirstYM("2026-06", SalesPrecisionYear); got != "2026-01" {
		t.Fatalf("year first: %q", got)
	}
	if got := SalesPeriodFirstYM("2026-09", SalesPrecisionMonth); got != "2026-09" {
		t.Fatalf("month: %q", got)
	}
	if got := SalesPeriodBannerNote("2026-10", SalesPrecisionQuarter); got != "4분기 중" {
		t.Fatalf("note: %q", got)
	}
	if got := FormatSalesMonthBannerTitle("2026-09", "부여군도서관 제안", SalesPrecisionMonth, false); got != "9월 · 부여군도서관 제안 (월 미정)" {
		t.Fatalf("month title: %q", got)
	}
	if got := FormatSalesMonthBannerTitle("2026-10", "부여군도서관 제안", SalesPrecisionQuarter, false); got != "10월 · 부여군도서관 제안 (4분기 중)" {
		t.Fatalf("quarter title: %q", got)
	}
	b, ok := SalesMonthBannerFromProject(SalesProject{
		SalesID: "SP-1", Name: "부여군도서관 제안", ExpectedYM: "2026-09", ExpectedPrecision: SalesPrecisionMonth,
	})
	if !ok || b.Href != "/sales/SP-1" || b.PlaceYM != "2026-09" {
		t.Fatalf("project banner: %+v ok=%v", b, ok)
	}
	if IsSalesMonthOnly("2026-09-15") || !IsSalesMonthOnly("2026-09") {
		t.Fatal("month-only 판정")
	}
	if err := RequireSalesDateOrYM("2026-09"); err != nil {
		t.Fatalf("YM 허용: %v", err)
	}
}

func TestSalesProjectConfirmationAndDisplay(t *testing.T) {
	p := &SalesProject{Name: "가칭 사업", IsTentativeName: true, Stage: SalesStageProposal, Probability: 40}
	if n, total := p.ConfirmedCount(); n != 0 || total != 4 {
		t.Fatalf("확정도=%d/%d", n, total)
	}
	if got := p.DisplayStage(&SalesStageDef{Code: SalesStageProposal, Label: "견적", Probability: 40}); got != "견적 · 40%" {
		t.Fatalf("수주 전 표시: %q", got)
	}
	p.HasOverride, p.OverrideValue = true, 35
	if got := p.DisplayStage(&SalesStageDef{Code: SalesStageProposal, Label: "견적"}); got != "견적 · 35%" {
		t.Fatalf("수동 확도: %q", got)
	}
	won := &SalesProject{Stage: SalesStageWon, Probability: 100, Name: "확정사업"}
	if got := won.DisplayStage(&SalesStageDef{Code: SalesStageWon, Label: "수주", Probability: 100}); got != "수주" {
		t.Fatalf("수주 후 표시: %q", got)
	}
	lost := &SalesProject{Stage: SalesStageLost, LostReason: "예산 삭감"}
	if got := lost.DisplayStage(&SalesStageDef{Code: SalesStageLost, Label: "실주"}); got != "실주 · 예산 삭감" {
		t.Fatalf("실주 표시: %q", got)
	}
	sup := &SalesProject{DealType: SalesDealSupply, Stage: SalesStageQuoted, ExpectedAmount: 1_200_000}
	if got := sup.DisplayStage(&SalesStageDef{Code: SalesStageQuoted, Label: "견적 제출"}); got != "견적 제출 · 1,200,000원" {
		t.Fatalf("단품 표시: %q", got)
	}
	if strings.Contains(sup.DisplayStage(&SalesStageDef{Code: SalesStageQuoted, Label: "견적 제출"}), "%") {
		t.Fatal("단품에 확도가 붙었다")
	}
	if err := p.RequireWonConfirmation(); err == nil {
		t.Fatal("미확정인데 수주가 통과했다")
	}
	p.IsTentativeName = false
	p.CustomerConfirmed, p.ExpectedYMConfirmed, p.ExpectedAmountConfirmed = true, true, true
	if err := p.RequireWonConfirmation(); err != nil {
		t.Fatal(err)
	}
}

func TestSalesProposalProgressFromActivities(t *testing.T) {
	p := SalesProposalProgressFrom([]SalesActivity{
		{ActivityType: SalesActTypeQuote},
		{ActivityType: SalesActTypeRFP, Title: "[이관] 제안서제출(RFP)"},
	})
	if !p.Quote || !p.RFP || p.Proposal {
		t.Fatalf("%+v", p)
	}
	if p.Mark(true) != "✔" || p.Mark(false) != "○" {
		t.Fatal("진척 기호")
	}
}

func TestLoadSalesStagesReadsProbabilityFromCodes(t *testing.T) {
	stages := LoadSalesStages(
		[]Code{
			{CodeValue: SalesStageProposal, CodeName: "견적", SortOrder: 3},
			{CodeValue: SalesStageSubmit, CodeName: "제안서 제출", SortOrder: 6},
		},
		[]Code{
			{CodeValue: SalesStageProposal, CodeName: "18"},
			{CodeValue: SalesStageSubmit, CodeName: "50"},
		},
	)
	d := FindSalesStage(stages, SalesStageProposal)
	if d == nil || d.Probability != 18 || d.Label != "견적" {
		t.Fatalf("codes 확도가 반영되지 않음: %+v", d)
	}
}

func TestIsSalesStageBackward(t *testing.T) {
	contact := &SalesStageDef{Code: SalesStageContact, SortOrder: 1}
	lead := &SalesStageDef{Code: SalesStageLead, SortOrder: 2}
	lost := &SalesStageDef{Code: SalesStageLost, SortOrder: 6}
	if !IsSalesStageBackward(lead, contact) {
		t.Fatal("뒤로 이동이 후퇴로 안 잡힘")
	}
	if IsSalesStageBackward(contact, lead) {
		t.Fatal("앞으로 이동이 후퇴로 잡힘")
	}
	if !IsSalesStageBackward(lead, lost) {
		t.Fatal("실주는 사유가 필요해야 한다")
	}
}

func TestMergeSalesTimelineMixesChangesAndSkipsStageDup(t *testing.T) {
	acts := []SalesActivity{{
		ActivityDate: "2026-08-11", StartTime: "10:00", Title: "방문미팅", TypeLabel: "방문미팅",
	}}
	hist := []SalesStageHistory{{
		ChangedAt: "2026-08-14 09:00:00", FromLabel: "견적요청", ToLabel: "RFP제안",
	}}
	changes := []SalesChange{
		{FieldKey: SalesChangeAmount, OldValue: "30,000,000원", NewValue: "180,000,000원", ChangedAt: "2026-08-14 11:00:00"},
		{FieldKey: SalesChangeStage, OldValue: "견적요청", NewValue: "RFP제안", ChangedAt: "2026-08-14 09:00:00"},
	}
	ev := MergeSalesTimeline(acts, hist, changes)
	if len(ev) != 3 {
		t.Fatalf("섞인 건수=%d want 3 (단계 변경 중복 제외)", len(ev))
	}
	kinds := ev[0].Kind + "," + ev[1].Kind + "," + ev[2].Kind
	if !strings.Contains(kinds, SalesTimelineChange) || !strings.Contains(kinds, SalesTimelineStage) || !strings.Contains(kinds, SalesTimelineActivity) {
		t.Fatalf("한 타임라인에 안 섞였다: %s", kinds)
	}
	foundAmt := false
	for _, e := range ev {
		if e.Kind == SalesTimelineChange && strings.Contains(e.Title, "30,000,000") && strings.Contains(e.Title, "180,000,000") {
			foundAmt = true
		}
		if e.Kind == SalesTimelineChange && e.FieldKey == SalesChangeStage {
			t.Fatal("단계 변경이 타임라인에 두 번 들어갔다")
		}
	}
	if !foundAmt {
		t.Fatal("금액 변경이 타임라인에 없다")
	}
}

func TestCanPromoteSalesAndWorkProjectFromSales(t *testing.T) {
	lead := &SalesProject{Stage: SalesStageLead, Status: SalesStatusActive}
	if CanPromoteSales(lead) {
		t.Fatal("lead 인데 승격이 열렸다")
	}
	won := &SalesProject{Stage: SalesStage4Closed, CloseReason: SalesCloseContracted, Status: SalesStatusActive}
	if !CanPromoteSales(won) {
		t.Fatal("계약 종료인데 승격이 안 열린다")
	}
	done := &SalesProject{Stage: SalesStage4Closed, CloseReason: SalesCloseContracted, Status: SalesStatusPromoted}
	if CanPromoteSales(done) {
		t.Fatal("이미 승격완료인데 버튼이 열렸다")
	}
	ok := &SalesProject{
		SalesID: "SL-001", Name: "2027년 충남교육청", Stage: SalesStage4Closed, CloseReason: SalesCloseContracted,
		Status: SalesStatusContracted, CustomerID: "C041-26-001",
		ExpectedYM: "2027-03", ExpectedAmount: 30_000_000, ContractAmount: 30_000_000, Notes: "재계약",
		ContractedAt: "2027-03-01",
	}
	if !CanPromoteSales(ok) {
		t.Fatal("계약인데 승격이 안 열린다")
	}
	supply := &SalesProject{Stage: SalesStage4Closed, CloseReason: SalesCloseContracted, Status: SalesStatusContracted, DealType: SalesDealSupply}
	if !CanPromoteSales(supply) {
		t.Fatal("단품 계약도 승격해야 한다")
	}
	wp := WorkProjectFromSales(ok)
	if wp.Name != "2027년 충남교육청" || wp.CustomerID != "C041-26-001" || wp.OrderingPartyID != "C041-26-001" {
		t.Fatalf("사업명·고객·발주처가 안 옮겨졌다: %+v", wp)
	}
	if wp.ContractAmount != 30_000_000 {
		t.Fatalf("계약금액 초기값=%d", wp.ContractAmount)
	}
	if !strings.Contains(wp.Notes, "30,000,000") || !strings.Contains(wp.Notes, "재계약") {
		t.Fatalf("금액이 비고로 안 옮겨졌다: %q", wp.Notes)
	}
	if wp.ContractType != "" || wp.BillingType != "" {
		t.Fatal("계약 유형은 승격 화면에서 처음 입력해야 한다")
	}
	if wp.StartDate != "2027-03-01" {
		t.Fatalf("시작일=%q", wp.StartDate)
	}
}

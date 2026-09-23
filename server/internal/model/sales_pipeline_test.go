package model

import (
	"testing"
	"time"
)

func TestBuildSalesPipelineMetrics(t *testing.T) {
	stages := fallbackSalesStages()
	now := time.Date(2026, 8, 21, 0, 0, 0, 0, time.Local)
	projects := []SalesProject{
		{SalesID: "a", Name: "리드", Stage: SalesStage4Discover, Probability: 10, ExpectedAmount: 30_000_000, ExpectedYM: "2026-09", CreatedAt: "2026-08-01 00:00:00"},
		{SalesID: "b", Name: "계약", Stage: SalesStage4Closed, CloseReason: SalesCloseContracted, Probability: 100, ExpectedAmount: 180_000_000, ContractAmount: 180_000_000, ExpectedYM: "2026-10", WonAt: "2026-07-11", CreatedAt: "2026-07-01 00:00:00"},
		{SalesID: "c", Name: "실패", Stage: SalesStage4Closed, CloseReason: SalesCloseLost, Probability: 0, ExpectedAmount: 50_000_000, ExpectedYM: "2026-09", CreatedAt: "2026-07-01 00:00:00"},
	}
	hist := []SalesStageHistory{
		{HistoryID: "1", SalesID: "a", ToStage: SalesStage4Discover, ChangedAt: "2026-08-01 00:00:00"},
		{HistoryID: "2", SalesID: "b", ToStage: SalesStage4Discover, ChangedAt: "2026-07-01 00:00:00"},
		{HistoryID: "3", SalesID: "b", ToStage: SalesStage4Closed, ChangedAt: "2026-07-11 00:00:00"},
		{HistoryID: "4", SalesID: "c", ToStage: SalesStage4Closed, ChangedAt: "2026-07-01 00:00:00"},
	}
	acts := []SalesActivity{
		{OurMembers: "최혜영, 양기헌"},
		{OurMembers: "최혜영"},
	}
	p := BuildSalesPipeline(projects, stages, hist, acts, now, []string{"최혜영", "태자운"})
	if p.TotalCount != 3 {
		t.Fatalf("건수=%d", p.TotalCount)
	}
	if p.WeightedTotal != 3_000_000+180_000_000 {
		t.Fatalf("가중=%d", p.WeightedTotal)
	}
	if p.WinRateLabel != "50%" {
		t.Fatalf("수주율=%s want 50%%", p.WinRateLabel)
	}
	empty := BuildSalesPipeline(nil, stages, nil, nil, now, nil)
	if empty.WinRateLabel != "—" {
		t.Fatalf("분모 0 수주율=%s", empty.WinRateLabel)
	}
	var disc, closed SalesPipelineStage
	for _, s := range p.Stages {
		if s.Code == SalesStage4Discover {
			disc = s
		}
		if s.Code == SalesStage4Closed {
			closed = s
		}
	}
	if disc.Count != 1 || disc.Amount != 30_000_000 {
		t.Fatalf("발굴 단계: %+v", disc)
	}
	if closed.DwellLabel == "—" || closed.DwellSamples < 1 {
		t.Fatalf("종료 체류일이 없다: %+v", closed)
	}
	if len(p.Months) < 2 {
		t.Fatalf("예정월 분포=%d", len(p.Months))
	}
	var hy, tae int
	for _, person := range p.People {
		if person.Name == "최혜영" {
			hy = person.Count
		}
		if person.Name == "태자운" {
			tae = person.Count
		}
	}
	if hy != 2 {
		t.Fatalf("최혜영 활동=%d", hy)
	}
	if tae != 0 {
		t.Fatalf("빈 담당자 열이 빠졌다 태자운=%d", tae)
	}
}

func TestBuildSalesTimelineAxisAndKanbanStages(t *testing.T) {
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	axis := BuildSalesTimelineAxis([]SalesProject{
		{SalesID: "a", Name: "A", ExpectedYM: "2026-09", Stage: SalesStage4Discover, Probability: 10},
		{SalesID: "b", Name: "B", ExpectedYM: "", Stage: SalesStage4Propose, Probability: 20},
	}, fallbackSalesStages(), now)
	if len(axis.Months) < 2 {
		t.Fatalf("월 축=%v", axis.Months)
	}
	if axis.Months[0] != "2026-08" || axis.Months[len(axis.Months)-1] != "2026-09" {
		t.Fatalf("월 축=%v", axis.Months)
	}
	if !axis.Rows[1].Unscheduled {
		t.Fatal("시기 없는 사업이 미정으로 안 갔다")
	}
	proc, lost := SalesKanbanStages(fallbackSalesStages())
	if len(proc) != 4 {
		t.Fatalf("열=%d want 4", len(proc))
	}
	if lost != nil {
		t.Fatalf("실주 열을 빼지 않는다: %+v", lost)
	}
	if proc[0].Code != SalesStage4Discover || proc[1].Code != SalesStage4Propose {
		t.Fatalf("순서=%s,%s", proc[0].Code, proc[1].Code)
	}
}

func TestFormatSalesMoneyShort(t *testing.T) {
	if got := FormatSalesMoneyShort(1_240_000_000); got != "12.4억" {
		t.Fatalf("12.4억 got=%s", got)
	}
	if got := FormatSalesMoneyShort(90_000_000); got != "0.9억" {
		t.Fatalf("0.9억 got=%s", got)
	}
	if got := FormatSalesMoneyShort(300_000_000); got != "3억" {
		t.Fatalf("3억 got=%s", got)
	}
}

func TestFormatSalesRateDash(t *testing.T) {
	if FormatSalesRate(0, 0) != "—" {
		t.Fatal("분모 0은 —")
	}
	if FormatSalesDwell(1.5, 0) != "—" {
		t.Fatal("체류 표본 0은 —")
	}
}

func TestSalesPipelineAmountQuotes(t *testing.T) {
	p := &SalesProject{Stage: SalesStage4Propose, ExpectedAmount: 1_000_000}
	quotes := []SalesQuote{
		{QuoteNo: "A", Rev: 0, Total: 100, Status: QuoteStatusSent},
		{QuoteNo: "A", Rev: 1, Total: 200, Status: QuoteStatusSent},
		{QuoteNo: "A", Rev: 2, Total: 300, Status: QuoteStatusSent},
		{QuoteNo: "B", Rev: 0, Total: 50, Status: QuoteStatusSent},
		{QuoteNo: "C", Rev: 0, Total: 999, Status: QuoteStatusLost},
		{QuoteNo: "D", Rev: 0, Total: 1, Status: QuoteStatusExpired},
	}
	sum, n := ValidQuoteSum(quotes)
	if sum != 350 || n != 2 {
		t.Fatalf("sum=%d n=%d want 350, 2", sum, n)
	}
	amt, src := SalesPipelineAmount(p, quotes)
	if amt != 350 || src != "quote" {
		t.Fatalf("propose amt=%d src=%s", amt, src)
	}
	lost := &SalesProject{Stage: SalesStage4Closed, CloseReason: SalesCloseLost, ExpectedAmount: 9_000_000, ContractAmount: 9_000_000}
	amt, src = SalesPipelineAmount(lost, quotes)
	if amt != 0 || src != "" {
		t.Fatalf("lost amt=%d src=%s", amt, src)
	}
	drop := &SalesProject{Stage: SalesStage4Closed, CloseReason: SalesCloseDropped, ExpectedAmount: 8_000_000}
	amt, _ = SalesPipelineAmount(drop, quotes)
	if amt != 0 {
		t.Fatalf("drop amt=%d", amt)
	}
}

package model

import (
	"testing"
	"time"
)

func TestSalesProbabilityTable(t *testing.T) {
	disc := &SalesProject{Stage: SalesStage4Discover}
	if SalesProbability(disc) != 10 {
		t.Fatalf("discover=%d", SalesProbability(disc))
	}
	pr := &SalesProject{Stage: SalesStage4Propose}
	if SalesProbability(pr) != 20 {
		t.Fatalf("propose no rfp=%d", SalesProbability(pr))
	}
	pr.RFPReceivedAt = "2026-09-01"
	if SalesProbability(pr) != 50 {
		t.Fatalf("propose rfp default=%d", SalesProbability(pr))
	}
	pr.WinProb = IntPtr(70)
	if SalesProbability(pr) != 70 {
		t.Fatalf("propose win_prob=%d", SalesProbability(pr))
	}
	bid := &SalesProject{Stage: SalesStage4Bid, BidStatus: SalesBidPending, ProbabilityFinal: IntPtr(60)}
	if SalesProbability(bid) != 60 {
		t.Fatalf("pending=%d", SalesProbability(bid))
	}
	bid.BidStatus = SalesBidWon
	if SalesProbability(bid) != 100 {
		t.Fatalf("won=%d", SalesProbability(bid))
	}
	closed := &SalesProject{Stage: SalesStage4Closed, CloseReason: SalesCloseContracted}
	if SalesProbability(closed) != 100 {
		t.Fatalf("contracted=%d", SalesProbability(closed))
	}
	closed.CloseReason = SalesCloseLost
	if SalesProbability(closed) != 0 {
		t.Fatalf("lost=%d", SalesProbability(closed))
	}
}

func TestSalesWinSampleSkipsDropped(t *testing.T) {
	won, lost := SalesWinSample([]SalesProject{
		{WonAt: "2026-01-01"}, {WonAt: "2026-01-02"}, {WonAt: "2026-01-03"},
		{CloseReason: SalesCloseLost},
		{CloseReason: SalesCloseDropped, Status: SalesStatusDropped},
		{CloseReason: SalesCloseDropped, WonAt: ""},
	})
	if won != 3 || lost != 1 {
		t.Fatalf("won=%d lost=%d", won, lost)
	}
	if FormatSalesRate(won, won+lost) != "75%" {
		t.Fatalf("rate=%s", FormatSalesRate(won, won+lost))
	}
}

func TestCanPromoteSalesClosedContracted(t *testing.T) {
	p := &SalesProject{Stage: SalesStage4Closed, CloseReason: SalesCloseContracted, DealType: SalesDealSupply, Status: SalesStatusContracted}
	if !CanPromoteSales(p) {
		t.Fatal("단품 계약도 승격해야 한다")
	}
	p.Status = SalesStatusPromoted
	if CanPromoteSales(p) {
		t.Fatal("이미 승격")
	}
}

func TestSalesContractUnsignedUsesBidStatus(t *testing.T) {
	p := &SalesProject{Stage: SalesStage4Bid, BidStatus: SalesBidWon, WonAt: "2026-01-01"}
	if !SalesContractUnsigned(p, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("30일 지났는데 미계약이 아니다")
	}
	p.Stage = SalesStage4Closed
	p.CloseReason = SalesCloseContracted
	if SalesContractUnsigned(p, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("종료 건을 미계약으로 봤다")
	}
}

func TestEffectiveProbabilityUsesSalesProbability(t *testing.T) {
	p := &SalesProject{Stage: SalesStage4Discover, Probability: 99, HasOverride: true, OverrideValue: 40}
	if p.EffectiveProbability() != 10 {
		t.Fatalf("got %d", p.EffectiveProbability())
	}
}

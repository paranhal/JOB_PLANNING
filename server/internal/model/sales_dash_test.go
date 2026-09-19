package model

import "testing"

func TestCountOpenAndMonthWonAndDelayed(t *testing.T) {
	items := []SalesProject{
		{SalesID: "a", Stage: SalesStageLead, ExpectedAmount: 10},
		{SalesID: "b", Stage: SalesStageWon, ExpectedAmount: 50, WonAt: "2026-09-03"},
		{SalesID: "c", Stage: SalesStageLost, ExpectedAmount: 9},
	}
	if n := CountOpenSales(items); n != 1 {
		t.Fatalf("open=%d", n)
	}
	if got := SumMonthWon(items, "2026-09"); got != 50 {
		t.Fatalf("month won=%d", got)
	}
	next := map[string]SalesActivity{
		"a": {SalesID: "a", NextAction: "방문", NextActionDate: "2026-09-01"},
		"b": {SalesID: "b", NextAction: "월만", NextActionDate: "2026-08"},
	}
	if n := CountSalesDelayed(next, "2026-09-10"); n != 1 {
		t.Fatalf("delayed=%d", n)
	}
	if SalesDnLabel("2026-09-12", "2026-09-10") != "D-2" {
		t.Fatalf("dn=%s", SalesDnLabel("2026-09-12", "2026-09-10"))
	}
}

func TestOrderScheduleAndContractLife(t *testing.T) {
	o := SalesOrder{Status: OrderStatusOpen, DueDate: "2026-09-01"}
	if OrderScheduleLabel(o, "2026-09-10") != "지연" {
		t.Fatal("납기 지난 미납품은 지연")
	}
	o.Status = OrderStatusDelivered
	if OrderScheduleLabel(o, "2026-09-10") != "완료" {
		t.Fatal("납품완료는 완료")
	}
	p := WorkProject{EndDate: "2026-09-20"}
	if p.ContractLifeStatus("2026-09-10") != "만료임박" {
		t.Fatalf("life=%s", p.ContractLifeStatus("2026-09-10"))
	}
}

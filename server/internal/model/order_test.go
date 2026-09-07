package model

import "testing"

func TestOrderLinesCopiedFromQuoteThenDiffFlag(t *testing.T) {
	q := &SalesQuote{
		VATMode: QuoteVATExcluded,
		Lines: []SalesQuoteLine{
			{LineID: "QL-1", Name: "감열지", Qty: 2, Unit: "EA", UnitPrice: 150000},
			{LineID: "QL-2", Name: "설치", Qty: 1, Unit: "식", UnitPrice: 500000},
		},
	}
	lines := OrderLinesFromQuote(q)
	if len(lines) != 2 {
		t.Fatalf("라인=%d", len(lines))
	}
	if lines[0].Qty != 2 || lines[0].UnitPrice != 150000 || lines[0].QuoteQty != 2 || lines[0].QuoteUnitPrice != 150000 {
		t.Fatalf("복사 불일치 %+v", lines[0])
	}
	if lines[0].QuoteLineID != "QL-1" {
		t.Fatal("견적 라인 id가 안 붙었다")
	}
	o := &SalesOrder{VATMode: QuoteVATExcluded, Lines: lines}
	ApplyOrderTotals(o)
	if o.DiffersFromQuote() {
		t.Fatal("그대로인데 견적과 다름이 떴다")
	}
	o.Lines[0].Qty = 3
	ApplyOrderTotals(o)
	if !o.DiffersFromQuote() || !o.Lines[0].DiffersFromQuote() {
		t.Fatal("수량 바꿨는데 견적과 다름이 없다")
	}
}

func TestOrderMarginSellMinusCost(t *testing.T) {
	o := &SalesOrder{
		Lines: []SalesOrderLine{
			{LineID: "L1", Qty: 1, UnitPrice: 1_000_000, Amount: 1_000_000},
		},
		Purchases: []SalesPurchase{
			{LineID: "L1", Qty: 1, CostPrice: 700_000},
		},
	}
	o.ApplyCosts()
	if o.CostTotal != 700_000 || o.Margin != 300_000 {
		t.Fatalf("원가=%d 마진=%d", o.CostTotal, o.Margin)
	}
	if o.Lines[0].Margin != 300_000 {
		t.Fatalf("라인 마진=%d", o.Lines[0].Margin)
	}
}

func TestOrderStripCostClearsValues(t *testing.T) {
	o := &SalesOrder{
		CostTotal: 700_000, Margin: 300_000,
		Lines:     []SalesOrderLine{{Cost: 700_000, Margin: 300_000}},
		Purchases: []SalesPurchase{{CostPrice: 700_000}},
	}
	o.StripCost()
	if o.CostTotal != 0 || o.Margin != 0 || o.Lines[0].Cost != 0 || o.Purchases[0].CostPrice != 0 {
		t.Fatalf("스트립 실패 %+v", o)
	}
}

func TestCanSeeMarginAdminSalesOnly(t *testing.T) {
	if !CanSeeMargin(RoleAdmin) || !CanSeeMargin(RoleSales) {
		t.Fatal("관리자·영업이 못 본다")
	}
	if CanSeeMargin(RoleTech) || CanSeeMargin(RoleOffice) || CanSeeMargin(RoleObserver) {
		t.Fatal("기술·행정·옵저버에게 마진이 열린다")
	}
}

func TestOrderKanbanFourColumns(t *testing.T) {
	v := FillOrderKanban([]SalesOrder{{OrderID: "O-1", Status: OrderStatusOpen, Title: "a"}})
	if len(v.Columns) != 4 {
		t.Fatalf("열=%d", len(v.Columns))
	}
	if OrderKanbanBucket(OrderStatusClosed) != OrderStatusDelivered {
		t.Fatal("종료가 납품 완료 열로 안 간다")
	}
}

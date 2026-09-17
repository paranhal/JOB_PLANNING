package repository

import (
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestSupplyDealDoesNotChangeExecVisitCompleteAndBudgetOutOfConversion(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "supply_kpi.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c1','세종시교육청','세종시교육청',1)`); err != nil {
		t.Fatal(err)
	}
	setMetricsPolicy(t, db, "2026-08-01", "as,maintenance")
	if _, err := db.Exec(`
		INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date,
			start_datetime, status, assigned_to, complete_datetime, data_origin)
		VALUES ('a1','R1','c1','2026-08-01','2026-08-03','2026-08-03','completed','양기헌','2026-08-05','app')`); err != nil {
		t.Fatal(err)
	}

	stats := NewStatsRepo(db)
	anchor := time.Date(2026, 8, 7, 0, 0, 0, 0, time.Local)
	snap := func() string {
		t.Helper()
		cols := BuildStatsPeriodColumns(model.StatsViewMonth, anchor)
		f := ParseMeetingFilter(model.StatsScopeTeam, "", "")
		if err := stats.FillPeriodOverview(cols, f); err != nil {
			t.Fatal(err)
		}
		kpi, err := stats.LoadStatsKPI(model.StatsViewMonth, cols, f)
		if err != nil {
			t.Fatal(err)
		}
		return formatSupplyKPI(kpi)
	}
	before := snap()

	sales := NewSalesRepo(db)
	p := &model.SalesProject{Name: "RFID 게이트 단품", DealType: model.SalesDealSupply, ExpectedAmount: 3_000_000}
	if err := sales.Create(p); err != nil {
		t.Fatal(err)
	}
	quotes := NewQuoteRepo(db)
	deal := &model.SalesQuote{
		FormType: model.QuoteFormA, VATMode: model.QuoteVATExcluded, RoundRule: model.QuoteRoundNone,
		QuoteDate: time.Now().Format("2006-01-02"), Title: "게이트", OwnerName: "최혜영", OwnerPhone: "010",
		CustomerID: "c1", RecipientName: "세종시교육청", SalesID: p.SalesID, Purpose: model.QuotePurposeDeal,
		Status: model.QuoteStatusSent,
		Lines:  []model.SalesQuoteLine{{Name: "RFID 게이트", Qty: 1, Unit: "EA", UnitPrice: 1_000_000}},
	}
	if err := quotes.Create(deal); err != nil {
		t.Fatal(err)
	}
	if err := quotes.SetStatus(deal.QuoteID, model.QuoteStatusWon); err != nil {
		t.Fatal(err)
	}
	budget := &model.SalesQuote{
		FormType: model.QuoteFormA, VATMode: model.QuoteVATExcluded, RoundRule: model.QuoteRoundNone,
		QuoteDate: time.Now().Format("2006-01-02"), Title: "예산", OwnerName: "최혜영", OwnerPhone: "010",
		CustomerID: "c1", RecipientName: "세종시교육청", Purpose: model.QuotePurposeBudget, BudgetYear: 2027,
		Status: model.QuoteStatusSent,
		Lines:  []model.SalesQuoteLine{{Name: "출입통제", Qty: 1, Unit: "식", UnitPrice: 37_950_000}},
	}
	if err := quotes.Create(budget); err != nil {
		t.Fatal(err)
	}
	orders := NewOrderRepo(db)
	gotDeal, _ := quotes.Get(deal.QuoteID)
	o, err := orders.CreateFromQuote(gotDeal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := orders.AddDelivery(&model.SalesDelivery{
		OrderID: o.OrderID, LineID: o.Lines[0].LineID, Qty: 1, DeliveryDate: time.Now().Format("2006-01-02"),
	}, []string{"SN-1"}, "로비"); err != nil {
		t.Fatal(err)
	}

	after := snap()
	if after != before {
		t.Fatalf("단품 거래가 §4 지표를 바꿨다\n before %s\n after  %s", before, after)
	}

	m, err := sales.LoadSupplyMetrics(time.Now(), true)
	if err != nil {
		t.Fatal(err)
	}
	if m.ConversionSubmitted != 1 || m.ConversionWon != 1 {
		t.Fatalf("예산용이 전환율에 들어갔다 won=%d sent=%d", m.ConversionWon, m.ConversionSubmitted)
	}
}

func formatSupplyKPI(kpi model.StatsKPICard) string {
	return "visit=" + strconv.FormatFloat(kpi.VisitAvgDays, 'f', -1, 64) +
		" complete=" + strconv.FormatFloat(kpi.CompleteAvgDays, 'f', -1, 64) +
		" nVisit=" + strconv.Itoa(kpi.VisitSample) +
		" nComplete=" + strconv.Itoa(kpi.CompleteSample)
}

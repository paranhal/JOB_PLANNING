package model

import (
	"fmt"
	"strings"
	"time"
)

const (
	OrderStatusOpen       = "open"
	OrderStatusPurchasing = "purchasing"
	OrderStatusReady      = "ready"
	OrderStatusDelivered  = "delivered"
	OrderStatusClosed     = "closed"

	OrderNoPrefix = "VI-수주-"

	PurchaseStatusOrdered = "ordered"
	PurchaseStatusArrived = "arrived"
)

var ErrOrderQuoteRequired = fmt.Errorf("견적이 필요합니다")
var ErrOrderNoLines = fmt.Errorf("넘길 라인이 없습니다")

type SalesOrder struct {
	OrderID       string
	SalesID       string
	QuoteID       string
	OrderNo       string
	OrderDate     string
	PONo          string
	PODate        string
	CustomerID    string
	RecipientName string
	Title         string
	DueDate       string
	VATMode       string
	Amount        int
	VAT           int
	Total         int
	Status        string
	BillingStatus string
	InvoiceNo     string
	InvoicedAt    string
	PaidAt        string
	PaidAmount    int
	Remarks       string
	CreatedAt     string
	UpdatedAt     string
	Lines         []SalesOrderLine
	Purchases     []SalesPurchase
	Deliveries    []SalesDelivery
	Assets        []Asset
	CostTotal     int
	Margin        int
}

type SalesOrderLine struct {
	LineID         string
	OrderID        string
	QuoteLineID    string
	Seq            int
	ItemID         string
	GroupLabel     string
	Name           string
	Spec           string
	Qty            float64
	Unit           string
	UnitPrice      int
	Amount         int
	MMRate         float64
	DiscountRate   float64
	Note           string
	QuoteQty       float64
	QuoteUnitPrice int
	Cost           int
	Margin         int
}

type SalesPurchase struct {
	PurchaseID   string
	OrderID      string
	LineID       string
	SupplierName string
	SupplierID   string
	PurchaseDate string
	Qty          float64
	CostPrice    int
	EtaDate      string
	ArrivedDate  string
	Status       string
	Note         string
}

func (ln SalesOrderLine) DiffersFromQuote() bool {
	return ln.Qty != ln.QuoteQty || ln.UnitPrice != ln.QuoteUnitPrice
}

func (o *SalesOrder) DiffersFromQuote() bool {
	if o == nil {
		return false
	}
	for _, ln := range o.Lines {
		if ln.DiffersFromQuote() {
			return true
		}
	}
	return false
}

func (ln SalesOrderLine) CalcAmount() int {
	return LineAmount(SalesQuoteLine{
		Qty: ln.Qty, UnitPrice: ln.UnitPrice, MMRate: ln.MMRate, DiscountRate: ln.DiscountRate,
	})
}

func OrderLinesFromQuote(q *SalesQuote) []SalesOrderLine {
	if q == nil {
		return nil
	}
	out := make([]SalesOrderLine, 0, len(q.Lines))
	for i, src := range PrepareQuoteLines(q.Lines) {
		out = append(out, SalesOrderLine{
			QuoteLineID:    src.LineID,
			Seq:            i + 1,
			ItemID:         src.ItemID,
			GroupLabel:     src.GroupLabel,
			Name:           src.Name,
			Spec:           src.Spec,
			Qty:            src.Qty,
			Unit:           src.Unit,
			UnitPrice:      src.UnitPrice,
			Amount:         src.Amount,
			MMRate:         src.MMRate,
			DiscountRate:   src.DiscountRate,
			Note:           src.Note,
			QuoteQty:       src.Qty,
			QuoteUnitPrice: src.UnitPrice,
		})
	}
	return out
}

func ApplyOrderTotals(o *SalesOrder) {
	if o == nil {
		return
	}
	sum := 0
	for i := range o.Lines {
		o.Lines[i].Amount = o.Lines[i].CalcAmount()
		sum += o.Lines[i].Amount
	}
	tot := ComputeQuoteTotalsFromBase(sum, o.VATMode, QuoteRoundNone)
	o.Amount = tot.Supply
	o.VAT = tot.VAT
	o.Total = tot.Total
}

func (o *SalesOrder) ApplyCosts() {
	if o == nil {
		return
	}
	byLine := map[string]int{}
	cost := 0
	for i := range o.Purchases {
		p := &o.Purchases[i]
		amt := purchaseCostAmount(*p)
		cost += amt
		if p.LineID != "" {
			byLine[p.LineID] += amt
		}
	}
	o.CostTotal = cost
	sell := 0
	for i := range o.Lines {
		o.Lines[i].Cost = byLine[o.Lines[i].LineID]
		o.Lines[i].Margin = o.Lines[i].Amount - o.Lines[i].Cost
		sell += o.Lines[i].Amount
	}
	o.Margin = sell - cost
}

func purchaseCostAmount(p SalesPurchase) int {
	q := p.Qty
	if q <= 0 {
		q = 1
	}
	v := q * float64(p.CostPrice)
	if v <= 0 {
		return 0
	}
	return int(v)
}

func (o *SalesOrder) StripCost() {
	if o == nil {
		return
	}
	o.CostTotal = 0
	o.Margin = 0
	for i := range o.Lines {
		o.Lines[i].Cost = 0
		o.Lines[i].Margin = 0
	}
	for i := range o.Purchases {
		o.Purchases[i].CostPrice = 0
	}
}

func CanSeeMargin(role string) bool {
	r := NormalizeRole(role)
	return r == RoleAdmin || r == RoleSales
}

func NormalizeOrderStatus(s string) string {
	switch strings.TrimSpace(s) {
	case OrderStatusPurchasing, OrderStatusReady, OrderStatusDelivered, OrderStatusClosed:
		return strings.TrimSpace(s)
	default:
		return OrderStatusOpen
	}
}

func OrderStatusLabel(s string) string {
	switch NormalizeOrderStatus(s) {
	case OrderStatusPurchasing:
		return "발주"
	case OrderStatusReady:
		return "입고"
	case OrderStatusDelivered, OrderStatusClosed:
		return "납품 완료"
	default:
		return "수주"
	}
}

func OrderKanbanBucket(s string) string {
	switch NormalizeOrderStatus(s) {
	case OrderStatusPurchasing:
		return OrderStatusPurchasing
	case OrderStatusReady:
		return OrderStatusReady
	case OrderStatusDelivered, OrderStatusClosed:
		return OrderStatusDelivered
	default:
		return OrderStatusOpen
	}
}

func OrderStatusDefs() []KanbanColumnDef {
	return []KanbanColumnDef{
		{Key: OrderStatusOpen, Title: "수주", Border: "border-slate-200"},
		{Key: OrderStatusPurchasing, Title: "발주", Border: "border-sky-200"},
		{Key: OrderStatusReady, Title: "입고", Border: "border-amber-200"},
		{Key: OrderStatusDelivered, Title: "납품 완료", Border: "border-emerald-200"},
	}
}

func FillOrderKanban(items []SalesOrder) KanbanView {
	cols := emptyKanbanColumns(OrderStatusDefs())
	idx := indexKanbanColumns(cols)
	seen := map[string]bool{}
	for i := range items {
		o := items[i]
		id := strings.TrimSpace(o.OrderID)
		if id == "" || seen[id] {
			continue
		}
		b := OrderKanbanBucket(o.Status)
		j, ok := idx[b]
		if !ok {
			continue
		}
		seen[id] = true
		card := KanbanCard{
			ID:         id,
			RefID:      id,
			RefNumber:  o.OrderNo,
			Title:      o.Title,
			Href:       "/orders/" + id,
			EditHref:   "/orders/" + id + "/edit",
			OrgName:    o.RecipientName,
			Extra:      o.OrderNo + " · " + formatSalesAmount(o.Total) + "원",
			Bucket:     b,
			AmountDesc: o.Total,
		}
		if o.DiffersFromQuote() {
			card.DelayBadge = "견적과 다름"
			card.DelayClass = "bg-amber-50 text-amber-800"
		}
		cols[j].Items = append(cols[j].Items, card)
	}
	v := finishKanbanView(cols)
	for i := range v.Columns {
		v.Columns[i].CountUnit = "건"
	}
	return v
}

func FormatOrderNo(iso string, seq int) string {
	t, err := ParseQuoteDateISO(iso)
	if err != nil || seq < 1 {
		return ""
	}
	return fmt.Sprintf("%s%s-%03d", OrderNoPrefix, t.Format("20060102"), seq)
}

func OrderDateToday() string {
	return time.Now().Format("2006-01-02")
}

func NormalizePurchaseStatus(s string) string {
	if strings.TrimSpace(s) == PurchaseStatusArrived {
		return PurchaseStatusArrived
	}
	return PurchaseStatusOrdered
}

func PurchaseStatusLabel(s string) string {
	if NormalizePurchaseStatus(s) == PurchaseStatusArrived {
		return "입고"
	}
	return "발주"
}

func (o *SalesOrder) DisplayNo() string {
	if o == nil {
		return ""
	}
	return strings.TrimSpace(o.OrderNo)
}

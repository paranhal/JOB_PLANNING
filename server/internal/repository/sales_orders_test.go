package repository

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestOrderRepoCreateFromQuoteCopiesLinesThenDiffers(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "orders.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	quotes := NewQuoteRepo(db)
	q := &model.SalesQuote{
		SalesID:       mustSalesForQuote(t, db),
		FormType:      model.QuoteFormA,
		VATMode:       model.QuoteVATExcluded,
		RoundRule:     model.QuoteRoundNone,
		QuoteDate:     time.Now().Format("2006-01-02"),
		Title:         "감열지 수주",
		OwnerName:     "최혜영",
		OwnerPhone:    "010-1111-2222",
		RecipientName: "세종시교육청",
		Lines: []model.SalesQuoteLine{
			{Name: "감열지", Qty: 2, Unit: "EA", UnitPrice: 150000},
			{Name: "설치", Qty: 1, Unit: "식", UnitPrice: 500000},
		},
	}
	if err := quotes.Create(q); err != nil {
		t.Fatal(err)
	}

	orders := NewOrderRepo(db)
	o, err := orders.CreateFromQuote(q)
	if err != nil {
		t.Fatal(err)
	}
	got, err := orders.Get(o.OrderID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Lines) != 2 {
		t.Fatalf("라인=%d", len(got.Lines))
	}
	if got.Lines[0].Name != "감열지" || got.Lines[0].Qty != 2 || got.Lines[0].UnitPrice != 150000 {
		t.Fatalf("복사가 다르다 %+v", got.Lines[0])
	}
	if got.Lines[0].QuoteQty != 2 || got.Lines[0].QuoteUnitPrice != 150000 {
		t.Fatal("견적 스냅샷이 없다")
	}
	if got.DiffersFromQuote() {
		t.Fatal("그대로인데 견적과 다름")
	}
	if !strings.HasPrefix(got.OrderNo, "VI-수주-") {
		t.Fatalf("번호=%s", got.OrderNo)
	}

	got.Lines[0].Qty = 3
	if err := orders.Update(got); err != nil {
		t.Fatal(err)
	}
	after, err := orders.Get(got.OrderID)
	if err != nil {
		t.Fatal(err)
	}
	if !after.DiffersFromQuote() || !after.Lines[0].DiffersFromQuote() {
		t.Fatal("수량 바꿨는데 견적과 다름이 없다")
	}
	if after.Lines[0].QuoteQty != 2 || after.Lines[0].LineID != got.Lines[0].LineID {
		t.Fatal("라인 id 또는 견적 스냅샷이 바뀌었다")
	}
}

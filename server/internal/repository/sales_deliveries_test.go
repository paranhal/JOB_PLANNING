package repository

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestDeliveryCreatesAssetsBySerialSkipsConsumableLaborAndUnplanned(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "deliver.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c1','세종시교육청','세종시교육청',1)`); err != nil {
		t.Fatal(err)
	}

	items := NewSalesItemRepo(db)
	gate := &model.SalesItem{Name: "RFID 게이트", ItemKind: model.SalesItemKindGoods, Category: model.SalesItemCatRFID,
		Model: "EZ-5120", Manufacturer: "앤로보틱스", IsActive: true}
	paper := &model.SalesItem{Name: "감열지", ItemKind: model.SalesItemKindGoods, Category: model.SalesItemCatConsumable, IsActive: true}
	labor := &model.SalesItem{Name: "응용SW개발", ItemKind: model.SalesItemKindLabor, IsActive: true}
	asjob := &model.SalesItem{Name: "AS 출장", ItemKind: model.SalesItemKindService, IsActive: true}
	for _, it := range []*model.SalesItem{gate, paper, labor, asjob} {
		if err := items.Create(it); err != nil {
			t.Fatal(err)
		}
	}

	quotes := NewQuoteRepo(db)
	q := &model.SalesQuote{
		FormType: model.QuoteFormA, VATMode: model.QuoteVATExcluded, RoundRule: model.QuoteRoundNone,
		QuoteDate: time.Now().Format("2006-01-02"), Title: "납품", OwnerName: "최혜영", OwnerPhone: "010",
		CustomerID: "c1", RecipientName: "세종시교육청", SalesID: mustSalesForQuote(t, db),
		Lines: []model.SalesQuoteLine{
			{ItemID: gate.ItemID, Name: gate.Name, Qty: 3, Unit: "EA", UnitPrice: 1000},
			{ItemID: paper.ItemID, Name: paper.Name, Qty: 10, Unit: "EA", UnitPrice: 100},
			{ItemID: labor.ItemID, Name: labor.Name, Qty: 1, Unit: "식", UnitPrice: 1},
			{ItemID: asjob.ItemID, Name: asjob.Name, Qty: 1, Unit: "식", UnitPrice: 1},
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
	got, _ := orders.Get(o.OrderID)
	if got.CustomerID != "c1" {
		t.Fatalf("customer=%s", got.CustomerID)
	}

	lineOf := func(name string) string {
		for _, ln := range got.Lines {
			if ln.Name == name {
				return ln.LineID
			}
		}
		t.Fatalf("라인 없음 %s", name)
		return ""
	}

	created, err := orders.AddDelivery(&model.SalesDelivery{
		OrderID: o.OrderID, LineID: lineOf(gate.Name), Qty: 3, DeliveryDate: "2026-09-04",
		Place: "로비", ReceiverName: "김사서", InstalledBy: "양기헌",
	}, []string{"SN-A", "SN-B", "SN-C"}, "1층")
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 3 {
		t.Fatalf("게이트 자산=%d", len(created))
	}
	sns := map[string]bool{}
	for _, a := range created {
		sns[a.SerialNumber] = true
		if a.SalesOrderID != o.OrderID || a.InstallDate != "2026-09-04" || a.InstallerType != model.InstallerTypeSelf {
			t.Fatalf("자산 필드 %+v", a)
		}
		if !a.MaintFieldsEmpty() {
			t.Fatal("maint_* 가 채워졌다")
		}
		full, err := NewAssetRepo(db).GetByID(a.AssetID)
		if err != nil || full == nil {
			t.Fatal(err)
		}
		if !full.MaintFieldsEmpty() || full.SalesOrderID != o.OrderID {
			t.Fatalf("GetByID maint/수주 %+v", full)
		}
	}
	if !sns["SN-A"] || !sns["SN-B"] || !sns["SN-C"] {
		t.Fatalf("시리얼 %+v", sns)
	}

	for _, name := range []string{paper.Name, labor.Name, asjob.Name} {
		n, err := orders.AddDelivery(&model.SalesDelivery{
			OrderID: o.OrderID, LineID: lineOf(name), Qty: 1, DeliveryDate: "2026-09-04",
		}, []string{"SHOULD-NOT"}, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(n) != 0 {
			t.Fatalf("%s 자산=%d", name, len(n))
		}
	}

	preview, err := NewMaintenanceRepo(db).PreviewSiteConfigsFromAssets()
	if err != nil {
		t.Fatal(err)
	}
	if len(preview) != 0 {
		t.Fatalf("정기점검 대상에 들어갔다: %+v", preview)
	}

	empty, err := orders.AddDelivery(&model.SalesDelivery{
		OrderID: o.OrderID, LineID: lineOf(gate.Name), Qty: 1, DeliveryDate: "2026-09-05",
	}, nil, "2층")
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 1 || empty[0].SerialNumber != "" {
		t.Fatalf("빈 시리얼 %+v", empty)
	}

	upItems, _, err := NewWorkBoardRepo(db).ListUnplanned("", nil, model.UnplannedAssetSerial)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, it := range upItems {
		if it.HasKind(model.UnplannedAssetSerial) && it.RefID == empty[0].AssetID {
			found = true
			if !strings.Contains(it.Title, model.UnplannedAssetSerialTitle) {
				t.Fatalf("제목=%q", it.Title)
			}
		}
	}
	if !found {
		t.Fatal("미계획 업무함에 시리얼 미입력이 없다")
	}
}

package repository

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestSalesItemSpecsReplaceSearchMigrateDeactivate(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "item_specs.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewSalesItemRepo(db)

	it := &model.SalesItem{Name: "클리너카드", ItemKind: model.SalesItemKindGoods, IsActive: true}
	if err := repo.Create(it); err != nil {
		t.Fatal(err)
	}
	specs := []model.SalesItemSpec{
		{Spec: "A.D형", Unit: "EA", Price: 24000},
		{Spec: "T형", Unit: "EA", Price: 39600},
		{Spec: "Box", Unit: "Box", Price: 79200},
	}
	if _, err := repo.ReplaceSpecsAndPrices(it.ItemID, specs, nil); err != nil {
		t.Fatal(err)
	}
	got, err := repo.ListSpecs(it.ItemID, true)
	if err != nil || len(got) != 3 {
		t.Fatalf("3줄: n=%d err=%v", len(got), err)
	}
	if got[0].Spec != "A.D형" || got[1].Spec != "T형" || got[2].Spec != "Box" {
		t.Fatalf("순서: %+v", got)
	}

	if _, err := repo.ReplaceSpecsAndPrices(it.ItemID, nil, nil); err != nil {
		t.Fatal(err)
	}
	got, err = repo.ListSpecs(it.ItemID, true)
	if err != nil || len(got) != 0 {
		t.Fatalf("0줄: n=%d err=%v", len(got), err)
	}

	if _, err := repo.ReplaceSpecsAndPrices(it.ItemID, specs, nil); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.ListSpecs(it.ItemID, true)
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES
		('SUP1','공급A','공급A',1),('SUP2','공급B','공급B',1)`); err != nil {
		t.Fatal(err)
	}
	prices := []model.SalesItemSupplierPrice{
		{SupplierID: "SUP1", SpecID: got[0].SpecID, Unit: "", Price: 18000},
		{SupplierID: "SUP1", SpecID: got[1].SpecID, Price: 25000},
		{SupplierID: "SUP2", SpecID: got[0].SpecID, Price: 19000},
		{SupplierID: "SUP2", SpecID: got[1].SpecID, Price: 26000},
	}
	if _, err := repo.ReplaceSpecsAndPrices(it.ItemID, got, prices); err != nil {
		t.Fatal(err)
	}
	loaded := *it
	if err := repo.LoadCatalog(&loaded); err != nil {
		t.Fatal(err)
	}
	if len(loaded.Suppliers) != 2 {
		t.Fatalf("공급사 묶음=%d", len(loaded.Suppliers))
	}
	nRows := 0
	for _, b := range loaded.Suppliers {
		nRows += len(b.Rows)
	}
	if nRows != 4 {
		t.Fatalf("공급사 줄=%d", nRows)
	}

	found, err := repo.List(SalesItemFilter{Search: "A.D형"})
	if err != nil || len(found) == 0 || found[0].ItemID != it.ItemID {
		t.Fatalf("규격 검색: %+v err=%v", found, err)
	}
	found = repo.AttachSpecSummaries(found)
	if found[0].SpecRangeLabel() != "3개 규격 · 24,000~79,200원" {
		t.Fatalf("목록 라벨=%q", found[0].SpecRangeLabel())
	}

	sales := NewSalesRepo(db)
	p := &model.SalesProject{Name: "견적연결"}
	if err := sales.Create(p); err != nil {
		t.Fatal(err)
	}
	q := &model.SalesQuote{
		SalesID: p.SalesID, FormType: model.QuoteFormA, VATMode: model.QuoteVATExcluded,
		RoundRule: model.QuoteRoundNone, QuoteDate: time.Now().Format("2006-01-02"),
		Title: "클리너", OwnerName: "최혜영", OwnerPhone: "010-1111-2222", RecipientName: "도서관",
		Lines: []model.SalesQuoteLine{
			{Name: "클리너카드", Spec: got[0].Spec, SpecID: got[0].SpecID, Qty: 1, Unit: "EA", UnitPrice: 24000},
		},
	}
	if err := NewQuoteRepo(db).Create(q); err != nil {
		t.Fatal(err)
	}
	off, err := repo.ReplaceSpecsAndPrices(it.ItemID, got[1:], nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(off) != 1 || off[0] != got[0].SpecID {
		t.Fatalf("끄기=%v", off)
	}
	all, _ := repo.ListSpecs(it.ItemID, true)
	var inactive *model.SalesItemSpec
	for i := range all {
		if all[i].SpecID == got[0].SpecID {
			inactive = &all[i]
		}
	}
	if inactive == nil || inactive.IsActive {
		t.Fatalf("견적 규격이 지워졌거나 켜져 있다: %+v", all)
	}

	mig := &model.SalesItem{Name: "이관품목", Spec: "short", Unit: "EA", ListPrice: 12000, IsActive: true}
	if err := repo.Create(mig); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM sales_item_specs WHERE item_id=?`, mig.ItemID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM id_sequences WHERE seq_key=?`, itemSpecsMetaKey); err != nil {
		t.Fatal(err)
	}
	applySalesItemSpecs(db)
	n, err := repo.ListSpecs(mig.ItemID, true)
	if err != nil || len(n) != 1 || n[0].Spec != "short" || n[0].Price != 12000 {
		t.Fatalf("이관: %+v err=%v", n, err)
	}

	m := model.SpecMargins(loaded.Specs, func() []model.SalesItemSupplierPrice {
		var out []model.SalesItemSupplierPrice
		for _, b := range loaded.Suppliers {
			out = append(out, b.Rows...)
		}
		return out
	}())
	if len(m) == 0 || m[0].Profit != 24000-18000 {
		t.Fatalf("이익: %+v", m)
	}
}

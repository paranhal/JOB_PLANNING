package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestSalesItemsSeedFromAssetsDistinctAndReview(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "sales_items.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('c1','도서관','도서관',1)`); err != nil {
		t.Fatal(err)
	}
	for _, row := range [][]string{
		{"AS1", "자동대출반납기용 감열지", "59x80mm", "나이콤", "materials", "SN1"},
		{"AS2", "자동대출반납기용 감열지", "59x80mm", "나이콤", "materials", "SN2"},
		{"AS3", "RFID 게이트", "EZ-5120HCF", "앤로보틱스", "rfid", "SN3"},
	} {
		if _, err := db.Exec(`
			INSERT INTO assets (asset_id, customer_id, product_name, model_name, manufacturer, product_category, serial_number)
			VALUES (?,?,?,?,?,?,?)`, row[0], "c1", row[1], row[2], row[3], row[4], row[5]); err != nil {
			t.Fatal(err)
		}
	}
	if err := SeedSalesItemsFromAssets(db); err != nil {
		t.Fatal(err)
	}
	if err := SeedSalesItemsFromAssets(db); err != nil {
		t.Fatal(err)
	}

	repo := NewSalesItemRepo(db)
	items, err := repo.List(SalesItemFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("distinct 품목=%d", len(items))
	}
	var paper, gate *model.SalesItem
	for i := range items {
		switch items[i].Name {
		case "자동대출반납기용 감열지":
			paper = &items[i]
		case "RFID 게이트":
			gate = &items[i]
		}
	}
	if paper == nil || gate == nil {
		t.Fatalf("시드 품명이 없다: %+v", items)
	}
	if paper.IsActive || !paper.NeedsReview || paper.Notes != "확인 필요" {
		t.Fatalf("감열지 초안: active=%v review=%v notes=%q", paper.IsActive, paper.NeedsReview, paper.Notes)
	}
	if paper.Category != model.SalesItemCatConsumable || paper.ItemKind != model.SalesItemKindGoods {
		t.Fatalf("감열지 분류=%s kind=%s", paper.Category, paper.ItemKind)
	}
	if gate.Category != model.SalesItemCatRFID {
		t.Fatalf("게이트 분류=%s", gate.Category)
	}

	sales := NewSalesRepo(db)
	p := &model.SalesProject{Name: "기존 구축", IsTentativeName: true}
	if err := sales.Create(p); err != nil {
		t.Fatal(err)
	}
	got, err := sales.Get(p.SalesID)
	if err != nil {
		t.Fatal(err)
	}
	if got.DealType != model.SalesDealBuild || got.Stage != model.SalesStage4Discover {
		t.Fatalf("영업 기본값이 바뀌었다: deal=%s stage=%s", got.DealType, got.Stage)
	}

	if err := repo.TouchLastQuoted(paper.ItemID, 150000, "세종시립도서관", "2026-08-16"); err != nil {
		t.Fatal(err)
	}
	fresh, err := repo.Get(paper.ItemID)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.LastPrice != 150000 || fresh.LastCustomer != "세종시립도서관" || fresh.LastQuotedAt != "2026-08-16" {
		t.Fatalf("최근 견적: %+v", fresh)
	}
}

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

	if _, err := db.Exec(`UPDATE sales_items SET last_price=150000, last_customer='세종시립도서관', last_quoted_at='2026-08-16' WHERE item_id=?`, paper.ItemID); err != nil {
		t.Fatal(err)
	}
	fresh, err := repo.Get(paper.ItemID)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.LastPrice != 150000 {
		t.Fatalf("남은 last_price 열: %+v", fresh)
	}
	if err := repo.Update(&model.SalesItem{
		ItemID: fresh.ItemID, ItemKind: fresh.ItemKind, Name: fresh.Name, Spec: fresh.Spec,
		Unit: fresh.Unit, Manufacturer: fresh.Manufacturer, IsActive: fresh.IsActive, NeedsReview: fresh.NeedsReview,
	}); err != nil {
		t.Fatal(err)
	}
	after, err := repo.Get(paper.ItemID)
	if err != nil {
		t.Fatal(err)
	}
	if after.LastPrice != 150000 || after.LastCustomer != "세종시립도서관" {
		t.Fatalf("수정이 last_* 를 지웠다: %+v", after)
	}
}

func TestLinkSalesItemPartiesByExactName(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "item_party.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, party_kind) VALUES ('C00-26-001','앤로보틱스','앤로보틱스',1,'partner')`); err != nil {
		t.Fatal(err)
	}
	repo := NewSalesItemRepo(db)
	it := &model.SalesItem{Name: "RFID 게이트", Manufacturer: "앤로보틱스", Unit: "EA", IsActive: true}
	if err := repo.Create(it); err != nil {
		t.Fatal(err)
	}
	linkSalesItemParties(db)
	got, err := repo.Get(it.ItemID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ManufacturerID != "C00-26-001" {
		t.Fatalf("이관 연결 안 됨: %+v", got)
	}
	if got.Manufacturer != "앤로보틱스" {
		t.Fatalf("글자값을 지웠다: %q", got.Manufacturer)
	}
}

func TestCleanLaborJobNamesMerge(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "labor_clean.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`DELETE FROM labor_rates WHERE year=2025`); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO labor_rates (rate_id, year, job_code, job_name, monthly) VALUES
		('LR2025-01',2025,'01','1 IT기획자',1),
		('LR2025-99',2025,'99','① IT기획자',2)`)
	if err != nil {
		t.Fatal(err)
	}
	cleanLaborJobNames(db)
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM labor_rates WHERE year=2025 AND job_name='IT기획자'`).Scan(&n)
	if n != 1 {
		t.Fatalf("합친 뒤 건수=%d", n)
	}
	var name string
	_ = db.QueryRow(`SELECT job_name FROM labor_rates WHERE rate_id='LR2025-01'`).Scan(&name)
	if name != "IT기획자" {
		t.Fatalf("앞 행 이름=%q", name)
	}
	var gone int
	_ = db.QueryRow(`SELECT COUNT(*) FROM labor_rates WHERE rate_id='LR2025-99'`).Scan(&gone)
	if gone != 0 {
		t.Fatal("뒤엣것을 안 버렸다")
	}
}

func TestCreatePartnerNameOnly(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "partner.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	custo := NewCustomerRepo(db)
	a, existed, err := custo.CreatePartnerNameOnly("나이콤")
	if err != nil || existed || a == nil {
		t.Fatalf("create: existed=%v err=%v", existed, err)
	}
	if a.PartyKind != model.PartyKindPartner || a.CustomerID == "" {
		t.Fatalf("파트너: %+v", a)
	}
	b, existed, err := custo.CreatePartnerNameOnly("나이콤")
	if err != nil || !existed || b.CustomerID != a.CustomerID {
		t.Fatalf("같은 이름: existed=%v id=%s want=%s", existed, b.CustomerID, a.CustomerID)
	}
}

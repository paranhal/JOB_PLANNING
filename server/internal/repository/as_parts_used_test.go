package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestASPartsUsedReplaceAndSummary(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "parts.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c1','도서관','도서관',1,'O01')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, customer_id, symptom, status, org_id)
		VALUES ('as1','AS-1','c1','게이트','in_progress','O01')`); err != nil {
		t.Fatal(err)
	}
	r := NewASPartsUsedRepo(db)
	rows := []model.ASPartUsed{
		{PartName: "메인보드", PartCode: "PT-M01", Qty: 1},
		{PartName: "팬", Qty: 2},
	}
	if err := r.ReplaceByAS("as1", "P-1", rows); err != nil {
		t.Fatal(err)
	}
	got, err := r.ListByAS("as1")
	if err != nil || len(got) != 2 {
		t.Fatalf("n=%d err=%v", len(got), err)
	}
	sum := model.FormatPartsUsedSummary(got)
	if sum != "메인보드(PT-M01) ×1, 팬 ×2" {
		t.Fatalf("summary=%q", sum)
	}
	if err := r.DeleteByAS("as1"); err != nil {
		t.Fatal(err)
	}
	got, _ = r.ListByAS("as1")
	if len(got) != 0 {
		t.Fatalf("delete leftover %d", len(got))
	}
}

func TestSuggestPartsOrgAndShortQuery(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "parts-sug.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO sales_items (item_id, name, gov_item_no, is_active, org_id)
		VALUES ('i1','메인보드','PT-M01',1,'O01'),
		       ('i2','SI보드','PT-S01',1,'O03'),
		       ('i3','공용팬','FAN-1',1,'')`); err != nil {
		t.Fatal(err)
	}
	repo := NewSalesItemRepo(db)
	short, err := repo.SuggestParts("O01", "메", 20)
	if err != nil || len(short) != 0 {
		t.Fatalf("1글자 n=%d err=%v", len(short), err)
	}
	hit, err := repo.SuggestParts("O01", "메인", 20)
	if err != nil || len(hit) != 1 || hit[0].ItemID != "i1" {
		t.Fatalf("O01 메인 %+v err=%v", hit, err)
	}
	shared, err := repo.SuggestParts("O01", "공용", 20)
	if err != nil || len(shared) != 1 {
		t.Fatalf("공용 org 빈칸 %+v err=%v", shared, err)
	}
	other, err := repo.SuggestParts("O01", "SI보", 20)
	if err != nil || len(other) != 0 {
		t.Fatalf("타조직 노출 %+v err=%v", other, err)
	}
}

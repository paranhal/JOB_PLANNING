package repository

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestASReceiptGroupCreateAndList(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "as_group.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?,?,?,1)`,
		"c1", "기관", "기관"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO assets (asset_id, customer_id, product_name) VALUES ('A1','c1','자가대출기')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO assets (asset_id, customer_id, product_name) VALUES ('A2','c1','사서대출기')`); err != nil {
		t.Fatal(err)
	}

	repo := NewASRepo(db)
	gid := NewReceiptGroupID()
	at := time.Date(2026, 8, 31, 10, 0, 0, 0, time.Local)
	a := &model.ASReceipt{
		CustomerID: "c1", AssetID: "A1", Symptom: "카드 인식 안 됨",
		ReceiptDatetime: at, ReceiptGroupID: gid, ConfirmContact: "010-0000-0000",
	}
	b := &model.ASReceipt{
		CustomerID: "c1", AssetID: "A2", Symptom: "영수증 용지 걸림",
		ReceiptDatetime: at, ReceiptGroupID: gid,
	}
	if err := repo.Create(a); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(b); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetByID(a.ASID)
	if err != nil || got == nil {
		t.Fatalf("get: %v", err)
	}
	if got.ReceiptGroupID != gid {
		t.Fatalf("group=%s want %s", got.ReceiptGroupID, gid)
	}
	if got.ConfirmContact != "010-0000-0000" {
		t.Fatalf("contact=%q", got.ConfirmContact)
	}
	mates, err := repo.ListByReceiptGroup(gid)
	if err != nil {
		t.Fatal(err)
	}
	if len(mates) != 2 {
		t.Fatalf("mates=%d", len(mates))
	}
	items, _, err := repo.ListFiltered("", "", "", nil, "", "", 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	var seen int
	for _, it := range items {
		if it.ASID == a.ASID || it.ASID == b.ASID {
			seen++
			if it.GroupSize != 2 {
				t.Fatalf("%s group_size=%d", it.ASNumber, it.GroupSize)
			}
		}
	}
	if seen != 2 {
		t.Fatalf("목록에서 묶음 건 %d", seen)
	}
}

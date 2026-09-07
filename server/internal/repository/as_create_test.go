package repository

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestASCreateSkipsTakenNumber(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "as_num.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?,?,?,1)`,
		"c1", "기관", "기관"); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 8, 19, 10, 0, 0, 0, time.Local)
	if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, receipt_datetime, customer_id, status, created_at, updated_at)
		VALUES ('R2608-001','R2608-001',?,?, 'received', datetime('now'), datetime('now'))`,
		at.Format("2006-01-02 15:04:05"), "c1"); err != nil {
		t.Fatal(err)
	}

	got := &model.ASReceipt{
		CustomerID: "c1", Symptom: "충돌 재시도", ReceiptDatetime: at,
	}
	if err := NewASRepo(db).Create(got); err != nil {
		t.Fatal(err)
	}
	if got.ASID != "R2608-002" {
		t.Fatalf("as_id=%s want R2608-002", got.ASID)
	}
}

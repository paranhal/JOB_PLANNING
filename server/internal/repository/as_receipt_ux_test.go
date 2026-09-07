package repository

import (
	"path/filepath"
	"testing"
)

func TestApplyAS34ReceiptUXSeedsPartyKindAndCodes(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "ux.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('C000-26-008','비젼아이티','비젼아이티',1),
		       ('C000-26-009','채움씨앤아이','채움씨앤아이',1),
		       ('C1','가나도서관','가나도서관',1)`); err != nil {
		t.Fatal(err)
	}
	applyAS34ReceiptUX(db)

	var own, partner, cust string
	if err := db.QueryRow(`SELECT party_kind FROM customers WHERE customer_id='C000-26-008'`).Scan(&own); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT party_kind FROM customers WHERE customer_id='C000-26-009'`).Scan(&partner); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COALESCE(party_kind,'') FROM customers WHERE customer_id='C1'`).Scan(&cust); err != nil {
		t.Fatal(err)
	}
	if own != "own" {
		t.Fatalf("비젼아이티 party_kind=%q", own)
	}
	if partner != "partner" {
		t.Fatalf("채움씨앤아이 party_kind=%q", partner)
	}
	if cust != "customer" && cust != "" {
		t.Fatalf("가나도서관 party_kind=%q", cust)
	}

	list, err := NewCustomerRepo(db).ListForReceipt()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range list {
		if c.OrgName == "비젼아이티" || c.OrgName == "채움씨앤아이" {
			t.Fatalf("접수 목록에 협력사/자사가 있다: %s", c.OrgName)
		}
	}
	foundLib := false
	for _, c := range list {
		if c.OrgName == "가나도서관" {
			foundLib = true
		}
	}
	if !foundLib {
		t.Fatal("고객 가나도서관이 접수 목록에 없다")
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM codes WHERE code_group LIKE 'urgency_reason%'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n < 10 {
		t.Fatalf("긴급 사유 코드 %d개", n)
	}
	var prime, internal int
	db.QueryRow(`SELECT COUNT(*) FROM codes WHERE code_group='receipt_channel' AND code_value='prime'`).Scan(&prime)
	db.QueryRow(`SELECT COUNT(*) FROM codes WHERE code_group='receipt_channel' AND code_value='internal'`).Scan(&internal)
	if prime != 1 || internal != 1 {
		t.Fatalf("채널 prime=%d internal=%d", prime, internal)
	}

	ownItems, total, err := NewCustomerRepo(db).List("", "", "", "", "", 1, 20, false, "own")
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(ownItems) != 1 || ownItems[0].OrgName != "비젼아이티" {
		t.Fatalf("own 필터: total=%d items=%v", total, ownItems)
	}
}

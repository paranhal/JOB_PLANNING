package repository

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestRebuildKeywordLinksFromDictionary(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "kw-links.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?,?,?,1)`,
		"c1", "기관", "기관"); err != nil {
		t.Fatal(err)
	}
	repo := NewASRepo(db)
	as := &model.ASReceipt{
		CustomerID: "c1", Symptom: "홈페이지가 열리지 않습니다",
		ReceiptDatetime: time.Now(), Urgency: "normal", Priority: "normal",
	}
	if err := repo.Create(as); err != nil {
		t.Fatal(err)
	}

	n, err := repo.RebuildKeywordLinks(nil)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("홈페이지 1행 want 1 got %d", n)
	}
	if got := countKeywordLinks(t, db, ""); got != 1 {
		t.Fatalf("링크 수 %d", got)
	}
	n2, err := repo.RebuildKeywordLinks(nil)
	if err != nil {
		t.Fatal(err)
	}
	if n2 != 1 || countKeywordLinks(t, db, "") != 1 {
		t.Fatalf("두 번 돌려도 늘면 안 됨 n=%d rows=%d", n2, countKeywordLinks(t, db, ""))
	}

	if _, err := db.Exec(`UPDATE as_receipts SET symptom=? WHERE as_id=?`, "전자도서관 접속 불가", as.ASID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.RebuildKeywordLinks([]string{as.ASID}); err != nil {
		t.Fatal(err)
	}
	if countKeywordLinks(t, db, "KW001") != 0 {
		t.Fatal("옛 홈페이지 링크가 남아 있다")
	}
	if countKeywordLinks(t, db, "KW002") != 1 {
		t.Fatal("전자도서관 링크가 없다")
	}

	as2 := &model.ASReceipt{
		CustomerID: "c1", Symptom: "홈페이지 오류가 납니다",
		ReceiptDatetime: time.Now(), Urgency: "normal", Priority: "normal",
	}
	if err := repo.Create(as2); err != nil {
		t.Fatal(err)
	}
	n3, err := repo.RebuildKeywordLinks([]string{as2.ASID})
	if err != nil {
		t.Fatal(err)
	}
	if n3 != 2 {
		t.Fatalf("키워드 두 개 → 2행 got %d", n3)
	}

	as3 := &model.ASReceipt{
		CustomerID: "c1", Symptom: "전혀다른고유증상XYZ",
		ReceiptDatetime: time.Now(), Urgency: "normal", Priority: "normal",
	}
	if err := repo.Create(as3); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_keyword_links(as_id, keyword_id, source, field) VALUES (?,?,?,?)`,
		as3.ASID, "KW001", model.KWSourceManual, model.KWFieldSymptom); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.RebuildKeywordLinks(nil); err != nil {
		t.Fatal(err)
	}
	var src string
	err = db.QueryRow(`SELECT source FROM as_keyword_links WHERE as_id=? AND keyword_id=? AND field=?`,
		as3.ASID, "KW001", model.KWFieldSymptom).Scan(&src)
	if err != nil || src != model.KWSourceManual {
		t.Fatalf("manual 링크가 지워졌다 src=%q err=%v", src, err)
	}

	var idx int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_as_keyword_links_kw'`).Scan(&idx); err != nil || idx != 1 {
		t.Fatalf("idx_as_keyword_links_kw missing n=%d err=%v", idx, err)
	}
}

func countKeywordLinks(t *testing.T, db *sql.DB, keywordID string) int {
	t.Helper()
	var n int
	var err error
	if keywordID == "" {
		err = db.QueryRow(`SELECT COUNT(*) FROM as_keyword_links`).Scan(&n)
	} else {
		err = db.QueryRow(`SELECT COUNT(*) FROM as_keyword_links WHERE keyword_id=?`, keywordID).Scan(&n)
	}
	if err != nil {
		t.Fatal(err)
	}
	return n
}

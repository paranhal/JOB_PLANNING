package repository

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestASPartsUsedSchema074NoSeed(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "parts074.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='as_parts_used'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("table n=%d err=%v", n, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM as_parts_used`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("시드 금지 count=%d err=%v", n, err)
	}
	var col int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('as_parts_used') WHERE name IN ('unit_price','amount','price')`).Scan(&col); err != nil || col != 0 {
		t.Fatalf("단가·금액 컬럼 금지 col=%d err=%v", col, err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('c074','기관','기관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, customer_id, symptom, status, receipt_datetime)
		VALUES ('as074','AS-074','c074','증상','received', datetime('now'))`); err != nil {
		t.Fatal(err)
	}

	down, err := asPartsUsedDownSQL()
	if err != nil {
		t.Fatal(err)
	}
	if err := execSQLIgnoreDupCol(db, down); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='as_parts_used'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("down 후 table n=%d err=%v", n, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM as_receipts WHERE as_id='as074'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("기존 접수 불변 n=%d err=%v", n, err)
	}
	applyASPartsUsed(db)
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='as_parts_used'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("재적용 n=%d err=%v", n, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM as_parts_used`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("재적용 시드 count=%d", n)
	}
	up, _ := asPartsUsedMigrationSQL()
	for _, line := range strings.Split(up, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "--") {
			continue
		}
		if strings.Contains(strings.ToUpper(trim), "INSERT") {
			t.Fatal("074 up SQL 에 INSERT 가 있으면 안 됨")
		}
	}
}

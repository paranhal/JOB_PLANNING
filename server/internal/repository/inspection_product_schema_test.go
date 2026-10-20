package repository

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectionProductSchema072NoSeed(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "insp072.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for _, name := range []string{"inspection_products", "site_inspection_products"} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&n); err != nil || n != 1 {
			t.Fatalf("table %s n=%d err=%v", name, n, err)
		}
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM inspection_products`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("시드 금지 count=%d err=%v", n, err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c072','기관','기관',1,'O01')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_site_config (customer_id, short_name, has_klas, has_rfid, inspection_cycle, entry_category)
		VALUES ('c072','기관',1,1,'monthly','normal')`); err != nil {
		t.Fatal(err)
	}

	down, err := inspectionProductDownSQL()
	if err != nil {
		t.Fatal(err)
	}
	if err := execSQLIgnoreDupCol(db, down); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='inspection_products'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("down 후 table n=%d err=%v", n, err)
	}
	var klas int
	if err := db.QueryRow(`SELECT has_klas FROM maintenance_site_config WHERE customer_id='c072'`).Scan(&klas); err != nil || klas != 1 {
		t.Fatalf("has_klas 유지 klas=%d err=%v", klas, err)
	}
	applyInspectionProducts(db)
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='inspection_products'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("재적용 n=%d err=%v", n, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM inspection_products`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("재적용 시드 count=%d", n)
	}
	up, _ := inspectionProductMigrationSQL()
	for _, line := range strings.Split(up, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "--") {
			continue
		}
		if strings.Contains(strings.ToUpper(trim), "INSERT") {
			t.Fatal("072 up SQL 에 INSERT 가 있으면 안 됨")
		}
	}
}

package repository

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestContractImportSchema073NoSeed(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "c073.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, name := range []string{"contract_end_customers", "contract_import_batches"} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&n); err != nil || n != 1 {
			t.Fatalf("table %s n=%d err=%v", name, n, err)
		}
	}
	if !workProjectHasColumn(db, "direction") {
		t.Fatal("direction")
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM contract_import_batches`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("시드 금지 %d %v", n, err)
	}
	down, err := contractImportDownSQL()
	if err != nil {
		t.Fatal(err)
	}
	if err := execSQLIgnoreDupCol(db, down); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='contract_end_customers'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("down n=%d", n)
	}
	applyContractImport(db)
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='contract_end_customers'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("재적용 n=%d", n)
	}
	up, _ := contractImportMigrationSQL()
	for _, line := range strings.Split(up, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "--") {
			continue
		}
		if strings.Contains(strings.ToUpper(trim), "INSERT") {
			t.Fatal("073 up SQL 에 INSERT 가 있으면 안 됨")
		}
	}
}

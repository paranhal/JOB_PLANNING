package repository

import (
	"path/filepath"
	"testing"
)

func TestMnt95_15AMigrationUpDown(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "m083.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('maintenance_site_config') WHERE name='bulk_batch_id'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("bulk_batch_id n=%d err=%v", n, err)
	}
	up, err := mnt9515AMigrationSQL()
	if err != nil {
		t.Fatal(err)
	}
	if mnt9515AHasSeedInsert(up) {
		t.Fatal("시드 INSERT 금지")
	}
	down, err := mnt9515ADownSQL()
	if err != nil || down == "" {
		t.Fatalf("down %v", err)
	}
	if err := execSQLIgnoreDupCol(db, down); err != nil {
		t.Fatal(err)
	}
	if err := execSQLIgnoreDupCol(db, up); err != nil {
		t.Fatal(err)
	}
}

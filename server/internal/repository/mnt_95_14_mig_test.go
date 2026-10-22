package repository

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestMnt95_14MigrationUpDown(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "m082.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, col := range []string{"round_no", "work_group_id"} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('maintenance_visits') WHERE name=?`, col).Scan(&n); err != nil || n != 1 {
			t.Fatalf("visits.%s n=%d err=%v", col, n, err)
		}
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('maintenance_site_config') WHERE name='rounds_per_period'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("rounds_per_period n=%d err=%v", n, err)
	}
	for _, tbl := range []string{"maintenance_visit_assignees", "maintenance_region_assignees", "maintenance_site_assignees"} {
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, tbl).Scan(&n); err != nil || n != 1 {
			t.Fatalf("table %s n=%d err=%v", tbl, n, err)
		}
	}
	up, err := mnt9514MigrationSQL()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToUpper(up), "INSERT INTO") && !strings.Contains(up, "--") {
		for _, line := range strings.Split(up, "\n") {
			trim := strings.TrimSpace(line)
			if strings.HasPrefix(trim, "--") {
				continue
			}
			if strings.Contains(strings.ToUpper(trim), "INSERT") {
				t.Fatal("시드 INSERT 금지")
			}
		}
	}
	down, err := mnt9514DownSQL()
	if err != nil || strings.TrimSpace(down) == "" {
		t.Fatalf("down %v", err)
	}
	if err := execSQLIgnoreDupCol(db, down); err != nil {
		t.Fatal(err)
	}
	if err := execSQLIgnoreDupCol(db, up); err != nil {
		t.Fatal(err)
	}
}

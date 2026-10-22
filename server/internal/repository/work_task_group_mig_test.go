package repository

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkTaskGroupColumnUpDown(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "wtg.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('work_tasks') WHERE name='work_group_id'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatal("InitDB 후 work_group_id 없음")
	}

	up, err := workTaskGroupMigrationSQL()
	if err != nil || !strings.Contains(up, "work_group_id") || !strings.Contains(up, "ix_work_tasks_group") {
		t.Fatalf("up sql %v %q", err, up)
	}
	if workTaskActionSQLHasSeedInsert(up) {
		t.Fatal("시드 INSERT")
	}
	down, err := workTaskGroupDownSQL()
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

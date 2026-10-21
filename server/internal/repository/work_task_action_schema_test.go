package repository

import (
	"path/filepath"
	"testing"
)

func Test079WorkTaskActionNoSeed(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "p79.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	up, err := workTaskActionMigrationSQL()
	if err != nil {
		t.Fatal(err)
	}
	if workTaskActionSQLHasSeedInsert(up) {
		t.Fatal("079 up SQL 에 시드 INSERT 가 있으면 안 됨")
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('work_tasks') WHERE name='action_taken'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("action_taken n=%d err=%v", n, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='work_task_processes'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("processes n=%d err=%v", n, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='work_task_parts_used'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("parts n=%d err=%v", n, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_task_processes`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("시드 금지 processes=%d", n)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_task_parts_used`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("시드 금지 parts=%d", n)
	}

	down, err := workTaskActionDownSQL()
	if err != nil {
		t.Fatal(err)
	}
	if err := execSQLIgnoreDupCol(db, down); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='work_task_processes'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("down processes n=%d", n)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='work_task_parts_used'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("down parts n=%d", n)
	}
}

func Test080WorkTaskActionExtraNoSeed(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "p80.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	up, err := workTaskActionExtraMigrationSQL()
	if err != nil {
		t.Fatal(err)
	}
	if workTaskActionSQLHasSeedInsert(up) {
		t.Fatal("080 up SQL 에 시드 INSERT 가 있으면 안 됨")
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('work_tasks') WHERE name='target_category_id'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("target_category_id n=%d err=%v", n, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('work_tasks') WHERE name='work_reason'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("work_reason n=%d err=%v", n, err)
	}
}

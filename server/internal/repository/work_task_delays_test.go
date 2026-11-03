package repository

import (
	"path/filepath"
	"strings"
	"testing"
)

func Test95_80_MigrationUpDownNoSeed(t *testing.T) {
	up, err := workTaskDelaysMigrationSQL()
	if err != nil {
		t.Fatal(err)
	}
	if workTaskDelaySQLHasSeedInsert(up) {
		t.Fatal("운영 보정 INSERT 금지")
	}
	if !strings.Contains(up, "work_task_delays") || !strings.Contains(up, "orig_work_date") {
		t.Fatal("up 스키마 부족")
	}
	down, err := workTaskDelaysDownSQL()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(down, "DROP TABLE") {
		t.Fatal("down 없음")
	}
	db, err := InitDB(filepath.Join(t.TempDir(), "m96.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	applyWorkTaskDelaysV96(db)
	applyWorkTaskDelaysV96(db)
	if _, err := db.Exec(`SELECT orig_work_date, orig_due_date, carry_count FROM work_tasks LIMIT 1`); err != nil {
		t.Fatal(err)
	}
	if err := execWorkGroupSQL(db, down); err != nil {
		t.Fatal(err)
	}
}

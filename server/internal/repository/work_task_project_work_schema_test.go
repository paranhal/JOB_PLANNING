package repository

import (
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func Test076WorkTaskProjectWorkNoSeed(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "p76.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='work_task_sites'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("work_task_sites n=%d err=%v", n, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_task_sites`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("시드 금지 sites=%d", n)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('work_tasks') WHERE name='work_subtype'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("work_subtype n=%d err=%v", n, err)
	}

	if _, err := db.Exec(`INSERT INTO work_tasks(task_id,work_type,title,status,org_id) VALUES('WT76','admin','기존',?,?)`, model.WBTaskWaiting, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	var sub string
	if err := db.QueryRow(`SELECT work_subtype FROM work_tasks WHERE task_id='WT76'`).Scan(&sub); err != nil || sub != "" {
		t.Fatalf("기존 행 subtype=%q err=%v", sub, err)
	}

	up, _ := workTaskProjectWorkMigrationSQL()
	for _, line := range strings.Split(up, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "--") {
			continue
		}
		if strings.Contains(strings.ToUpper(trim), "INSERT") {
			t.Fatal("076 up SQL 에 INSERT 가 있으면 안 됨")
		}
	}
	down, err := workTaskProjectWorkDownSQL()
	if err != nil {
		t.Fatal(err)
	}
	if err := execSQLIgnoreDupCol(db, down); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='work_task_sites'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("down sites n=%d", n)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_tasks WHERE task_id='WT76'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("down 후 업무 유지 n=%d", n)
	}
	applyWorkTaskProjectWork(db)
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='work_task_sites'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("재적용 n=%d", n)
	}
}

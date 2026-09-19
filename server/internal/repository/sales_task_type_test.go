package repository

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestApplySalesTaskTypeMigratesAdminWaiting(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "sales_type_mig.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	past := time.Now().AddDate(0, 0, -3).Format("2006-01-02")
	if _, err := db.Exec(`
		INSERT INTO work_tasks (task_id, work_type, title, due_date, work_date, status, progress, source_type, source_id, source_role)
		VALUES ('WT-MIG','admin','옛 영업','`+past+`','`+past+`','waiting',0,'sales_activity','SA-MIG','')`); err != nil {
		t.Fatal(err)
	}
	applySalesTaskType(db)

	var wt, st, role, complete string
	var progress int
	if err := db.QueryRow(`SELECT work_type, status, progress, source_role, COALESCE(complete_date,'') FROM work_tasks WHERE task_id='WT-MIG'`).
		Scan(&wt, &st, &progress, &role, &complete); err != nil {
		t.Fatal(err)
	}
	if wt != model.WBWorkSales || st != model.WBTaskComplete || progress != 100 || role != model.WBSourceRoleDone || complete != past {
		t.Fatalf("이관 결과 type=%s status=%s prog=%d role=%s complete=%s", wt, st, progress, role, complete)
	}

	if _, err := db.Exec(`
		INSERT INTO work_tasks (task_id, work_type, title, due_date, work_date, status, progress, source_type, source_id, source_role)
		VALUES ('WT-HAND','admin','손댄 건','`+past+`','`+past+`','in_progress',40,'sales_activity','SA-HAND','')`); err != nil {
		t.Fatal(err)
	}
	applySalesTaskType(db)
	if err := db.QueryRow(`SELECT work_type, status, progress FROM work_tasks WHERE task_id='WT-HAND'`).
		Scan(&wt, &st, &progress); err != nil {
		t.Fatal(err)
	}
	if wt != model.WBWorkSales || st != model.WBTaskInProgress || progress != 40 {
		t.Fatalf("손댄 건이 덮였다 type=%s status=%s prog=%d", wt, st, progress)
	}
}

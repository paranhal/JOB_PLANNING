package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestEnsureMaintenanceTasksCreatesMissing(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "ensure.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('C1','나성동도서관','나성동도서관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_plans (plan_id, plan_year, title, status)
		VALUES ('MP1', 2026, '2026', 'approved')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits
		(visit_id, plan_id, visit_date, customer_id, sort_order, entry_category, assignee, product_type, completed)
		VALUES ('v1','MP1','2026-08-14','C1',0,'normal','최혜영','KLAS',0)`); err != nil {
		t.Fatal(err)
	}

	repo := NewWBRepo(db)
	n, err := repo.EnsureMaintenanceTasks()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("created=%d", n)
	}
	task, err := repo.GetTaskBySource(model.WBSourceMaintenance, "v1")
	if err != nil || task == nil {
		t.Fatalf("task=%v err=%v", task, err)
	}
	if task.WorkDate != "2026-08-14" || task.DueDate != "2026-08-14" {
		t.Fatalf("%+v", task)
	}

	n2, err := repo.EnsureMaintenanceTasks()
	if err != nil {
		t.Fatal(err)
	}
	if n2 != 0 {
		t.Fatalf("duplicate create=%d", n2)
	}
}

func TestEnsureMaintenanceTasksCreatesCompletedVisit(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "ensure_done.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('C1','나성동도서관','나성동도서관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_plans (plan_id, plan_year, title, status)
		VALUES ('MP1', 2026, '2026', 'approved')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits
		(visit_id, plan_id, visit_date, customer_id, sort_order, entry_category, assignee, product_type, completed, completed_date)
		VALUES ('v-done','MP1','2026-08-10','C1',0,'normal','양기헌','앤로보틱스',1,'2026-08-10')`); err != nil {
		t.Fatal(err)
	}
	repo := NewWBRepo(db)
	n, err := repo.EnsureMaintenanceTasks()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("created=%d", n)
	}
	task, err := repo.GetTaskBySource(model.WBSourceMaintenance, "v-done")
	if err != nil || task == nil {
		t.Fatalf("task=%v err=%v", task, err)
	}
	if task.Status != model.WBTaskComplete || task.WorkDate != "2026-08-10" {
		t.Fatalf("%+v", task)
	}
}

func TestUnplaceKeepsSourceRow(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "unplace.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewWBRepo(db)
	t0 := &model.WorkTask{
		WorkType: model.WBWorkMaintenance, Title: "[점검]테스트",
		DueDate: "2026-08-14", WorkDate: "2026-08-14", StartTime: "09:00", EndTime: "09:30",
		DurationMin: 30, Status: model.WBTaskWaiting, Priority: model.WBPriorityNormal,
		SourceType: model.WBSourceMaintenance, SourceID: "v-keep",
	}
	if err := repo.CreateTask(t0); err != nil {
		t.Fatal(err)
	}
	if err := repo.UnplaceTask(t0.TaskID); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetTaskBySource(model.WBSourceMaintenance, "v-keep")
	if err != nil || got == nil {
		t.Fatalf("row missing: %v", err)
	}
	if got.WorkDate != "" || got.StartTime != "" {
		t.Fatalf("cleared? %+v", got)
	}
	if got.DueDate != "2026-08-14" {
		t.Fatalf("due_date should remain: %s", got.DueDate)
	}
}

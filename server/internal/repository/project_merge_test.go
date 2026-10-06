package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestProjectMergeMovesThenUndo(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "merge.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO work_projects (project_id, name, status) VALUES ('KEEP','남김','active'), ('DROP','없앰','active')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('CM','c','c',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, status, project_id)
		VALUES ('AS1','AS1','CM','2026-08-01','received','DROP')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO work_tasks (task_id, work_type, title, status, project_id)
		VALUES ('T1','admin','행정','waiting','DROP')`); err != nil {
		t.Fatal(err)
	}
	repo := NewProjectRepo(db)
	prev, err := repo.MergePreview("KEEP", "DROP")
	if err != nil || prev.AS != 1 || prev.Tasks != 1 {
		t.Fatalf("preview %+v err=%v", prev, err)
	}
	id, err := repo.MergeProjects("KEEP", "DROP", "backup-test")
	if err != nil || id == "" {
		t.Fatalf("merge err=%v id=%s", err, id)
	}
	drop, err := repo.Get("DROP")
	if err != nil || drop.Status != model.WBProjectArchived {
		t.Fatalf("drop status %+v err=%v", drop, err)
	}
	var pid string
	if err := db.QueryRow(`SELECT project_id FROM as_receipts WHERE as_id='AS1'`).Scan(&pid); err != nil || pid != "KEEP" {
		t.Fatalf("as project=%q err=%v", pid, err)
	}
	if err := repo.UndoMerge(id); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT project_id FROM as_receipts WHERE as_id='AS1'`).Scan(&pid); err != nil || pid != "DROP" {
		t.Fatalf("undo as=%q", pid)
	}
	drop, _ = repo.Get("DROP")
	if drop.Status != model.WBProjectActive {
		t.Fatalf("undo status=%s", drop.Status)
	}
}

func TestDuplicateProjectNameGroups(t *testing.T) {
	items := []model.WorkProject{
		{ProjectID: "A", Name: "같은이름", CustomerID: "C1", CustomerName: "고고객"},
		{ProjectID: "B", Name: "같은이름", CustomerID: "C1", CustomerName: "고고객"},
		{ProjectID: "C", Name: "다른이름", CustomerID: "C1"},
	}
	g := model.DuplicateProjectNameGroups(items)
	if len(g) != 1 || g[0].Count != 2 {
		t.Fatalf("%+v", g)
	}
}

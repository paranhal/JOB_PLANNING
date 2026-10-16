package repository

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectKindMigrationIdempotentAndExistingMaintenance(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "pk.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`INSERT INTO work_projects (project_id, name, status) VALUES ('WP-t1','기존사업','active')`); err != nil {
		t.Fatal(err)
	}
	applyProjectKindAssignmentsV72(db)
	applyProjectKindAssignmentsV72(db)

	var kind string
	if err := db.QueryRow(`SELECT project_kind FROM work_projects WHERE project_id='WP-t1'`).Scan(&kind); err != nil {
		t.Fatal(err)
	}
	if kind != "maintenance" {
		t.Fatalf("기존 행 kind=%s", kind)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='project_assignments'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("assignments n=%d err=%v", n, err)
	}

	_, down, err := projectKindMigrationFiles()
	if err != nil {
		t.Fatal(err)
	}
	if err := execSQLScript(db, down); err != nil {
		t.Fatal(err)
	}
	if workProjectHasColumn(db, "project_kind") {
		t.Fatal("down 후에도 project_kind")
	}
	applyProjectKindAssignmentsV72(db)
	if !workProjectHasColumn(db, "project_kind") {
		t.Fatal("재적용 후 컬럼 없음")
	}

	if _, err := db.Exec(`INSERT INTO work_projects (project_id, name, status, project_kind) VALUES ('WP-t2','납품잔여','active','supply')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_projects (project_id, name, status, project_kind) VALUES ('WP-t3','소모잔여','active','consumable')`); err != nil {
		t.Fatal(err)
	}
	applyProjectKindAssignmentsV72(db)
	var k2, k3 string
	_ = db.QueryRow(`SELECT project_kind FROM work_projects WHERE project_id='WP-t2'`).Scan(&k2)
	_ = db.QueryRow(`SELECT project_kind FROM work_projects WHERE project_id='WP-t3'`).Scan(&k3)
	if k2 != "build" || k3 != "other" {
		t.Fatalf("대응표 build=%s other=%s", k2, k3)
	}
	var nOther int
	if err := db.QueryRow(`SELECT COUNT(*) FROM codes WHERE code_id='PKC05'`).Scan(&nOther); err != nil || nOther != 1 {
		t.Fatalf("PKC05 n=%d err=%v", nOther, err)
	}
}

func TestProjectKindSQLFilesExist(t *testing.T) {
	up, down, err := projectKindMigrationFiles()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(up, "project_kind") || !strings.Contains(up, "project_assignments") || !strings.Contains(up, "PKC05") {
		t.Fatal("060 up")
	}
	if !strings.Contains(down, "DROP TABLE IF EXISTS project_assignments") {
		t.Fatal("060 down")
	}
}

package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestCountProjectActivitiesMapsWorkType(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "act.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	prepo := NewProjectRepo(db)
	wb := NewWBRepo(db)
	p := &model.WorkProject{Name: "활동사업", PlanYear: 2026, Status: model.WBProjectActive, ContractAmount: 1, ProjectKind: model.ProjectKindBuild, ProjectKindSet: 1}
	if err := prepo.Create(p); err != nil {
		t.Fatal(err)
	}
	tk := &model.WorkTask{WorkType: model.WBWorkAdmin, ProjectID: p.ProjectID, Title: "admin", Status: model.WBTaskWaiting, DueDate: "2026-10-20", WorkDate: "2026-10-19"}
	if err := wb.CreateTask(tk); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks (task_id, work_type, project_id, title, status, due_date, work_date, org_id)
		VALUES ('WT-AS1','as',?,'as','waiting','2026-10-20','2026-10-19','ORG-LIB'),
		       ('WT-M1','maintenance',?,'mnt','waiting','2026-10-20','2026-10-19','ORG-LIB'),
		       ('WT-R1','replace',?,'rep','waiting','2026-10-20','2026-10-19','ORG-LIB')`,
		p.ProjectID, p.ProjectID, p.ProjectID); err != nil {
		t.Fatal(err)
	}
	orphan := &model.WorkTask{WorkType: model.WBWorkSupport, Title: "미지정", Status: model.WBTaskWaiting, DueDate: "2026-10-20", WorkDate: "2026-10-19"}
	if err := wb.CreateTask(orphan); err != nil {
		t.Fatal(err)
	}
	got, err := prepo.CountProjectActivities()
	if err != nil {
		t.Fatal(err)
	}
	if got.AS[p.ProjectID] != 2 || got.Mnt[p.ProjectID] != 1 || got.Wait[p.ProjectID] != 1 {
		t.Fatalf("counts as=%d mnt=%d wait=%d", got.AS[p.ProjectID], got.Mnt[p.ProjectID], got.Wait[p.ProjectID])
	}
	if got.Unspec < 1 {
		t.Fatal("미지정")
	}
}

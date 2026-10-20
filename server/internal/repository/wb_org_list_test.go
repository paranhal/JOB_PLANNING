package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestListTasksBetweenForOrgIsolatesAndLegacyAssignee(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "wb-org.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order)
		VALUES('O03','O03','SI사업팀','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	users := NewUserRepo(db)
	u01 := &model.User{Username: "lib-t", PasswordHash: "x", FullName: "도서관담당A", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	u03 := &model.User{Username: "si-t", PasswordHash: "x", FullName: "SI담당C", Role: model.RoleTech, IsActive: true, OrgID: "O03", ProfileDone: true}
	if err := users.Create(u01); err != nil {
		t.Fatal(err)
	}
	if err := users.Create(u03); err != nil {
		t.Fatal(err)
	}
	wb := NewWBRepo(db)
	t01 := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "도서관일정O01", OrgID: model.OrgIDLibrary, WorkDate: "2026-10-19", Status: model.WBTaskWaiting, Assignee: u01.FullName, AssigneeUserID: u01.UserID}
	t03 := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "SI일정O03", OrgID: "O03", WorkDate: "2026-10-19", Status: model.WBTaskWaiting, Assignee: u03.FullName, AssigneeUserID: u03.UserID}
	legacy := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "레거시담당O03", OrgID: model.OrgIDLibrary, WorkDate: "2026-10-19", Status: model.WBTaskWaiting, Assignee: u03.FullName, AssigneeUserID: u03.UserID}
	unknown := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "소속불명", OrgID: model.OrgIDLibrary, WorkDate: "2026-10-19", Status: model.WBTaskWaiting}
	for _, tsk := range []*model.WorkTask{t01, t03, legacy, unknown} {
		if err := wb.CreateTask(tsk); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`UPDATE work_tasks SET org_id='' WHERE task_id=?`, legacy.TaskID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE work_tasks SET org_id='', assignee='', assignee_user_id='', created_by_user_id='' WHERE task_id=?`, unknown.TaskID); err != nil {
		t.Fatal(err)
	}

	o03, err := wb.ListTasksBetweenForOrg("2026-10-19", "2026-10-19", "O03")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, x := range o03 {
		got[x.Title] = true
	}
	if !got["SI일정O03"] || !got["레거시담당O03"] {
		t.Fatalf("O03 목록 %+v", titles(o03))
	}
	if got["도서관일정O01"] || got["소속불명"] {
		t.Fatalf("O03에 타 조직/불명 %+v", titles(o03))
	}

	o01, err := wb.ListTasksBetweenForOrg("2026-10-19", "2026-10-19", model.OrgIDLibrary)
	if err != nil {
		t.Fatal(err)
	}
	got01 := map[string]bool{}
	for _, x := range o01 {
		got01[x.Title] = true
	}
	if !got01["도서관일정O01"] {
		t.Fatalf("O01 목록 %+v", titles(o01))
	}
	if got01["SI일정O03"] || got01["레거시담당O03"] {
		t.Fatalf("O01에 O03 %+v", titles(o01))
	}

	all, err := wb.ListTasksBetweenForOrg("2026-10-19", "2026-10-19", OrgAll)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) < 3 {
		t.Fatalf("전체 %d", len(all))
	}

	undated := &model.WorkTask{WorkType: model.WBWorkAS, Title: "미정O01", OrgID: model.OrgIDLibrary, SourceType: model.WBSourceAS, SourceID: "AS-O01", Status: model.WBTaskWaiting, Assignee: u01.FullName, AssigneeUserID: u01.UserID}
	if err := wb.CreateTask(undated); err != nil {
		t.Fatal(err)
	}
	u03list, err := wb.ListUndatedASTasksForOrg("O03")
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range u03list {
		if x.Title == "미정O01" {
			t.Fatal("O03 날짜미정에 O01")
		}
	}
}

func titles(tasks []model.WorkTask) []string {
	out := make([]string, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, t.Title)
	}
	return out
}

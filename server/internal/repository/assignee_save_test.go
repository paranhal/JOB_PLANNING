package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestSaveAssigneesASReceiptAndTask(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "asg.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ApplyAssigneePickerV91ForTest(db)
	users := NewUserRepo(db)
	users.EnsureAdmin("x")
	u := &model.User{Username: "asg1", PasswordHash: "x", FullName: "김담당", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	if err := users.Create(u); err != nil {
		t.Fatal(err)
	}
	u2 := &model.User{Username: "asg2", PasswordHash: "x", FullName: "이보조", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	if err := users.Create(u2); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers(customer_id, org_name, official_name, is_active, org_id) VALUES ('C1','학교','학교',1,?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts(as_id, as_number, receipt_datetime, customer_id, symptom, status, assigned_to, org_id, created_at, updated_at)
		VALUES ('AS1','AS-1',datetime('now'),'C1','증','received','', ?, datetime('now'), datetime('now'))`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	people := []AssigneePerson{
		{UserID: u.UserID, Name: u.FullName, Role: model.WBMemberOwner},
		{UserID: u2.UserID, Name: u2.FullName, Role: model.WBMemberSupport},
	}
	if err := SaveAssignees(db, AssigneeTargetASReceipt, "AS1", people, ""); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM as_receipt_assignees WHERE as_id='AS1'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("rows %d", n)
	}
	var name, uid string
	if err := db.QueryRow(`SELECT assigned_to, COALESCE(assigned_user_id,'') FROM as_receipts WHERE as_id='AS1'`).Scan(&name, &uid); err != nil {
		t.Fatal(err)
	}
	if name != u.FullName || uid != u.UserID {
		t.Fatalf("mirror %s %s", name, uid)
	}
	if got := FormatAssigneesLabel(people); got != "김담당 외 1명" {
		t.Fatalf("label %q", got)
	}

	wb := NewWBRepo(db)
	task := &model.WorkTask{Title: "t", WorkType: model.WBWorkAdmin, OrgID: model.OrgIDLibrary, Assignee: u.FullName, WorkDate: "2026-10-26", DueDate: "2026-10-26"}
	if err := wb.CreateTask(task); err != nil {
		t.Fatal(err)
	}
	if err := SaveAssignees(db, AssigneeTargetWorkTask, task.TaskID, people, ""); err != nil {
		t.Fatal(err)
	}
	ms, err := wb.ListMembers(task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) < 2 {
		t.Fatalf("members %d", len(ms))
	}
}

func TestSQLAssignedToUserDelegates(t *testing.T) {
	expr, args := SQLAssignedToUser(AssigneeKindAS, "a", []string{"김담당"})
	if expr == "" || len(args) == 0 {
		t.Fatalf("empty sql")
	}
}

func TestBackfillAssigneePickerNoNameGuess(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "bf.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers(customer_id, org_name, official_name, is_active, org_id) VALUES ('C1','학교','학교',1,?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts(as_id, as_number, receipt_datetime, customer_id, symptom, status, assigned_to, assigned_user_id, org_id, created_at, updated_at)
		VALUES ('ASB','AS-B',datetime('now'),'C1','증','received','외부협력사','', ?, datetime('now'), datetime('now'))`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	ApplyAssigneePickerV91ForTest(db)
	var uid, name string
	if err := db.QueryRow(`SELECT user_id, assignee_name FROM as_receipt_assignees WHERE as_id='ASB'`).Scan(&uid, &name); err != nil {
		t.Fatal(err)
	}
	if uid != "" {
		t.Fatalf("이름만 있는 건을 ID로 추정하면 안 됨: %s", uid)
	}
	if name != "외부협력사" {
		t.Fatalf("name %s", name)
	}
}

package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestStaffLeaveUnique(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "leave.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	u := &model.User{Username: "yang", PasswordHash: "x", FullName: "양기헌", Role: model.RoleTech, IsActive: true}
	if err := NewUserRepo(db).Create(u); err != nil {
		t.Fatal(err)
	}
	repo := NewStaffLeaveRepo(db)
	if err := repo.Create(model.StaffLeave{UserID: u.UserID, UserName: u.FullName, Date: "2026-08-21", Kind: model.LeaveKindAnnual}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(model.StaffLeave{UserID: u.UserID, Date: "2026-08-21", Kind: model.LeaveKindHalfAM}); err == nil {
		t.Fatal("UNIQUE(user_id, leave_date) 가 막혀야 함")
	}
}

func TestAssigneeForVisitSkipsLeave(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "asg.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('c1','도서관','도서관',1)`); err != nil {
		t.Fatal(err)
	}
	u := &model.User{Username: "yang", PasswordHash: "x", FullName: "양기헌", Role: model.RoleTech, IsActive: true}
	if err := NewUserRepo(db).Create(u); err != nil {
		t.Fatal(err)
	}
	mnt := NewMaintenanceRepo(db)
	plan, err := mnt.CreatePlan(2026, "테스트")
	if err != nil {
		t.Fatal(err)
	}
	if err := mnt.InsertVisitFull(model.MaintenanceVisit{
		PlanID: plan.PlanID, VisitDate: "2026-01-06", CustomerID: "c1", ProductType: "KLAS", Assignee: "양기헌",
	}); err != nil {
		t.Fatal(err)
	}
	if got := mnt.AssigneeForVisit(plan.PlanID, "c1", "KLAS", "2026-01-05"); got != "양기헌" {
		t.Fatalf("연차 전 담당자=%q", got)
	}
	if err := NewStaffLeaveRepo(db).Create(model.StaffLeave{
		UserID: u.UserID, UserName: u.FullName, Date: "2026-01-05", Kind: model.LeaveKindAnnual,
	}); err != nil {
		t.Fatal(err)
	}
	if got := mnt.AssigneeForVisit(plan.PlanID, "c1", "KLAS", "2026-01-05"); got != "" {
		t.Fatalf("연차인 사람에게 배정되면 안 됨 got=%q", got)
	}
	if got := mnt.AssigneeForVisit(plan.PlanID, "c1", "KLAS", "2026-01-06"); got != "양기헌" {
		t.Fatalf("연차 아닌 날 담당자=%q", got)
	}
}

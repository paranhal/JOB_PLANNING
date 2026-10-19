package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestProjectAssignmentsMultiDutyUniqueAndDeletedReject(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "pa.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := NewUserRepo(db)
	if err := users.EnsureAdmin("x"); err != nil {
		t.Fatal(err)
	}
	staff := &model.User{Username: "staff1", PasswordHash: "x", FullName: "직원", Role: model.RoleTech, IsActive: true, OrgID: "O01"}
	if err := users.Create(staff); err != nil {
		t.Fatal(err)
	}
	gone := &model.User{Username: "gone1", PasswordHash: "x", FullName: "삭제자", Role: model.RoleTech, IsActive: true, OrgID: "O01"}
	if err := users.Create(gone); err != nil {
		t.Fatal(err)
	}
	if err := users.DeleteUserAccount(gone.UserID); err != nil {
		t.Fatal(err)
	}

	repo := NewProjectRepo(db)
	p := &model.WorkProject{Name: "배정사업", Status: model.WBProjectActive, ProjectKind: model.ProjectKindConstruction}
	if err := repo.Create(p); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.Get(p.ProjectID)
	if got.ProjectKind != model.ProjectKindConstruction {
		t.Fatalf("kind=%s", got.ProjectKind)
	}

	err = repo.ReplaceAssignments(p.ProjectID, staff.UserID, []model.ProjectAssignment{
		{UserID: staff.UserID, Duty: model.ProjectDutySales},
		{UserID: staff.UserID, Duty: model.ProjectDutyExecution},
	})
	if err != nil {
		t.Fatal(err)
	}
	list, err := repo.ListAssignments(p.ProjectID)
	if err != nil || len(list) != 2 {
		t.Fatalf("len=%d err=%v", len(list), err)
	}

	peer := &model.User{Username: "staff2", PasswordHash: "x", FullName: "동료", Role: model.RoleTech, IsActive: true, OrgID: "O01"}
	if err := users.Create(peer); err != nil {
		t.Fatal(err)
	}
	err = repo.ReplaceAssignments(p.ProjectID, staff.UserID, []model.ProjectAssignment{
		{UserID: staff.UserID, Duty: model.ProjectDutyExecution},
		{UserID: peer.UserID, Duty: model.ProjectDutyExecution},
	})
	if err != nil {
		t.Fatal(err)
	}
	same, err := repo.ListAssignments(p.ProjectID)
	if err != nil || len(same) != 2 {
		t.Fatalf("같은 역할 여러 명 len=%d err=%v", len(same), err)
	}

	err = repo.ReplaceAssignments(p.ProjectID, staff.UserID, []model.ProjectAssignment{
		{UserID: gone.UserID, Duty: model.ProjectDutySales},
	})
	if err == nil {
		t.Fatal("삭제된 사용자 배정이 통과했다")
	}

	keep := &model.WorkProject{Name: "남김", Status: model.WBProjectActive, ProjectKind: model.ProjectKindMaintenance}
	drop := &model.WorkProject{Name: "없앰", Status: model.WBProjectActive, ProjectKind: model.ProjectKindMaintenance}
	if err := repo.Create(keep); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(drop); err != nil {
		t.Fatal(err)
	}
	_ = repo.ReplaceAssignments(keep.ProjectID, "", []model.ProjectAssignment{
		{UserID: staff.UserID, Duty: model.ProjectDutySales},
	})
	_ = repo.ReplaceAssignments(drop.ProjectID, "", []model.ProjectAssignment{
		{UserID: staff.UserID, Duty: model.ProjectDutySales},
		{UserID: staff.UserID, Duty: model.ProjectDutyAccounting},
	})
	if _, err := repo.MergeProjects(keep.ProjectID, drop.ProjectID, "t"); err != nil {
		t.Fatal(err)
	}
	after, _ := repo.ListAssignments(keep.ProjectID)
	if len(after) != 2 {
		t.Fatalf("merge assignments=%d", len(after))
	}
}

func TestProjectKindRequiredAtHandlerLayerDocumentedByEmptyCreateDefault(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "pk2.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewProjectRepo(db)
	p := &model.WorkProject{Name: "기본유형", Status: model.WBProjectActive}
	if err := repo.Create(p); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.Get(p.ProjectID)
	if got.ProjectKindSet != 0 {
		t.Fatalf("미선택 생성은 미확정이어야 한다 kind=%s set=%d", got.ProjectKind, got.ProjectKindSet)
	}
}

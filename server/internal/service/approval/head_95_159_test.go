package approval

import (
	"testing"

	"customer-support/internal/model"
)

func TestOrgApproverHeadFirstThenSameOrgAdmin(t *testing.T) {
	e, users, head, _, _ := testEngine(t)
	admin := &model.User{Username: "fallback-admin", PasswordHash: "x", FullName: "가관리자", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01"}
	if err := users.Create(admin); err != nil {
		t.Fatal(err)
	}
	got, err := e.OrgApprover("O01")
	if err != nil || got != head {
		t.Fatalf("explicit head priority got=%q err=%v", got, err)
	}
	if err := e.ClearOrgHead(actor(head, "O01", model.RoleOrgAdmin), "O01"); err != nil {
		t.Fatal(err)
	}
	got, err = e.OrgApprover("O01")
	if err != nil || got != admin.UserID {
		t.Fatalf("fallback got=%q want=%q err=%v", got, admin.UserID, err)
	}
}

func TestSetAndClearOrgHeadRejectCrossOrg(t *testing.T) {
	e, users, head, _, _ := testEngine(t)
	other := &model.User{Username: "other-admin", PasswordHash: "x", FullName: "타조직", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O02"}
	if err := users.Create(other); err != nil {
		t.Fatal(err)
	}
	admin := actor(head, "O01", model.RoleOrgAdmin)
	if err := e.SetOrgHead(admin, other.UserID, "O01"); err == nil {
		t.Fatal("cross-org target accepted")
	}
	if err := e.ClearOrgHead(admin, "O02"); err == nil {
		t.Fatal("cross-org clear accepted")
	}
	if err := e.ClearOrgHead(admin, "O01"); err != nil {
		t.Fatal(err)
	}
	if got, err := e.OrgHead("O01"); err != nil || got != "" {
		t.Fatalf("head=%q err=%v", got, err)
	}
}

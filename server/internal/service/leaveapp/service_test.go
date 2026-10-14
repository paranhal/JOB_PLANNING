package leaveapp

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/repository"
	"customer-support/internal/service/approval"
	"customer-support/internal/service/leavecalc"
)

func testSvc(t *testing.T) (*Service, string, string) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "lv.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	head := &model.User{Username: "head1", PasswordHash: "x", FullName: "팀장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01"}
	staff := &model.User{Username: "staff1", PasswordHash: "x", FullName: "직원", Role: model.RoleTech, IsActive: true, OrgID: "O01"}
	for _, u := range []*model.User{head, staff} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	eng := &approval.Engine{DB: db, Now: func() time.Time { return time.Date(2026, 8, 7, 10, 0, 0, 0, time.Local) }}
	svc := New(db, eng)
	svc.Now = eng.Now
	eng.Hooks = svc
	if err := eng.SetOrgHead(approval.Actor{UserID: head.UserID, Role: model.RoleOrgAdmin}, head.UserID, "O01"); err != nil {
		t.Fatal(err)
	}
	return svc, head.UserID, staff.UserID
}

func TestStaffLeaveSubmitPendingAndHWPX(t *testing.T) {
	svc, head, staff := testSvc(t)
	id, err := svc.Submit(approval.Actor{UserID: staff, OrgID: "O01", Role: model.RoleTech, Name: "직원"}, Input{
		TypeKey: leavecalc.TypeAnnual, PeriodMode: "days",
		StartDate: "2026-08-10", EndDate: "2026-08-10", Reason: "개인 사정",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.Get(id)
	if err != nil || got == nil || got.Status != "pending" || !strings.Contains(got.DocNo, "근태") {
		t.Fatalf("%+v err=%v", got, err)
	}
	inbox, err := svc.Eng.ListInbox("O01", head, "2026-08-07")
	if err != nil || len(inbox) != 1 {
		t.Fatalf("inbox %d %v", len(inbox), err)
	}
	if err := svc.Eng.Approve(approval.Actor{UserID: head, OrgID: "O01", Role: model.RoleOrgAdmin}, inbox[0].RequestID, "", ""); err != nil {
		t.Fatal(err)
	}
	got, _ = svc.Get(id)
	if got.Status != "approved" {
		t.Fatalf("status %s", got.Status)
	}
	var n int
	_ = svc.DB.QueryRow(`SELECT COUNT(*) FROM staff_leaves WHERE source_request_id=?`, id).Scan(&n)
	if n != 1 {
		t.Fatalf("staff_leaves %d", n)
	}
	doc, name, err := svc.FillHWPX(id)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(name, ".hwpx") || len(doc) < 100 {
		t.Fatalf("hwpx %s %d", name, len(doc))
	}
}

func TestHeadSelfApproveAndOverlap(t *testing.T) {
	svc, head, _ := testSvc(t)
	id, err := svc.Submit(approval.Actor{UserID: head, OrgID: "O01", Role: model.RoleOrgAdmin, Name: "팀장"}, Input{
		TypeKey: leavecalc.TypeAnnual, PeriodMode: "days",
		StartDate: "2026-08-11", EndDate: "2026-08-11", Reason: "자기 결재",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Get(id)
	if got.Status != "approved" {
		t.Fatalf("self %s", got.Status)
	}
	_, err = svc.Submit(approval.Actor{UserID: head, OrgID: "O01", Role: model.RoleOrgAdmin, Name: "팀장"}, Input{
		TypeKey: leavecalc.TypeAnnual, PeriodMode: "days",
		StartDate: "2026-08-11", EndDate: "2026-08-11", Reason: "겹침",
	})
	if err == nil {
		t.Fatal("overlap")
	}
}

func TestDeputyOptionalAndHalf(t *testing.T) {
	svc, _, staff := testSvc(t)
	id, err := svc.Submit(approval.Actor{UserID: staff, OrgID: "O01", Role: model.RoleTech, Name: "직원"}, Input{
		TypeKey: leavecalc.TypeHalf, PeriodMode: "hours", HalfPart: "am",
		StartDate: "2026-08-12", EndDate: "2026-08-12", Reason: "반차",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Get(id)
	if got.DeputyUserID != "" || got.WorkMinutes != leavecalc.DefaultHalfDayMinutes {
		t.Fatalf("%+v", got)
	}
}

func TestPolicyCRequiresDeputy(t *testing.T) {
	svc, head, staff := testSvc(t)
	p, _ := svc.Eng.GetPolicy("O01")
	p.DelegWithLeave = true
	if err := svc.Eng.SavePolicy(approval.Actor{UserID: head, OrgID: "O01", Role: model.RoleOrgAdmin}, "O01", p); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Submit(approval.Actor{UserID: head, OrgID: "O01", Role: model.RoleOrgAdmin, Name: "팀장"}, Input{
		TypeKey: leavecalc.TypeAnnual, PeriodMode: "days",
		StartDate: "2026-08-13", EndDate: "2026-08-13", Reason: "팀장휴가",
	})
	if err == nil {
		t.Fatal("deputy required")
	}
	id, err := svc.Submit(approval.Actor{UserID: head, OrgID: "O01", Role: model.RoleOrgAdmin, Name: "팀장"}, Input{
		TypeKey: leavecalc.TypeAnnual, PeriodMode: "days",
		StartDate: "2026-08-13", EndDate: "2026-08-13", Reason: "팀장휴가",
		ApproverDeputyUserID: staff,
	})
	if err != nil {
		t.Fatal(err)
	}
	var n int
	_ = svc.DB.QueryRow(`SELECT COUNT(*) FROM approval_delegations WHERE leave_req_id=?`, id).Scan(&n)
	if n != 1 {
		t.Fatalf("delegation %d", n)
	}
}

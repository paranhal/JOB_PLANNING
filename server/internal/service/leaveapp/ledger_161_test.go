package leaveapp

import (
	"errors"
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/service/approval"
	"customer-support/internal/service/leavecalc"
)

func Test95_161_UnregisteredSubmitNoLedger(t *testing.T) {
	svc, _, staff := testSvc(t)
	id, err := svc.Submit(approval.Actor{UserID: staff, OrgID: "O01", Role: model.RoleTech, Name: "직원"}, Input{
		TypeKey: leavecalc.TypeAnnual, PeriodMode: "days",
		StartDate: "2026-08-10", EndDate: "2026-08-10", Reason: "미등록",
	})
	if err != nil {
		t.Fatal(err)
	}
	if svc.ledger().GrantCount(staff) != 0 {
		t.Fatal("미등록 원장")
	}
	got, _ := svc.Get(id)
	if got.DeductMinutes != 0 || got.AbsenceMinutes != 0 {
		t.Fatalf("%+v", got)
	}
}

func Test95_161_ShortageAbsenceAndCancel(t *testing.T) {
	svc, head, staff := testSvc(t)
	if err := svc.ledger().Enable(staff, "O01", ""); err != nil {
		t.Fatal(err)
	}
	if err := svc.ledger().AddOpening(staff, "2026-01-01", "2026-12-31", 480, "1"); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Submit(approval.Actor{UserID: staff, OrgID: "O01", Role: model.RoleTech}, Input{
		TypeKey: leavecalc.TypeAnnual, PeriodMode: "days",
		StartDate: "2026-08-10", EndDate: "2026-08-14", Reason: "5일",
	})
	if !errors.Is(err, leavecalc.ErrAbsenceConsent) {
		t.Fatalf("want consent got %v", err)
	}
	id, err := svc.Submit(approval.Actor{UserID: staff, OrgID: "O01", Role: model.RoleTech}, Input{
		TypeKey: leavecalc.TypeAnnual, PeriodMode: "days",
		StartDate: "2026-08-10", EndDate: "2026-08-14", Reason: "5일동의", AbsenceConsent: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Get(id)
	if got.DeductMinutes != 15*480 && got.AbsenceMinutes == 0 {
		t.Fatalf("plan %+v", got)
	}
	if got.AbsenceMinutes <= 0 || got.DeductMinutes+got.AbsenceMinutes != got.WorkMinutes {
		t.Fatalf("split %+v", got)
	}
	inbox, err := svc.Eng.ListInbox("O01", head, "2026-08-07")
	if err != nil || len(inbox) == 0 {
		t.Fatal(err)
	}
	if err := svc.Eng.Approve(approval.Actor{UserID: head, OrgID: "O01", Role: model.RoleOrgAdmin}, inbox[0].RequestID, "", ""); err != nil {
		t.Fatal(err)
	}
	if svc.ledger().AbsenceNet(id) != got.AbsenceMinutes {
		t.Fatalf("abs %d want %d", svc.ledger().AbsenceNet(id), got.AbsenceMinutes)
	}
	if err := svc.CancelApproved(id); err != nil {
		t.Fatal(err)
	}
	if svc.ledger().AbsenceNet(id) != 0 || svc.ledger().UseNet(id) != 0 {
		t.Fatalf("cancel abs=%d use=%d", svc.ledger().AbsenceNet(id), svc.ledger().UseNet(id))
	}
}

func Test95_161_MilitaryNoDeduct(t *testing.T) {
	svc, head, staff := testSvc(t)
	if err := svc.ledger().Enable(staff, "O01", "2026-01-01"); err != nil {
		t.Fatal(err)
	}
	id, err := svc.Submit(approval.Actor{UserID: staff, OrgID: "O01", Role: model.RoleTech}, Input{
		TypeKey: leavecalc.TypeMilitaryTraining, PeriodMode: "days",
		StartDate: "2026-08-10", EndDate: "2026-08-10", Reason: "예비군",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Get(id)
	if got.DeductMinutes != 0 || got.AbsenceMinutes != 0 {
		t.Fatalf("%+v", got)
	}
	inbox, _ := svc.Eng.ListInbox("O01", head, "2026-08-07")
	if err := svc.Eng.Approve(approval.Actor{UserID: head, OrgID: "O01", Role: model.RoleOrgAdmin}, inbox[0].RequestID, "", ""); err != nil {
		t.Fatal(err)
	}
	if svc.ledger().UseNet(id) != 0 || svc.ledger().AbsenceNet(id) != 0 {
		t.Fatal("military ledger")
	}
}

func Test95_161_RejectWithdrawNoConsume(t *testing.T) {
	svc, head, staff := testSvc(t)
	if err := svc.ledger().Enable(staff, "O01", "2026-01-01"); err != nil {
		t.Fatal(err)
	}
	id, err := svc.Submit(approval.Actor{UserID: staff, OrgID: "O01", Role: model.RoleTech}, Input{
		TypeKey: leavecalc.TypeAnnual, PeriodMode: "days",
		StartDate: "2026-08-10", EndDate: "2026-08-10", Reason: "회수",
	})
	if err != nil {
		t.Fatal(err)
	}
	req, _ := svc.Get(id)
	if err := svc.Eng.Withdraw(approval.Actor{UserID: staff, OrgID: "O01", Role: model.RoleTech}, req.ApprovalRequestID); err != nil {
		t.Fatal(err)
	}
	_ = svc.SyncStatus(id, "withdrawn")
	if svc.ledger().UseNet(id) != 0 {
		t.Fatal("withdraw consume")
	}
	id2, err := svc.Submit(approval.Actor{UserID: staff, OrgID: "O01", Role: model.RoleTech}, Input{
		TypeKey: leavecalc.TypeAnnual, PeriodMode: "days",
		StartDate: "2026-08-11", EndDate: "2026-08-11", Reason: "반려",
	})
	if err != nil {
		t.Fatal(err)
	}
	req2, _ := svc.Get(id2)
	inbox, _ := svc.Eng.ListInbox("O01", head, "2026-08-07")
	var ar string
	for _, it := range inbox {
		if it.RequestID == req2.ApprovalRequestID {
			ar = it.RequestID
		}
	}
	if ar == "" {
		t.Fatal("inbox")
	}
	if err := svc.Eng.Reject(approval.Actor{UserID: head, OrgID: "O01", Role: model.RoleOrgAdmin}, ar, "불가"); err != nil {
		t.Fatal(err)
	}
	_ = svc.SyncStatus(id2, "rejected")
	if svc.ledger().UseNet(id2) != 0 {
		t.Fatal("reject consume")
	}
}

func Test95_161_ApproveAtomic(t *testing.T) {
	svc, head, staff := testSvc(t)
	if err := svc.ledger().Enable(staff, "O01", "2026-01-01"); err != nil {
		t.Fatal(err)
	}
	id, err := svc.Submit(approval.Actor{UserID: staff, OrgID: "O01", Role: model.RoleTech}, Input{
		TypeKey: leavecalc.TypeAnnual, PeriodMode: "days",
		StartDate: "2026-08-10", EndDate: "2026-08-10", Reason: "원자",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Get(id)
	tx, err := svc.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ledger().ConsumeApproved(tx, staff, id, "2026-08-10", got.DeductMinutes); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if svc.ledger().UseNet(id) != 0 {
		t.Fatal("rollback leaked")
	}
	inbox, _ := svc.Eng.ListInbox("O01", head, "2026-08-07")
	if err := svc.Eng.Approve(approval.Actor{UserID: head, OrgID: "O01", Role: model.RoleOrgAdmin}, inbox[0].RequestID, "", ""); err != nil {
		t.Fatal(err)
	}
	if svc.ledger().UseNet(id) != got.DeductMinutes {
		t.Fatalf("after approve %d", svc.ledger().UseNet(id))
	}
}

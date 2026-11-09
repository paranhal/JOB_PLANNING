package leaveapp

import (
	"strings"
	"testing"

	"customer-support/internal/hwpx"
	"customer-support/internal/model"
	"customer-support/internal/repository"
	"customer-support/internal/service/approval"
	"customer-support/internal/service/leavecalc"
)

func TestFormatStampDateTimeKSTMinutes(t *testing.T) {
	if got := FormatStampDateTime("2026-08-07 10:05:59"); got != "2026-08-07 10:05" {
		t.Fatalf("naive=%q", got)
	}
	if got := FormatStampDateTime("2026-08-07T01:00:00Z"); got != "2026-08-07 10:00" {
		t.Fatalf("utc=%q", got)
	}
	if FormatStampDateTime("") != "" {
		t.Fatal("empty")
	}
}

func Test95_160_StampTimesFourApprovePaths(t *testing.T) {
	svc, head, staff := testSvc(t)
	want := "2026-08-07 10:00"
	staffAct := approval.Actor{UserID: staff, OrgID: "O01", Role: model.RoleTech, Name: "직원"}
	headAct := approval.Actor{UserID: head, OrgID: "O01", Role: model.RoleOrgAdmin, Name: "팀장"}

	id, err := svc.Submit(staffAct, Input{
		TypeKey: leavecalc.TypeAnnual, PeriodMode: "days",
		StartDate: "2026-08-10", EndDate: "2026-08-10", Reason: "일반승인일시",
	})
	if err != nil {
		t.Fatal(err)
	}
	pending := svc.StampCells(mustLeave(t, svc, id))
	if pending[2].Empty == false || pending[2].ActedAt != "" {
		t.Fatalf("결재 중 승인 %+v", pending[2])
	}
	if pending[0].ActedAt != want {
		t.Fatalf("작성 일시 %+v", pending[0])
	}
	inbox, err := svc.Eng.ListInbox("O01", head, "2026-08-07")
	if err != nil || len(inbox) == 0 {
		t.Fatalf("inbox %v", err)
	}
	if err := svc.Eng.Approve(headAct, inbox[0].RequestID, "", ""); err != nil {
		t.Fatal(err)
	}
	assertStampHWPX(t, svc, id, "팀장", want, "")

	users := repository.NewUserRepo(svc.DB)
	dep := &model.User{Username: "dep160", PasswordHash: "x", FullName: "대리인", Role: model.RoleTech, IsActive: true, OrgID: "O01"}
	if err := users.Create(dep); err != nil {
		t.Fatal(err)
	}
	delID, err := svc.Eng.CreateDelegation(headAct, "O01", head, dep.UserID, "2026-08-01", "2026-08-31", approval.SrcHead, "")
	if err != nil {
		t.Fatal(err)
	}
	id2, err := svc.Submit(staffAct, Input{
		TypeKey: leavecalc.TypeAnnual, PeriodMode: "days",
		StartDate: "2026-08-11", EndDate: "2026-08-11", Reason: "대결승인일시",
	})
	if err != nil {
		t.Fatal(err)
	}
	din, err := svc.Eng.ListInbox("O01", dep.UserID, "2026-08-07")
	if err != nil || len(din) == 0 {
		t.Fatalf("대결 inbox %v", err)
	}
	if err := svc.Eng.Approve(approval.Actor{UserID: dep.UserID, OrgID: "O01", Role: model.RoleTech, Name: "대리인"}, din[0].RequestID, "", ""); err != nil {
		t.Fatal(err)
	}
	assertStampHWPX(t, svc, id2, "대리인", want, "(대결)")

	selfID, err := svc.Submit(headAct, Input{
		TypeKey: leavecalc.TypeAnnual, PeriodMode: "days",
		StartDate: "2026-08-12", EndDate: "2026-08-12", Reason: "자기결재일시",
	})
	if err != nil {
		t.Fatal(err)
	}
	self := svc.StampCells(mustLeave(t, svc, selfID))
	if self[0].Name != "팀장" || self[2].Name != "팀장" || self[2].ActedAt != want {
		t.Fatalf("자기 결재 %+v", self)
	}
	doc, _, err := svc.FillHWPX(selfID)
	if err != nil {
		t.Fatal(err)
	}
	sec := hwpxSection0(t, doc)
	if strings.Count(sec, want) < 2 {
		t.Fatalf("자기 결재 HWPX 일시 %s", sec)
	}

	if err := svc.Eng.RevokeDelegation(headAct, delID); err != nil {
		t.Fatal(err)
	}
	p, err := svc.Eng.GetPolicy("O01")
	if err != nil {
		t.Fatal(err)
	}
	p.AdminEmergencyApprove = true
	if err := svc.Eng.SavePolicy(headAct, "O01", p); err != nil {
		t.Fatal(err)
	}
	mgr := &model.User{Username: "mgr160", PasswordHash: "x", FullName: "관리자", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01"}
	if err := users.Create(mgr); err != nil {
		t.Fatal(err)
	}
	id3, err := svc.Submit(staffAct, Input{
		TypeKey: leavecalc.TypeAnnual, PeriodMode: "days",
		StartDate: "2026-08-13", EndDate: "2026-08-13", Reason: "긴급승인일시",
	})
	if err != nil {
		t.Fatal(err)
	}
	var arID string
	_ = svc.DB.QueryRow(`SELECT approval_request_id FROM leave_requests WHERE leave_req_id=?`, id3).Scan(&arID)
	if err := svc.Eng.Approve(approval.Actor{UserID: mgr.UserID, OrgID: "O01", Role: model.RoleOrgAdmin, Name: "관리자"}, arID, "", "팀장 출장"); err != nil {
		t.Fatal(err)
	}
	assertStampHWPX(t, svc, id3, "관리자", want, "(대결)")
}

func assertStampHWPX(t *testing.T, svc *Service, id, name, at, mark string) {
	t.Helper()
	cells := svc.StampCells(mustLeave(t, svc, id))
	if cells[2].Empty || cells[2].Name != name || cells[2].ActedAt != at || cells[2].Mark != mark {
		t.Fatalf("화면 승인 %+v want name=%s at=%s mark=%q", cells[2], name, at, mark)
	}
	if cells[1].Empty == false {
		t.Fatal("검토란")
	}
	doc, _, err := svc.FillHWPX(id)
	if err != nil {
		t.Fatal(err)
	}
	sec := hwpxSection0(t, doc)
	if strings.Contains(sec, "{{") {
		t.Fatal("자리표시자")
	}
	if !strings.Contains(sec, at) {
		t.Fatalf("HWPX 일시 없음 %s", at)
	}
	if mark != "" && !strings.Contains(sec, mark) {
		t.Fatalf("HWPX 표기 없음 %s", mark)
	}
	if !strings.Contains(sec, name) {
		t.Fatalf("HWPX 이름 없음 %s", name)
	}
}

func Test95_160_HWPXPermitDateHasMinutes(t *testing.T) {
	vals := hwpx.LeavePermitValues(hwpx.LeavePermitInput{
		ApproveStampDate: "2026-11-09 17:03", AuthorStampDate: "2026-11-09 16:50",
	})
	if vals["승인결재일"] != "2026-11-09 17:03" || vals["작성결재일"] != "2026-11-09 16:50" {
		t.Fatalf("%+v", vals)
	}
	if vals["검토결재일"] != "" {
		t.Fatal("검토결재일")
	}
}

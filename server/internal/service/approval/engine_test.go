package approval

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func testEngine(t *testing.T) (*Engine, *repository.UserRepo, string, string, string) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "e.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	head := &model.User{Username: "head1", PasswordHash: "x", FullName: "팀장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01", Email: "h@a"}
	staff := &model.User{Username: "staff1", PasswordHash: "x", FullName: "직원", Role: model.RoleTech, IsActive: true, OrgID: "O01", Email: "s@a"}
	other := &model.User{Username: "other1", PasswordHash: "x", FullName: "다른", Role: model.RoleTech, IsActive: true, OrgID: "O01", Email: "o@a"}
	for _, u := range []*model.User{head, staff, other} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	hooks := &MemoryHooks{Title: "시험 문서", JSON: `{"n":1}`}
	e := &Engine{DB: db, Now: func() time.Time { return time.Date(2026, 5, 1, 10, 0, 0, 0, time.Local) }, Hooks: hooks}
	if err := e.SetOrgHead(Actor{UserID: head.UserID}, head.UserID, "O01"); err != nil {
		t.Fatal(err)
	}
	return e, users, head.UserID, staff.UserID, other.UserID
}

func actor(id, org, role string) Actor {
	return Actor{UserID: id, OrgID: org, Role: role, Name: id}
}

func TestStateMachineTransitions(t *testing.T) {
	cases := []struct {
		from, action string
		ok           bool
	}{
		{StatusDraft, EvSubmit, true},
		{StatusDraft, EvApprove, false},
		{StatusInProgress, EvApprove, true},
		{StatusInProgress, EvReject, true},
		{StatusInProgress, EvWithdraw, true},
		{StatusApproved, EvReject, false},
		{StatusApproved, EvApprove, false},
		{StatusWithdrawn, EvApprove, false},
		{StatusRejected, EvResubmit, true},
		{StatusWithdrawn, EvResubmit, true},
	}
	for _, tc := range cases {
		if AllowedTransition(tc.from, tc.action) != tc.ok {
			t.Fatalf("%s %s want %v", tc.from, tc.action, tc.ok)
		}
	}
}

func TestSubmitSelfApproveAndRollback(t *testing.T) {
	e, _, head, _, _ := testEngine(t)
	id, err := e.CreateDraft("O01", DocLeave, "L1", head, "휴가")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Submit(actor(head, "O01", model.RoleOrgAdmin), id); err != nil {
		t.Fatal(err)
	}
	req, _ := e.GetRequest(id)
	if req.Status != StatusApproved || req.SnapshotJSON == "" {
		t.Fatalf("%+v", req)
	}
	ev, _ := e.EventTypes(id)
	if strings.Join(ev, ",") != EvSubmit+","+EvSelfApprove {
		t.Fatalf("events %v", ev)
	}
	h := e.Hooks.(*MemoryHooks)
	if h.ApprovedN != 1 {
		t.Fatal("callback")
	}
	h.FailAfter = true
	id2, _ := e.CreateDraft("O01", DocLeave, "L2", head, "휴가2")
	if err := e.Submit(actor(head, "O01", model.RoleOrgAdmin), id2); err == nil {
		t.Fatal("rollback")
	}
	var n int
	_ = e.DB.QueryRow(`SELECT COUNT(*) FROM approval_requests WHERE request_id=? AND status=?`, id2, StatusApproved).Scan(&n)
	if n != 0 {
		t.Fatal("rolled back request still approved")
	}
}

func TestStaffCannotSelfApproveAndPolicyOff(t *testing.T) {
	e, _, head, staff, _ := testEngine(t)
	id, _ := e.CreateDraft("O01", DocLeave, "L3", staff, "직원휴가")
	tid := ""
	_ = e.DB.QueryRow(`SELECT template_id FROM approval_templates WHERE org_id='O01' AND doc_type=?`, DocLeave).Scan(&tid)
	_, _ = e.DB.Exec(`UPDATE approval_template_steps SET approver_rule=? WHERE template_id=?`, "user:"+staff, tid)
	if err := e.Submit(actor(staff, "O01", model.RoleTech), id); err == nil {
		t.Fatal("staff self")
	}
	p, _ := e.GetPolicy("O01")
	p.SelfApproveLeave = false
	_ = e.SavePolicy(actor(head, "O01", model.RoleOrgAdmin), "O01", p)
	id2, _ := e.CreateDraft("O01", DocLeave, "L4", head, "팀장휴가")
	if err := e.Submit(actor(head, "O01", model.RoleOrgAdmin), id2); err == nil {
		t.Fatal("policy off")
	}
}

func TestNoHeadBlocksSubmit(t *testing.T) {
	e, _, _, staff, _ := testEngine(t)
	_, _ = e.DB.Exec(`UPDATE employee_profiles SET is_org_head=0`)
	id, _ := e.CreateDraft("O01", DocTest, "D1", staff, "x")
	if err := e.Submit(actor(staff, "O01", model.RoleTech), id); err == nil || !strings.Contains(err.Error(), "팀장") {
		t.Fatalf("err=%v", err)
	}
}

func TestSequentialApproveRejectWithdrawResubmit(t *testing.T) {
	e, users, head, staff, other := testEngine(t)
	a2 := &model.User{Username: "head2", PasswordHash: "x", FullName: "2차", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01"}
	if err := users.Create(a2); err != nil {
		t.Fatal(err)
	}
	if err := e.EnsurePolicy("O01"); err != nil {
		t.Fatal(err)
	}
	tid := ""
	_ = e.DB.QueryRow(`SELECT template_id FROM approval_templates WHERE org_id='O01' AND doc_type=?`, DocTest).Scan(&tid)
	_, _ = e.DB.Exec(`DELETE FROM approval_template_steps WHERE template_id=?`, tid)
	_, _ = e.DB.Exec(`INSERT INTO approval_template_steps(template_id, step_no, step_type, approver_rule) VALUES (?,?,?,?),(?,?,?,?)`,
		tid, 1, TypeApprove, "user:"+head, tid, 2, TypeApprove, "user:"+a2.UserID)
	id, _ := e.CreateDraft("O01", DocTest, "SEQ1", staff, "순차")
	if err := e.Submit(actor(staff, "O01", model.RoleTech), id); err != nil {
		t.Fatal(err)
	}
	if err := e.Approve(actor(a2.UserID, "O01", model.RoleOrgAdmin), id, "", ""); err == nil {
		t.Fatal("second too early")
	}
	if err := e.Approve(actor(head, "O01", model.RoleOrgAdmin), id, "ok", ""); err != nil {
		t.Fatal(err)
	}
	req, _ := e.GetRequest(id)
	if req.Status != StatusInProgress {
		t.Fatalf("status %s", req.Status)
	}
	if err := e.Approve(actor(a2.UserID, "O01", model.RoleOrgAdmin), id, "ok2", ""); err != nil {
		t.Fatal(err)
	}
	req, _ = e.GetRequest(id)
	if req.Status != StatusApproved {
		t.Fatal(req.Status)
	}
	idR, _ := e.CreateDraft("O01", DocTest, "REJ1", staff, "반려")
	_ = e.Submit(actor(staff, "O01", model.RoleTech), idR)
	if err := e.Reject(actor(head, "O01", model.RoleOrgAdmin), idR, "no"); err != nil {
		t.Fatal(err)
	}
	nid, err := e.Resubmit(actor(staff, "O01", model.RoleTech), idR)
	if err != nil {
		t.Fatal(err)
	}
	nreq, _ := e.GetRequest(nid)
	if nreq.Revision != 2 {
		t.Fatalf("rev %d", nreq.Revision)
	}
	old, _ := e.GetRequest(idR)
	if old.Status != StatusRejected {
		t.Fatal("old preserved")
	}
	idW, _ := e.CreateDraft("O01", DocTest, "W1", staff, "회수")
	_ = e.Submit(actor(staff, "O01", model.RoleTech), idW)
	if err := e.Withdraw(actor(other, "O01", model.RoleTech), idW); err == nil {
		t.Fatal("other withdraw")
	}
	if err := e.Withdraw(actor(staff, "O01", model.RoleTech), idW); err != nil {
		t.Fatal(err)
	}
}

func TestAccessAndLockAndReadonly(t *testing.T) {
	e, _, head, staff, other := testEngine(t)
	id, _ := e.CreateDraft("O01", DocTest, "A1", staff, "권한")
	_ = e.Submit(actor(staff, "O01", model.RoleTech), id)
	steps, _ := e.ListSteps(id)
	if AccessFor(actor(staff, "O01", model.RoleTech), mustReq(e, id), steps, "")&PermView == 0 {
		t.Fatal("drafter view")
	}
	if AccessFor(actor(head, "O01", model.RoleOrgAdmin), mustReq(e, id), steps, "")&PermAct == 0 {
		t.Fatal("head act")
	}
	if AccessFor(actor(other, "O01", model.RoleTech), mustReq(e, id), steps, "")&PermAct != 0 {
		t.Fatal("other act")
	}
	ro := actor(head, "O01", model.RoleOrgAdmin)
	ro.ReadOnly = true
	if AccessFor(ro, mustReq(e, id), steps, "")&PermAct != 0 {
		t.Fatal("readonly act")
	}
	va := actor(head, "O01", model.RoleOrgAdmin)
	va.ViewAs = true
	if err := e.Approve(va, id, "", ""); err == nil {
		t.Fatal("viewas")
	}
	if !e.IsLocked(DocTest, "A1") {
		t.Fatal("lock")
	}
	if ClockDash(0) != "—" {
		t.Fatal("dash")
	}
}

func mustReq(e *Engine, id string) Request {
	r, _ := e.GetRequest(id)
	return r
}

func TestDelegationPolicyAndEmergency(t *testing.T) {
	e, users, head, staff, other := testEngine(t)
	a := actor(head, "O01", model.RoleOrgAdmin)
	p, _ := e.GetPolicy("O01")
	p.DelegByHead = false
	_ = e.SavePolicy(a, "O01", p)
	if _, err := e.CreateDelegation(a, "O01", head, staff, "2026-05-01", "2026-05-10", SrcHead, ""); err == nil {
		t.Fatal("A off")
	}
	p.DelegByHead, p.DelegByAdmin = true, false
	_ = e.SavePolicy(a, "O01", p)
	if _, err := e.CreateDelegation(actor(staff, "O01", model.RoleTech), "O01", head, other, "2026-05-01", "2026-05-10", SrcAdmin, ""); err == nil {
		t.Fatal("B off")
	}
	p.DelegByAdmin = true
	_ = e.SavePolicy(a, "O01", p)
	id, err := e.CreateDelegation(a, "O01", head, staff, "2026-05-01", "2026-05-02", SrcHead, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.CreateDelegation(a, "O01", head, other, "2026-05-02", "2026-05-03", SrcHead, ""); err == nil {
		t.Fatal("overlap")
	}
	if _, err := e.CreateDelegation(a, "O01", head, head, "2026-06-01", "2026-06-02", SrcHead, ""); err == nil {
		t.Fatal("self del")
	}
	rid, _ := e.CreateDraft("O01", DocTest, "DEL1", other, "대결")
	_ = e.Submit(actor(other, "O01", model.RoleTech), rid)
	if err := e.Approve(actor(staff, "O01", model.RoleTech), rid, "대결", ""); err != nil {
		t.Fatal(err)
	}
	steps, _ := e.ListSteps(rid)
	if steps[0].ActMode != ModeDelegate || steps[0].ActedBy != staff {
		t.Fatalf("%+v", steps[0])
	}
	own, _ := e.CreateDraft("O01", DocTest, "DEL2", staff, "자기대결금지")
	_ = e.Submit(actor(staff, "O01", model.RoleTech), own)
	if err := e.Approve(actor(staff, "O01", model.RoleTech), own, "", ""); err == nil {
		t.Fatal("delegate own")
	}
	_ = e.RevokeDelegation(a, id)
	mgr := &model.User{Username: "mgr1", PasswordHash: "x", FullName: "관리", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01"}
	if err := users.Create(mgr); err != nil {
		t.Fatal(err)
	}
	ma := actor(mgr.UserID, "O01", model.RoleOrgAdmin)
	p.AdminEmergencyApprove = false
	_ = e.SavePolicy(a, "O01", p)
	rid3, _ := e.CreateDraft("O01", DocTest, "EM0", other, "비상불가")
	_ = e.Submit(actor(other, "O01", model.RoleTech), rid3)
	if err := e.Approve(ma, rid3, "", "사유"); err == nil {
		t.Fatal("D off")
	}
	p.AdminEmergencyApprove = true
	_ = e.SavePolicy(a, "O01", p)
	if err := e.Approve(ma, rid3, "", ""); err == nil {
		t.Fatal("reason required")
	}
	if err := e.Approve(ma, rid3, "", "팀장 출장"); err != nil {
		t.Fatal(err)
	}
	st, _ := e.ListSteps(rid3)
	if st[0].ActMode != ModeAdminEmergency {
		t.Fatalf("mode %s", st[0].ActMode)
	}
	if !RequireLeaveDeputy(Policy{DelegWithLeave: true}, true) {
		t.Fatal("C require")
	}
	p.DelegWithLeave = true
	_ = e.SavePolicy(a, "O01", p)
	if _, err := e.OnLeaveApprovedCreateDelegation(a, "O01", head, staff, "2026-07-01", "2026-07-05", "LR1"); err != nil {
		t.Fatal(err)
	}
	var mailN int
	_ = e.DB.QueryRow(`SELECT COUNT(*) FROM mail_outbox WHERE purpose='approval'`).Scan(&mailN)
	if mailN == 0 {
		t.Fatal("notify queue")
	}
}

func TestBoundaryDates(t *testing.T) {
	e, _, head, staff, _ := testEngine(t)
	a := actor(head, "O01", model.RoleOrgAdmin)
	_, err := e.CreateDelegation(a, "O01", head, staff, "2026-05-01", "2026-05-03", SrcHead, "")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := e.activeDelegate(head, "2026-05-01")
	if got != staff {
		t.Fatal("start")
	}
	got, _ = e.activeDelegate(head, "2026-05-03")
	if got != staff {
		t.Fatal("end")
	}
	got, _ = e.activeDelegate(head, "2026-04-30")
	if got != "" {
		t.Fatal("before")
	}
}

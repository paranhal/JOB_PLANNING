package repository

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func setupCoop(t *testing.T) (*CoopRepo, *UserRepo, *model.User, *model.User, *model.User, *model.User, *model.User) {
	t.Helper()
	db, err := InitDB(filepath.Join(t.TempDir(), "coop.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O02','O02','미디어','business',1,20)`); err != nil {
		t.Fatal(err)
	}
	users := NewUserRepo(db)
	oa1 := &model.User{Username: "oa1", PasswordHash: "x", FullName: "도서팀장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	tech1 := &model.User{Username: "t1", PasswordHash: "x", FullName: "도서기술", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	oa2 := &model.User{Username: "oa2", PasswordHash: "x", FullName: "미디어팀장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O02", ProfileDone: true}
	tech2 := &model.User{Username: "t2", PasswordHash: "x", FullName: "미디어기술", Role: model.RoleTech, IsActive: true, OrgID: "O02", ProfileDone: true}
	cl := &model.User{Username: "cl", PasswordHash: "x", FullName: "경영", Role: model.RoleCLevel, IsActive: true, IsReadOnly: true, ProfileDone: true}
	obs := &model.User{Username: "ob", PasswordHash: "x", FullName: "옵저버", Role: model.RoleObserver, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	for _, u := range []*model.User{oa1, tech1, oa2, tech2, cl, obs} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	return NewCoopRepo(db), users, oa1, tech1, oa2, tech2, cl
}

func coopA(u *model.User) model.CoopActor {
	return model.CoopActor{UserID: u.UserID, OrgID: u.OrgID, Role: u.Role, ReadOnly: u.IsReadOnly}
}

func TestCoopCreateAndACL(t *testing.T) {
	repo, _, oa1, tech1, oa2, tech2, cl := setupCoop(t)
	c := &model.Cooperation{TargetOrgID: "O02", Kind: model.CoopAdmin, Title: "지원요청", Body: "내용"}
	if err := repo.Create(coopA(tech1), c); err != nil {
		t.Fatal(err)
	}
	if c.Status != model.CoopRequested {
		t.Fatalf("status %s", c.Status)
	}
	self := &model.Cooperation{TargetOrgID: model.OrgIDLibrary, Kind: model.CoopAdmin, Title: "자기", Body: "x"}
	if err := repo.Create(coopA(tech1), self); err == nil {
		t.Fatal("자기 팀")
	}
	obsA := model.CoopActor{UserID: "x", OrgID: model.OrgIDLibrary, Role: model.RoleObserver}
	if err := repo.Create(obsA, &model.Cooperation{TargetOrgID: "O02", Kind: model.CoopAdmin, Title: "t", Body: "b"}); err != errCoopForbidden {
		t.Fatalf("observer %v", err)
	}
	if err := repo.Create(coopA(cl), &model.Cooperation{TargetOrgID: "O02", Kind: model.CoopAdmin, Title: "t", Body: "b"}); err != errCoopForbidden {
		t.Fatalf("clevel %v", err)
	}
	view := model.CoopActor{UserID: tech1.UserID, OrgID: tech1.OrgID, Role: model.RoleTech, ViewAs: true}
	if err := repo.Create(view, &model.Cooperation{TargetOrgID: "O02", Kind: model.CoopAdmin, Title: "t", Body: "b"}); err != errCoopForbidden {
		t.Fatal("view-as")
	}
	partner := model.CoopActor{UserID: tech1.UserID, OrgID: tech1.OrgID, Role: "partner"}
	if err := repo.Create(partner, &model.Cooperation{TargetOrgID: "O02", Kind: model.CoopAdmin, Title: "t", Body: "b"}); err != errCoopForbidden {
		t.Fatal("partner")
	}
	got, _ := repo.Get(c.CoopID)
	if !repo.CanSee(coopA(tech1), got) || !repo.CanSee(coopA(oa1), got) || !repo.CanSee(coopA(oa2), got) || !repo.CanSee(coopA(cl), got) {
		t.Fatal("열람")
	}
	if repo.CanSee(coopA(tech2), got) {
		t.Fatal("제3 팀원")
	}
	if repo.CanActInbox(coopA(cl), "O02") {
		t.Fatal("clevel 쓰기")
	}
	if err := repo.Reject(coopA(oa2), c.CoopID, "   ", ""); err == nil {
		t.Fatal("빈 거부")
	}
	if err := repo.Reject(coopA(oa2), c.CoopID, "인력 부족", ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.Accept(coopA(oa2), c.CoopID, ""); err == nil {
		t.Fatal("거부 후 수락")
	}
	n, err := repo.ReRequest(coopA(tech1), c.CoopID)
	if err != nil {
		t.Fatal(err)
	}
	if n.CoopID == c.CoopID || n.Status != model.CoopRequested {
		t.Fatalf("재요청 %+v", n)
	}
}

func TestCoopStateAndAssign(t *testing.T) {
	repo, users, _, tech1, oa2, tech2, _ := setupCoop(t)
	u3 := &model.User{Username: "t3", PasswordHash: "x", FullName: "미디어2", Role: model.RoleTech, IsActive: true, OrgID: "O02", ProfileDone: true}
	if err := users.Create(u3); err != nil {
		t.Fatal(err)
	}
	c := &model.Cooperation{TargetOrgID: "O02", Kind: model.CoopAdmin, Title: "행정지원", Body: "본문", WantStart: "2026-10-20", WantEnd: "2026-10-22"}
	if err := repo.Create(coopA(tech1), c); err != nil {
		t.Fatal(err)
	}
	if err := repo.Cancel(coopA(oa2), c.CoopID); err == nil {
		t.Fatal("대상 팀 취소")
	}
	if err := repo.Accept(coopA(oa2), c.CoopID, ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.Cancel(coopA(tech1), c.CoopID); err == nil {
		t.Fatal("수락 후 취소")
	}
	if err := repo.Assign(coopA(oa2), c.CoopID, "2026-10-21", "2026-10-23", "지시", "", []string{tech2.UserID, u3.UserID}); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.Get(c.CoopID)
	if got.Status != model.CoopAssigned || len(got.Assignees) != 2 {
		t.Fatalf("%+v", got)
	}
	var n int
	_ = repo.db.QueryRow(`SELECT COUNT(*) FROM work_tasks WHERE cooperation_id=?`, c.CoopID).Scan(&n)
	if n != 1 {
		t.Fatalf("업무 %d", n)
	}
	g := &model.WorkGroup{OrgID: "O02", Name: "협조그룹"}
	if err := repo.groups.Create(model.WorkGroupActor{UserID: oa2.UserID, OrgID: "O02", Role: model.RoleOrgAdmin}, g); err != nil {
		t.Fatal(err)
	}
	_ = repo.groups.AddMember(model.WorkGroupActor{UserID: oa2.UserID, OrgID: "O02", Role: model.RoleOrgAdmin}, g.GroupID, tech2.UserID, model.WorkGroupMember, "")
	if err := repo.Assign(coopA(oa2), c.CoopID, "2026-10-21", "2026-10-23", "그룹", g.GroupID, nil); err != nil {
		t.Fatal(err)
	}
	foreign := &model.WorkGroup{OrgID: model.OrgIDLibrary, Name: "도서그룹"}
	_ = repo.groups.Create(model.WorkGroupActor{UserID: tech1.UserID, OrgID: model.OrgIDLibrary, Role: model.RoleVisionAdmin}, foreign)
	if err := repo.Assign(coopA(oa2), c.CoopID, "2026-10-21", "2026-10-23", "x", foreign.GroupID, nil); err == nil {
		t.Fatal("타 조직 그룹")
	}
	if err := repo.Complete(coopA(oa2), c.CoopID, "", ""); err == nil {
		t.Fatal("미완료 사유")
	}
	if err := repo.Complete(coopA(oa2), c.CoopID, "남은 업무 후속", ""); err != nil {
		t.Fatal(err)
	}
}

func TestCoopASIncompleteAndEvents(t *testing.T) {
	repo, _, _, tech1, oa2, tech2, _ := setupCoop(t)
	c := &model.Cooperation{TargetOrgID: "O02", Kind: model.CoopAS, Title: "AS지원", Body: "고장"}
	if err := repo.Create(coopA(tech1), c); err != nil {
		t.Fatal(err)
	}
	if err := repo.Accept(coopA(oa2), c.CoopID, ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.Assign(coopA(oa2), c.CoopID, "2026-10-21", "2026-10-22", "현장", "", []string{tech2.UserID}); err != nil {
		t.Fatal(err)
	}
	var asID string
	_ = repo.db.QueryRow(`SELECT as_id FROM as_receipts WHERE cooperation_id=?`, c.CoopID).Scan(&asID)
	if asID == "" {
		t.Fatal("as")
	}
	as, err := repo.as.GetByID("O02", asID)
	if err != nil || as == nil {
		t.Fatal(err)
	}
	as.Status = "completed"
	now := time.Now()
	as.CompleteDatetime = &now
	if err := repo.as.Update(as); err == nil {
		t.Fatal("보완 전 완료")
	}
	if _, err := repo.db.Exec(`UPDATE cooperation_events SET action='x'`); err == nil {
		t.Fatal("event update")
	}
	if _, err := repo.db.Exec(`DELETE FROM cooperation_events`); err == nil {
		t.Fatal("event delete")
	}
	ev, _ := repo.Events(c.CoopID)
	if len(ev) < 2 {
		t.Fatalf("events %d", len(ev))
	}
	up, _ := coopMigrationSQL()
	down, _ := coopDownSQL()
	if !strings.Contains(up, "work_cooperations") || !strings.Contains(down, "DROP TABLE") {
		t.Fatal("sql")
	}
}

func TestCoopDeputyAndDelay(t *testing.T) {
	repo, users, _, tech1, oa2, tech2, _ := setupCoop(t)
	if _, err := repo.db.Exec(`INSERT INTO employee_profiles(user_id, org_id, is_org_head) VALUES (?,?,1)`, oa2.UserID, "O02"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.Exec(`INSERT INTO approval_delegations(delegation_id,org_id,from_user_id,to_user_id,start_date,end_date,source,created_by)
		VALUES('D1','O02',?,?,?,?, 'manual','t')`, oa2.UserID, tech2.UserID, "2000-01-01", "2099-01-01"); err != nil {
		t.Fatal(err)
	}
	c := &model.Cooperation{TargetOrgID: "O02", Kind: model.CoopAdmin, Title: "대결", Body: "내용"}
	if err := repo.Create(coopA(tech1), c); err != nil {
		t.Fatal(err)
	}
	if !repo.CanActInbox(coopA(tech2), "O02") {
		t.Fatal("대결자")
	}
	if err := repo.Accept(coopA(tech2), c.CoopID, tech2.UserID); err != nil {
		t.Fatal(err)
	}
	ev, _ := repo.Events(c.CoopID)
	found := false
	for _, e := range ev {
		if e.Action == "수락" && e.OnBehalf == tech2.UserID {
			found = true
		}
	}
	if !found {
		t.Fatalf("%+v", ev)
	}
	_ = users
	if _, err := repo.db.Exec(`UPDATE work_cooperations SET created_at='2020-01-01' WHERE coop_id=?`, c.CoopID); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.Get(c.CoopID)
	if got.Status == model.CoopRequested && !got.Overdue {
		t.Fatal("지연")
	}
	var notices int
	_ = repo.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='work_assign_notices'`).Scan(&notices)
}

package repository

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func setupWG(t *testing.T) (*WorkGroupRepo, *UserRepo, *model.User, *model.User, *model.User, *model.User) {
	t.Helper()
	db, err := InitDB(filepath.Join(t.TempDir(), "wg.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O02','O02','미디어', 'business',1,20)`); err != nil {
		t.Fatal(err)
	}
	users := NewUserRepo(db)
	oa := &model.User{Username: "oa", PasswordHash: "x", FullName: "조직장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	u1 := &model.User{Username: "u1", PasswordHash: "x", FullName: "조직원1", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	u2 := &model.User{Username: "u2", PasswordHash: "x", FullName: "조직원2", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	u3 := &model.User{Username: "u3", PasswordHash: "x", FullName: "미디어원", Role: model.RoleTech, IsActive: true, OrgID: "O02", ProfileDone: true}
	for _, u := range []*model.User{oa, u1, u2, u3} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	return NewWorkGroupRepo(db), users, oa, u1, u2, u3
}

func oaActor(u *model.User) model.WorkGroupActor {
	return model.WorkGroupActor{UserID: u.UserID, OrgID: u.OrgID, Role: u.Role}
}

func TestWorkGroupSchemaAndACL(t *testing.T) {
	repo, users, oa, u1, u2, u3 := setupWG(t)
	a := oaActor(oa)
	g1 := &model.WorkGroup{OrgID: model.OrgIDLibrary, Name: "그룹1"}
	if err := repo.Create(a, g1); err != nil {
		t.Fatal(err)
	}
	g2 := &model.WorkGroup{OrgID: model.OrgIDLibrary, Name: "그룹2"}
	if err := repo.Create(a, g2); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddMember(a, g1.GroupID, u1.UserID, model.WorkGroupMember, ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddMember(a, g1.GroupID, u2.UserID, model.WorkGroupMember, ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddMember(a, g2.GroupID, u1.UserID, model.WorkGroupMember, ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddMember(a, g2.GroupID, u3.UserID, model.WorkGroupMember, ""); err == nil {
		t.Fatal("타 조직 구성원")
	}
	if err := repo.AddMember(a, g1.GroupID, u1.UserID, model.WorkGroupMember, ""); err == nil {
		t.Fatal("같은 그룹 중복")
	}
	if err := repo.SetLeader(a, g1.GroupID, u1.UserID); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetLeader(a, g2.GroupID, u1.UserID); err != nil {
		t.Fatal(g2)
	}
	// 그룹2 그룹장은 u1이면 그룹원으로 바꾸기: 그룹2 그룹장=없음 후 u2? spec: g1 leader u1, g2 member u1. Set g2 leader to someone else...
	if err := repo.SetLeader(a, g2.GroupID, ""); err != nil {
		t.Fatal(err)
	}
	m1 := repo.memberOn(g1.GroupID, u1.UserID, todayYMD())
	m2 := repo.memberOn(g2.GroupID, u1.UserID, todayYMD())
	if m1 == nil || m1.Role != model.WorkGroupLeader {
		t.Fatalf("g1 leader %+v", m1)
	}
	if m2 == nil || m2.Role != model.WorkGroupMember {
		t.Fatalf("g2 member %+v", m2)
	}

	cl := &model.User{Username: "cl", PasswordHash: "x", FullName: "임원", Role: model.RoleCLevel, IsActive: true, IsReadOnly: true, ProfileDone: true}
	obs := &model.User{Username: "ob", PasswordHash: "x", FullName: "옵저버", Role: model.RoleObserver, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	ts := &model.User{Username: "ts", PasswordHash: "x", FullName: "테스터", Role: model.RoleTester, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	for _, u := range []*model.User{cl, obs, ts} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
		if err := repo.AddMember(a, g1.GroupID, u.UserID, model.WorkGroupMember, ""); err == nil {
			t.Fatalf("거부돼야 함 %s", u.Role)
		}
	}
	if _, err := repo.db.Exec(`INSERT INTO users(user_id,username,password_hash,full_name,role,is_active,org_id) VALUES('USR-PT','pt','x','협력','partner',1,?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddMember(a, g1.GroupID, "USR-PT", model.WorkGroupMember, ""); err == nil {
		t.Fatal("협력사")
	}

	lead := model.WorkGroupActor{UserID: u1.UserID, OrgID: model.OrgIDLibrary, Role: model.RoleTech}
	if err := repo.Create(lead, &model.WorkGroup{OrgID: model.OrgIDLibrary, Name: "불가"}); err == nil {
		t.Fatal("그룹장 생성")
	}
	if err := repo.Close(lead, g1.GroupID); err == nil {
		t.Fatal("그룹장 종료")
	}
	if err := repo.SetLeader(lead, g1.GroupID, u2.UserID); err == nil {
		t.Fatal("그룹장 변경")
	}
	if err := repo.RemoveMember(lead, g1.GroupID, u1.UserID); err == nil {
		t.Fatal("본인 제거")
	}
	if err := repo.RemoveMember(lead, g1.GroupID, u2.UserID); err != nil {
		t.Fatal(err)
	}
	on := todayYMD()
	if repo.memberOn(g1.GroupID, u2.UserID, on) == nil {
		t.Fatal("제거 당일은 포함")
	}
	next := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	if repo.memberOn(g1.GroupID, u2.UserID, next) != nil {
		t.Fatal("제거 다음날")
	}

	if _, err := repo.db.Exec(`UPDATE work_group_events SET action='x'`); err == nil {
		t.Fatal("이벤트 수정")
	}
	if _, err := repo.db.Exec(`DELETE FROM work_groups WHERE group_id=?`, g1.GroupID); err == nil {
		t.Fatal("그룹 삭제")
	}

	up, err := workGroupMigrationSQL()
	if err != nil {
		t.Fatal(err)
	}
	down, err := workGroupDownSQL()
	if err != nil {
		t.Fatal(err)
	}
	if err := execWorkGroupSQL(repo.db, down); err != nil {
		t.Fatal(err)
	}
	if err := execWorkGroupSQL(repo.db, up); err != nil {
		t.Fatal(err)
	}
}

func TestWorkGroupEffortRules(t *testing.T) {
	repo, _, oa, u1, u2, _ := setupWG(t)
	a := oaActor(oa)
	g := &model.WorkGroup{OrgID: model.OrgIDLibrary, Name: "투입그룹"}
	if err := repo.Create(a, g); err != nil {
		t.Fatal(err)
	}
	_ = repo.AddMember(a, g.GroupID, u1.UserID, model.WorkGroupMember, "")
	_ = repo.AddMember(a, g.GroupID, u2.UserID, model.WorkGroupMember, "")
	_ = repo.SetLeader(a, g.GroupID, u1.UserID)
	lead := model.WorkGroupActor{UserID: u1.UserID, OrgID: model.OrgIDLibrary, Role: model.RoleTech}
	other := model.WorkGroupActor{UserID: u2.UserID, OrgID: model.OrgIDLibrary, Role: model.RoleTech}
	day := todayYMD()
	if _, err := repo.AddEffort(lead, day, u2.UserID, g.GroupID, "P1", 60, "현장", ""); err != nil {
		t.Fatal(err)
	}
	e, err := repo.AddEffort(other, day, other.UserID, g.GroupID, "P1", 30, "현장", "")
	if err != nil {
		t.Fatal(err)
	}
	if e.GroupRole != model.WorkGroupMember {
		t.Fatalf("snapshot %s", e.GroupRole)
	}
	if _, err := repo.AddEffort(other, day, u1.UserID, g.GroupID, "P1", 10, "", ""); err == nil {
		t.Fatal("구성원이 대리")
	}
	if _, err := repo.AddEffort(lead, day, oa.UserID, g.GroupID, "P1", 10, "", ""); err == nil {
		t.Fatal("비구성원 대리")
	}
	if _, err := repo.AddEffort(lead, day, u2.UserID, g.GroupID, "P1", 1400, "", ""); err == nil {
		t.Fatal("1440 초과")
	}
	if err := repo.SetLeader(a, g.GroupID, u2.UserID); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.getEffort(e.EntryID)
	if got.GroupRole != model.WorkGroupMember {
		t.Fatal("스냅샷 변함")
	}
	sums, err := repo.EffortByProject("P1", day, day)
	if err != nil || len(sums) == 0 {
		t.Fatalf("%v %v", sums, err)
	}
	if err := repo.VoidEffort(other, e.EntryID); err != nil {
		t.Fatal(err)
	}
	after, _ := repo.EffortByUser(u2.UserID, day, day)
	for _, s := range after {
		if s.Minutes != 0 && s.Key != "" && s.Minutes == 30 {
			t.Fatal("무효 포함")
		}
	}
	g2 := &model.WorkGroup{OrgID: model.OrgIDLibrary, Name: "둘째"}
	_ = repo.Create(a, g2)
	_ = repo.AddMember(a, g2.GroupID, u1.UserID, model.WorkGroupMember, "")
	if _, err := repo.AddEffort(lead, day, u1.UserID, g2.GroupID, "P1", 15, "", ""); err != nil {
		t.Fatal(err)
	}
	byu, _ := repo.EffortByUser(u1.UserID, day, day)
	if len(byu) < 1 {
		t.Fatal("두 그룹 분리")
	}

	exp := &model.WorkGroup{OrgID: model.OrgIDLibrary, Name: "지난그룹", EndDate: "2000-01-01"}
	_ = repo.Create(a, exp)
	_ = repo.AddMember(a, exp.GroupID, u1.UserID, model.WorkGroupMember, "1999-01-01")
	if _, err := repo.AddEffort(lead, day, u1.UserID, exp.GroupID, "", 10, "", ""); err == nil {
		t.Fatal("기한 지난 그룹")
	}
}

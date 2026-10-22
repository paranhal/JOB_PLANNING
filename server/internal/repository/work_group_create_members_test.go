package repository

import (
	"strings"
	"testing"

	"customer-support/internal/model"
)

func TestCreateWithMembersTxnAndVisibleGroups(t *testing.T) {
	repo, _, oa, u1, u2, u3 := setupWG(t)
	a := oaActor(oa)

	g := &model.WorkGroup{OrgID: model.OrgIDLibrary, Name: "TX그룹", StartDate: "2026-01-01", EndDate: "2026-12-31"}
	if err := repo.CreateWithMembers(a, g, []string{u1.UserID, u2.UserID}, u1.UserID); err != nil {
		t.Fatal(err)
	}
	ms, _ := repo.MembersOn(g.GroupID, "")
	if len(ms) != 2 {
		t.Fatalf("n=%d", len(ms))
	}
	if g.LeaderID != u1.UserID && repo.memberOn(g.GroupID, u1.UserID, todayYMD()).Role != model.WorkGroupLeader {
		t.Fatal("그룹장")
	}

	bad := &model.WorkGroup{OrgID: model.OrgIDLibrary, Name: "TX실패"}
	if err := repo.CreateWithMembers(a, bad, []string{u1.UserID}, u2.UserID); err == nil {
		t.Fatal("그룹장이 구성원이 아닌데 허용")
	}
	items, _ := repo.ListGroups(model.OrgIDLibrary, "")
	for _, it := range items {
		if it.Name == "TX실패" {
			t.Fatal("롤백 안 됨")
		}
	}

	closed := &model.WorkGroup{OrgID: model.OrgIDLibrary, Name: "종료후보"}
	if err := repo.CreateWithMembers(a, closed, []string{u1.UserID}, u1.UserID); err != nil {
		t.Fatal(err)
	}
	if err := repo.Close(a, closed.GroupID); err != nil {
		t.Fatal(err)
	}
	expired := &model.WorkGroup{OrgID: model.OrgIDLibrary, Name: "지난후보", EndDate: "2000-01-01"}
	if err := repo.Create(a, expired); err != nil {
		t.Fatal(err)
	}
	_ = repo.AddMember(a, expired.GroupID, u1.UserID, model.WorkGroupMember, "1999-01-01")

	adminList, err := repo.ListVisibleActiveForWork(model.OrgIDLibrary, oa.UserID, true)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, it := range adminList {
		names[it.Name] = true
	}
	if !names["TX그룹"] {
		t.Fatal("관리자 활성 그룹")
	}
	if names["종료후보"] || names["지난후보"] {
		t.Fatal("종료·기간 지난 그룹이 후보")
	}

	staff, err := repo.ListVisibleActiveForWork(model.OrgIDLibrary, u2.UserID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(staff) != 1 || staff[0].Name != "TX그룹" {
		t.Fatalf("조직원 후보 %+v", staff)
	}
	none, err := repo.ListVisibleActiveForWork(model.OrgIDLibrary, u3.UserID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Fatalf("타 조직원 후보 %d", len(none))
	}
}

func TestWorkGroupAlreadyMemberIsSkipNotUniqueError(t *testing.T) {
	repo, _, oa, u1, _, _ := setupWG(t)
	a := oaActor(oa)
	g := &model.WorkGroup{OrgID: model.OrgIDLibrary, Name: "중복안내"}
	if err := repo.CreateWithMembers(a, g, []string{u1.UserID}, u1.UserID); err != nil {
		t.Fatal(err)
	}
	err := repo.AddMember(a, g.GroupID, u1.UserID, model.WorkGroupMember, "")
	if err == nil {
		t.Fatal("중복이 성공")
	}
	if !IsWorkGroupAlreadyMember(err) {
		t.Fatalf("이미 소속 판별 실패: %v", err)
	}
	if strings.Contains(strings.ToLower(err.Error()), "unique") {
		t.Fatal("유니크 오류를 그대로 노출")
	}
}

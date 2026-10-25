package repository

import (
	"testing"
	"time"

	"customer-support/internal/model"
)

func Test95_37LeaderMatrixMembersQueryCountTotals(t *testing.T) {
	repo, users, oa, u1, u2, u3 := setupWG(t)
	a := oaActor(oa)
	gA := &model.WorkGroup{OrgID: model.OrgIDLibrary, Name: "A조"}
	gB := &model.WorkGroup{OrgID: model.OrgIDLibrary, Name: "B조"}
	if err := repo.Create(a, gA); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(a, gB); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddMember(a, gA.GroupID, u1.UserID, model.WorkGroupMember, ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddMember(a, gA.GroupID, u2.UserID, model.WorkGroupMember, ""); err != nil {
		t.Fatal(err)
	}
	u4 := &model.User{Username: "u4", PasswordHash: "x", FullName: "조직원4", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	if err := users.Create(u4); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddMember(a, gA.GroupID, u4.UserID, model.WorkGroupMember, ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetLeader(a, gA.GroupID, u1.UserID); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddMember(a, gB.GroupID, u2.UserID, model.WorkGroupMember, ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetLeader(a, gB.GroupID, u2.UserID); err != nil {
		t.Fatal(err)
	}

	leadA := model.WorkGroupActor{UserID: u1.UserID, OrgID: model.OrgIDLibrary, Role: model.RoleTech}
	leadB := model.WorkGroupActor{UserID: u2.UserID, OrgID: model.OrgIDLibrary, Role: model.RoleTech}
	mem := model.WorkGroupActor{UserID: u4.UserID, OrgID: model.OrgIDLibrary, Role: model.RoleTech}
	out := model.WorkGroupActor{UserID: u3.UserID, OrgID: "O02", Role: model.RoleTech}

	gA.Name = "A조-수정"
	if err := repo.Update(leadA, gA); err != nil {
		t.Fatal("그룹장 수정", err)
	}
	if err := repo.Update(leadB, gA); err == nil {
		t.Fatal("다른 그룹장 수정")
	}
	if err := repo.Update(mem, gA); err == nil {
		t.Fatal("구성원 수정")
	}
	if err := repo.Update(out, gA); err == nil {
		t.Fatal("비구성원 수정")
	}
	if err := repo.Close(leadA, gA.GroupID); err != nil {
		t.Fatal("그룹장 종료", err)
	}
	if err := repo.Reopen(leadA, gA.GroupID); err != nil {
		t.Fatal("그룹장 재개", err)
	}
	if err := repo.Close(leadB, gA.GroupID); err == nil {
		t.Fatal("다른 그룹장 종료")
	}
	if err := repo.Create(leadA, &model.WorkGroup{OrgID: model.OrgIDLibrary, Name: "생성불가"}); err == nil {
		t.Fatal("그룹장 생성")
	}

	if err := repo.RemoveMember(leadA, gA.GroupID, u4.UserID); err != nil {
		t.Fatal(err)
	}
	on := todayYMD()
	if repo.memberOn(gA.GroupID, u4.UserID, on) == nil {
		t.Fatal("제거 당일 포함")
	}
	next := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	if repo.memberOn(gA.GroupID, u4.UserID, next) != nil {
		t.Fatal("제거 다음날")
	}
	if !repo.TaskGroupMemberOn(gA.GroupID, u4.UserID, on) {
		t.Fatal("기간 내 TaskGroupMemberOn")
	}
	if repo.TaskGroupMemberOn(gA.GroupID, u4.UserID, next) {
		t.Fatal("탈퇴 이후")
	}
	past := time.Now().AddDate(0, 0, -10).Format("2006-01-02")
	if repo.TaskGroupMemberOn(gA.GroupID, u4.UserID, past) {
		t.Fatal("가입 전")
	}
	if err := repo.AddMember(leadA, gA.GroupID, u4.UserID, model.WorkGroupMember, ""); err != nil {
		t.Fatal("재가입", err)
	}
	if repo.memberOn(gA.GroupID, u4.UserID, on) == nil {
		t.Fatal("재가입 후")
	}
	ev, err := repo.ListEvents(gA.GroupID, 20)
	if err != nil || len(ev) == 0 {
		t.Fatalf("이력 %v %d", err, len(ev))
	}
	if err := repo.AddMember(a, gA.GroupID, u3.UserID, model.WorkGroupMember, ""); err == nil {
		t.Fatal("타 조직")
	}

	if _, err := repo.db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c37','점검교','점검교',1,?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	asRepo := NewASRepo(repo.db)
	today := todayYMD()
	as1 := &model.ASReceipt{
		CustomerID: "c37", Symptom: "그룹AS", Status: "received",
		ReceiptDatetime: time.Now(), OrgID: model.OrgIDLibrary,
		WorkGroupID: gA.GroupID, VisitScheduledDate: today,
	}
	if err := asRepo.Create(as1); err != nil {
		t.Fatal(err)
	}
	n, err := CountDistinctByGroup(repo.db, "as_receipts", "as_id", gA.GroupID)
	if err != nil || n != 1 {
		t.Fatalf("합계 불변 want 1 got %d %v", n, err)
	}
	wb := NewWorkBoardRepo(repo.db)
	ResetSQLQueryN()
	one, err := wb.ListWorkToday(u2.UserID, []string{u2.FullName}, today)
	if err != nil {
		t.Fatal(err)
	}
	q1 := SQLQueryN()
	found := 0
	for _, it := range one {
		if it.RefID == as1.ASID {
			found++
		}
	}
	if found != 1 {
		t.Fatalf("구성원 목록 중복 %d", found)
	}
	team, err := wb.ListWorkToday("", nil, today)
	if err != nil {
		t.Fatal(err)
	}
	teamN := 0
	for _, it := range team {
		if it.WorkGroupID == gA.GroupID {
			teamN++
		}
	}
	if teamN != 1 {
		t.Fatalf("팀 합계가 구성원 수만큼 늘면 안 됨 %d", teamN)
	}
	for i := 0; i < 19; i++ {
		as := &model.ASReceipt{
			CustomerID: "c37", Symptom: "그룹AS더", Status: "received",
			ReceiptDatetime: time.Now(), OrgID: model.OrgIDLibrary,
			WorkGroupID: gA.GroupID, VisitScheduledDate: today,
		}
		if err := asRepo.Create(as); err != nil {
			t.Fatal(err)
		}
	}
	ResetSQLQueryN()
	more, err := wb.ListWorkToday(u2.UserID, []string{u2.FullName}, today)
	if err != nil {
		t.Fatal(err)
	}
	q2 := SQLQueryN()
	if len(more) < 20 {
		t.Fatalf("그룹 업무 %d", len(more))
	}
	if q2 > q1+8 {
		t.Fatalf("쿼리 수가 N에 비례 q1=%d q2=%d", q1, q2)
	}
	n, err = CountDistinctByGroup(repo.db, "as_receipts", "as_id", gA.GroupID)
	if err != nil || n != 20 {
		t.Fatalf("20건 distinct %d %v", n, err)
	}

	ApplyASWorkGroupV86ForTest(repo.db)
	ApplyASWorkGroupV86ForTest(repo.db)
	var cnt int
	if err := repo.db.QueryRow(`SELECT COUNT(*) FROM as_receipts`).Scan(&cnt); err != nil || cnt != 20 {
		t.Fatalf("마이그레이션 후 건수 %d %v", cnt, err)
	}
}

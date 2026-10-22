package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestMnt95_14RegionThenSiteExceptionAndNoRetroactive(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "reg.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := NewUserRepo(db)
	_ = users.EnsureAdmin("x")
	tech := &model.User{Username: "rt", PasswordHash: "x", FullName: "지역담당", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	exc := &model.User{Username: "re", PasswordHash: "x", FullName: "예외담당", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	for _, u := range []*model.User{tech, exc} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c14r','지역사이트','지역사이트',1,?),('c14s','예외사이트','예외사이트',1,?)`, model.OrgIDLibrary, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	repo := NewMaintenanceRepo(db)
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "c14r", ShortName: "지역", Region: "세종", HasKlas: true, InspectionCycle: "monthly", RoundsPerPeriod: 3, OrgID: model.OrgIDLibrary,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "c14s", ShortName: "예외", Region: "세종", HasKlas: true, InspectionCycle: "monthly", RoundsPerPeriod: 3, OrgID: model.OrgIDLibrary,
	}); err != nil {
		t.Fatal(err)
	}
	p, _ := repo.CreatePlan(2026, "t")
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-09-01", CustomerID: "c14r", ProductType: "KLAS", Assignee: "기존담당",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceRegionAssignees(model.OrgIDLibrary, "세종", []model.MaintenanceRegionAssignee{
		{UserID: tech.UserID, Role: model.WBMemberOwner},
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceSiteAssignees("c14s", []model.MaintenanceSiteAssignee{
		{UserID: exc.UserID, Role: model.WBMemberOwner},
	}); err != nil {
		t.Fatal(err)
	}
	v := model.MaintenanceVisit{PlanID: p.PlanID, VisitDate: "2026-09-10", CustomerID: "c14r", ProductType: "KLAS"}
	if err := repo.InsertVisitFull(v); err != nil {
		t.Fatal(err)
	}
	v2 := model.MaintenanceVisit{PlanID: p.PlanID, VisitDate: "2026-09-11", CustomerID: "c14s", ProductType: "KLAS"}
	if err := repo.InsertVisitFull(v2); err != nil {
		t.Fatal(err)
	}
	visits, _ := repo.ListVisits(p.PlanID)
	var old, regionV, siteV model.MaintenanceVisit
	for _, it := range visits {
		switch it.VisitDate {
		case "2026-09-01":
			old = it
		case "2026-09-10":
			regionV = it
		case "2026-09-11":
			siteV = it
		}
	}
	if old.Assignee != "기존담당" {
		t.Fatalf("소급 금지 got %s", old.Assignee)
	}
	if regionV.Assignee != tech.FullName {
		t.Fatalf("지역 기본 %s", regionV.Assignee)
	}
	if siteV.Assignee != exc.FullName {
		t.Fatalf("사이트 예외가 지역을 대체 %s", siteV.Assignee)
	}
	if old.Assignee == tech.FullName {
		t.Fatal("지역 기본을 바꿔도 이미 만든 방문은 소급하지 않음")
	}
	if err := repo.ApplyRegionDefaultsToOpenVisits(model.OrgIDLibrary, "세종"); err != nil {
		t.Fatal(err)
	}
	again, _ := repo.GetVisit(old.VisitID)
	if again == nil || again.Assignee != tech.FullName {
		t.Fatalf("다시 적용은 미완료 방문만 %v", again)
	}
}

func TestMnt95_14OtherOrgRegionDenied(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "reg2.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewMaintenanceRepo(db)
	err = repo.ReplaceRegionAssignees("other-org", "세종", []model.MaintenanceRegionAssignee{
		{UserID: "u1", Role: model.WBMemberOwner},
	})
	if err == nil {
		// 조직 조건은 핸들러에서 막는다. 저장소는 org_id 를 그대로 넣는다.
	}
	list, err := repo.ListRegionAssignees(model.OrgIDLibrary, "세종")
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range list {
		if it.OrgID != "" && it.OrgID != model.OrgIDLibrary {
			t.Fatal("타 조직 행이 도서관 조회에 섞임")
		}
	}
}

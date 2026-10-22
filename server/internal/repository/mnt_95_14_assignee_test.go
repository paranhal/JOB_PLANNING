package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestMnt95_14VisitAssigneesMirrorAndGroup(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "a1.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := NewUserRepo(db)
	if err := users.EnsureAdmin("x"); err != nil {
		t.Fatal(err)
	}
	admin, _ := users.GetByUsername("admin")
	tech := &model.User{Username: "t14", PasswordHash: "x", FullName: "기술원", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	off := &model.User{Username: "o14", PasswordHash: "x", FullName: "사무원", Role: model.RoleOffice, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	for _, u := range []*model.User{tech, off} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c14a','사이트','사이트',1,?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	repo := NewMaintenanceRepo(db)
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "c14a", ShortName: "사이트", HasKlas: true, InspectionCycle: "monthly", RoundsPerPeriod: 2, OrgID: model.OrgIDLibrary,
	}); err != nil {
		t.Fatal(err)
	}
	p, err := repo.CreatePlan(2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	v := model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-09-01", CustomerID: "c14a", ProductType: "KLAS",
		Assignee: tech.FullName,
	}
	if err := repo.InsertVisitFull(v); err != nil {
		t.Fatal(err)
	}
	visits, _ := repo.ListVisits(p.PlanID)
	id := visits[0].VisitID
	if err := repo.ReplaceVisitAssignees(id, []model.MaintenanceVisitAssignee{
		{UserID: tech.UserID, AssigneeName: tech.FullName, Role: model.WBMemberOwner},
		{UserID: off.UserID, AssigneeName: off.FullName, Role: model.WBMemberSupport},
	}, ""); err != nil {
		t.Fatal(err)
	}
	got, err := repo.ListVisitAssignees(id)
	if err != nil || len(got) != 2 {
		t.Fatalf("assignees %v %v", got, err)
	}
	fresh, _ := repo.GetVisit(id)
	if fresh == nil || fresh.Assignee != tech.FullName {
		t.Fatalf("미러 assignee=%v", fresh)
	}
	if err := repo.ReplaceVisitAssignees(id, []model.MaintenanceVisitAssignee{
		{UserID: off.UserID, AssigneeName: off.FullName, Role: model.WBMemberOwner},
	}, ""); err != nil {
		t.Fatal(err)
	}
	fresh, _ = repo.GetVisit(id)
	if fresh.Assignee != off.FullName {
		t.Fatalf("주담당 변경 미러 %s", fresh.Assignee)
	}

	wg := NewWorkGroupRepo(db)
	g := &model.WorkGroup{OrgID: model.OrgIDLibrary, Name: "점검출장"}
	a := model.WorkGroupActor{UserID: admin.UserID, OrgID: model.OrgIDLibrary, Role: model.RoleOrgAdmin}
	if err := wg.CreateWithMembers(a, g, []string{tech.UserID, off.UserID}, tech.UserID); err != nil {
		t.Fatal(err)
	}
	v2 := model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-09-02", CustomerID: "c14a", ProductType: "KLAS", WorkGroupID: g.GroupID,
	}
	if err := repo.InsertVisitFull(v2); err != nil {
		t.Fatal(err)
	}
	visits, _ = repo.ListVisits(p.PlanID)
	var vid2 string
	for _, it := range visits {
		if it.VisitDate == "2026-09-02" {
			vid2 = it.VisitID
			if it.Assignee != tech.FullName {
				t.Fatalf("그룹장 주담당 %s", it.Assignee)
			}
			if it.WorkGroupID != g.GroupID {
				t.Fatalf("그룹 %s", it.WorkGroupID)
			}
		}
	}
	ms, _ := repo.ListVisitAssignees(vid2)
	if len(ms) < 2 {
		t.Fatalf("그룹 구성원 참여자 %v", ms)
	}
}

func TestMnt95_14DailyTaskKeepsManualAssignee(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "a2.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c14b','사이트','사이트',1)`); err != nil {
		t.Fatal(err)
	}
	mnt := NewMaintenanceRepo(db)
	p, _ := mnt.CreatePlan(2026, "t")
	v := model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-09-03", CustomerID: "c14b", ProductType: "KLAS", Assignee: "김담당",
	}
	if err := mnt.InsertVisitFull(v); err != nil {
		t.Fatal(err)
	}
	visits, _ := mnt.ListVisits(p.PlanID)
	wb := NewWBRepo(db)
	if _, err := wb.EnsureMaintenanceTasks(); err != nil {
		t.Fatal(err)
	}
	task, err := wb.GetTaskBySource(model.WBSourceMaintenance, visits[0].VisitID)
	if err != nil || task == nil {
		t.Fatal(err)
	}
	task.Assignee = "수동담당"
	task.AssigneeSource = model.WBAssigneeSourceManual
	if err := wb.UpdateTask(task); err != nil {
		t.Fatal(err)
	}
	if err := mnt.AssignOpenVisit(visits[0].VisitID, "2026-09-03", "방문담당"); err != nil {
		t.Fatal(err)
	}
	_, _ = wb.SyncMaintenanceTaskFromVisit(visits[0].VisitID)
	again, _ := wb.GetTaskBySource(model.WBSourceMaintenance, visits[0].VisitID)
	if again.Assignee != "수동담당" {
		t.Fatalf("수동 담당 덮어씀 %s", again.Assignee)
	}
}

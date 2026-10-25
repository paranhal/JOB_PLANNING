package repository

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func Test95_48_KeepRulesAndSkipReasons(t *testing.T) {
	repo, close := protectRepo(t, "t48-rules.db")
	defer close()
	p, err := repo.CreatePlan(2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-08-12", CustomerID: "c1", ProductType: "KLAS", Completed: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-08-13", CustomerID: "c1", Notes: "현장 메모",
	}); err != nil {
		t.Fatal(err)
	}
	drops, err := repo.ListDuplicateDrops(p.PlanID, 2026, 8)
	if err != nil || len(drops) != 1 {
		t.Fatalf("drops=%d err=%v", len(drops), err)
	}
	if drops[0].SkipReason != DupSkipRecord || drops[0].StatusLabel() != "제외 — 조치 기록 있음" {
		t.Fatalf("%+v", drops[0])
	}
	n, skipped, err := repo.CollapseDuplicateMonthVisits(p.PlanID, 2026, 8)
	if err != nil || n != 0 || skipped != 1 {
		t.Fatalf("n=%d skipped=%d err=%v", n, skipped, err)
	}

	if _, err := repo.db.Exec(`UPDATE maintenance_visits SET notes=NULL WHERE visit_date='2026-08-13'`); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.Exec(`UPDATE maintenance_visits SET start_time='10:00' WHERE visit_date='2026-08-13'`); err != nil {
		t.Fatal(err)
	}
	drops, _ = repo.ListDuplicateDrops(p.PlanID, 2026, 8)
	if len(drops) != 1 || drops[0].SkipReason != DupSkipRecord {
		t.Fatalf("시간 기록 제외 실패 %+v", drops)
	}
	if _, err := repo.db.Exec(`UPDATE maintenance_visits SET start_time='' WHERE visit_date='2026-08-13'`); err != nil {
		t.Fatal(err)
	}
	list, _ := repo.ListVisitsByMonth(p.PlanID, 2026, 8)
	var dropID string
	for _, v := range list {
		if v.VisitDate == "2026-08-13" {
			dropID = v.VisitID
		}
	}
	if err := repo.ReplaceVisitTargets(dropID, model.OrgIDLibrary, []model.MaintenanceVisitTarget{{Name: "게이트"}}); err != nil {
		t.Fatal(err)
	}
	drops, _ = repo.ListDuplicateDrops(p.PlanID, 2026, 8)
	if len(drops) != 1 || drops[0].SkipReason != DupSkipRecord {
		t.Fatalf("장비 제외 실패 %+v", drops)
	}

	p2, err := repo.CreatePlan(2027, "t27")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p2.PlanID, VisitDate: "2027-03-10", CustomerID: "c1", ProductType: "KLAS", Completed: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p2.PlanID, VisitDate: "2027-03-12", CustomerID: "c1", ProductType: "KLAS", Completed: true, DupReason: "추가", RoundNo: 1,
	}); err != nil {
		t.Fatal(err)
	}
	both, err := repo.ListDuplicateDrops(p2.PlanID, 2027, 3)
	if err != nil || len(both) != 1 || both[0].SkipReason != DupSkipBothDone {
		t.Fatalf("둘 다 완료 %+v err=%v", both, err)
	}
	if both[0].StatusLabel() != "제외 — 둘 다 완료" {
		t.Fatalf("라벨 %s", both[0].StatusLabel())
	}
	n, skipped, err = repo.CollapseDuplicateMonthVisits(p2.PlanID, 2027, 3)
	if err != nil || n != 0 || skipped != 1 {
		t.Fatalf("둘 다 완료 정리 n=%d skipped=%d", n, skipped)
	}
}

func Test95_48_DifferentRoundProductMonthNotDuplicate(t *testing.T) {
	repo, close := protectRepo(t, "t48-slot.db")
	defer close()
	if _, err := repo.db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c2','서산고','서산고등학교',1)`); err != nil {
		t.Fatal(err)
	}
	p, err := repo.CreatePlan(2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-08-10", CustomerID: "c1", ProductType: "KLAS", RoundNo: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-08-20", CustomerID: "c1", ProductType: "KLAS", RoundNo: 2, DupReason: "2회차",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-08-11", CustomerID: "c1", ProductType: "RFID", RoundNo: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-09-10", CustomerID: "c1", ProductType: "KLAS", RoundNo: 1,
	}); err != nil {
		t.Fatal(err)
	}
	aug, err := repo.ListDuplicateDrops(p.PlanID, 2026, 8)
	if err != nil || len(aug) != 0 {
		t.Fatalf("회차·제품이 다른데 중복 %d err=%v", len(aug), err)
	}
	sep, err := repo.ListDuplicateDrops(p.PlanID, 2026, 9)
	if err != nil || len(sep) != 0 {
		t.Fatalf("다른 달 단독 방문이 중복 %d", len(sep))
	}

	repo.Now = func() time.Time { return time.Date(2026, 10, 25, 12, 0, 0, 0, time.Local) }
	p3, err := repo.CreatePlan(2025, "y25")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p3.PlanID, VisitDate: "2025-10-12", CustomerID: "c2", ProductType: "KLAS", Completed: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p3.PlanID, VisitDate: "2025-10-13", CustomerID: "c2",
	}); err != nil {
		t.Fatal(err)
	}
	n, skipped, err := repo.CollapseDuplicateMonthVisits(p3.PlanID, 2025, 10)
	if err != nil || n != 1 || skipped != 0 {
		t.Fatalf("이전 연도 n=%d skipped=%d err=%v", n, skipped, err)
	}
}

func Test95_48_DeleteVisitStillProtectedAndFlagOff(t *testing.T) {
	repo, close := protectRepo(t, "t48-del.db")
	defer close()
	p, err := repo.CreatePlan(2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-08-10", CustomerID: "c1", ProductType: "KLAS",
	}); err != nil {
		t.Fatal(err)
	}
	visits, _ := repo.ListVisits(p.PlanID)
	if err := repo.DeleteVisit(visits[0].VisitID); !errors.Is(err, ErrVisitDeleteProtected) {
		t.Fatalf("일반 삭제 보호 깨짐: %v", err)
	}
	if err := repo.deleteVisitAllowProtectedDuplicate(visits[0].VisitID, false); !errors.Is(err, ErrVisitDeleteProtected) {
		t.Fatalf("flag=false 가 보호를 건너뜀: %v", err)
	}
}

func Test95_48_MonthTargetRemainsOnKeep(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "t48-mt.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('semi1','반기A','반기A',1,?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	repo := NewMaintenanceRepo(db)
	repo.Now = func() time.Time { return time.Date(2026, 10, 25, 12, 0, 0, 0, time.Local) }
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "semi1", ShortName: "반기A", InspectionCycle: "semi", HasKlas: true,
	}); err != nil {
		t.Fatal(err)
	}
	p, err := repo.CreatePlan(2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.RegisterMonthTargets(p, 10, model.MonthTargetSourceSite, "", []string{"semi1"}, "u", "테스터"); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-10-12", CustomerID: "semi1", ProductType: "KLAS", Completed: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-10-13", CustomerID: "semi1",
	}); err != nil {
		t.Fatal(err)
	}
	n, _, err := repo.CollapseDuplicateMonthVisits(p.PlanID, 2026, 10)
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	targets, err := repo.ListActiveMonthTargets(p.PlanID, 2026)
	if err != nil || len(targets) == 0 {
		t.Fatalf("month target 사라짐 %d err=%v", len(targets), err)
	}
	left, _ := repo.ListVisitsByMonth(p.PlanID, 2026, 10)
	if len(left) != 1 || left[0].VisitDate != "2026-10-12" {
		t.Fatalf("Keep 없음 %+v", left)
	}
}

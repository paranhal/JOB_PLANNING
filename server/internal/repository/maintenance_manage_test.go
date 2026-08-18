package repository

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestListPlansWithCountsAndCopy(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "copy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c1','가나도서관','가나도서관',1)`); err != nil {
		t.Fatal(err)
	}
	if err := NewMaintenanceRepo(db).UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "c1", ShortName: "가나", Region: "세종", HasKlas: true, InspectionCycle: "monthly",
	}); err != nil {
		t.Fatal(err)
	}
	repo := NewMaintenanceRepo(db)
	src, err := repo.CreatePlan(2026, "2026년 정기점검")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: src.PlanID, VisitDate: "2026-11-10", CustomerID: "c1", ProductType: "KLAS", Assignee: "최혜영",
	}); err != nil {
		t.Fatal(err)
	}
	dest, err := repo.CopyPlan(src.PlanID, 2027, "")
	if err != nil {
		t.Fatal(err)
	}
	if dest.PlanYear != 2027 || dest.Title != "2027년 정기점검" {
		t.Fatalf("복사 계획: %+v", dest)
	}
	copied, err := repo.ListVisits(dest.PlanID)
	if err != nil || len(copied) != 1 {
		t.Fatalf("복사 방문: %d err=%v", len(copied), err)
	}
	if copied[0].VisitDate != "" || copied[0].Assignee != "최혜영" || copied[0].ProductType != "KLAS" {
		t.Fatalf("날짜 없이 담당자만: %+v", copied[0])
	}
	items, err := repo.ListPlansWithCounts()
	if err != nil || len(items) != 2 {
		t.Fatalf("목록: %d err=%v", len(items), err)
	}
	for _, it := range items {
		if it.PlanYear == 2027 && it.VisitCount != 0 {
			t.Fatalf("날짜 없는 복사는 방문 건수에 넣지 않는다: %+v", it)
		}
		if it.PlanYear == 2026 && it.VisitCount != 1 {
			t.Fatalf("원본 방문 건수: %+v", it)
		}
	}
	if _, err := repo.CopyPlan(src.PlanID, 2027, ""); err == nil {
		t.Fatal("같은 연도 계획이 있는데 복사가 됨")
	}

	if err := repo.AssignSlot(model.MaintenanceVisit{
		PlanID: dest.PlanID, VisitDate: "2027-03-10", CustomerID: "c1", ProductType: "KLAS",
	}); err != nil {
		t.Fatal(err)
	}
	after, _ := repo.ListVisits(dest.PlanID)
	if len(after) != 1 || after[0].VisitDate != "2027-03-10" || after[0].Assignee != "최혜영" {
		t.Fatalf("초안에 날짜를 넣으면 1건이어야 함: %+v", after)
	}
}

func TestBulkAssigneeAndProtectedDelete(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "bulk.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c1','가나도서관','가나도서관',1)`); err != nil {
		t.Fatal(err)
	}
	repo := NewMaintenanceRepo(db)
	repo.Now = func() time.Time { return time.Date(2026, 8, 17, 0, 0, 0, 0, time.Local) }
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "c1", ShortName: "가나", Region: "세종", HasKlas: true, HasRfid: true, InspectionCycle: "monthly",
	}); err != nil {
		t.Fatal(err)
	}
	p, err := repo.CreatePlan(2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-08-10", CustomerID: "c1", ProductType: "KLAS", Assignee: "A",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-11-10", CustomerID: "c1", ProductType: "KLAS", Assignee: "A",
	}); err != nil {
		t.Fatal(err)
	}
	n, err := repo.BulkUpdateAssignees(p.PlanID, 2026, VisitFilter{Month: 11, Product: "KLAS"}, "최혜영")
	if err != nil || n != 1 {
		t.Fatalf("담당자 변경 n=%d err=%v", n, err)
	}
	nov, _ := repo.ListVisitsFiltered(p.PlanID, 2026, VisitFilter{Month: 11})
	if len(nov) != 1 || nov[0].Assignee != "최혜영" {
		t.Fatalf("11월 담당자: %+v", nov)
	}
	aug, _ := repo.ListVisitsFiltered(p.PlanID, 2026, VisitFilter{Month: 8})
	if len(aug) != 1 || aug[0].Assignee != "A" {
		t.Fatalf("8월은 그대로: %+v", aug)
	}

	all, _ := repo.ListVisitsFiltered(p.PlanID, 2026, VisitFilter{})
	ids := []string{all[0].VisitID, all[1].VisitID}
	deleted, skipped, err := repo.DeleteVisitIDs(ids)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 || skipped != 1 {
		t.Fatalf("deleted=%d skipped=%d", deleted, skipped)
	}
}

package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestQuarterObligationsAnyMonthInQuarter(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "q.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c1','법제처','법제처',1),('c2','월간관','월간관',1)`); err != nil {
		t.Fatal(err)
	}
	repo := NewMaintenanceRepo(db)
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "c1", ShortName: "법제처", HasKlas: true, InspectionCycle: "quarterly",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "c2", ShortName: "월간", HasKlas: true, InspectionCycle: "monthly",
	}); err != nil {
		t.Fatal(err)
	}
	p, err := repo.CreatePlan(2026, "t")
	if err != nil {
		t.Fatal(err)
	}

	empty, err := repo.QuarterObligations(p.PlanID, 2026, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 1 || empty[0].CustomerID != "c1" || empty[0].SlotKey != "klas" {
		t.Fatalf("Q3 미충족=%v", empty)
	}

	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-09-10", CustomerID: "c1", ProductType: "KLAS",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-09-11", CustomerID: "c2", ProductType: "KLAS",
	}); err != nil {
		t.Fatal(err)
	}
	filled, err := repo.QuarterObligations(p.PlanID, 2026, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(filled) != 0 {
		t.Fatalf("9월 일정 확정이면 Q3 충족, got %v", filled)
	}

	q4, err := repo.QuarterObligations(p.PlanID, 2026, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(q4) != 1 || q4[0].CustomerID != "c1" {
		t.Fatalf("Q3 방문은 Q4를 채우지 않음: %v", q4)
	}

	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-10-05", CustomerID: "c1", ProductType: "KLAS", Completed: false,
	}); err != nil {
		t.Fatal(err)
	}
	q4ok, err := repo.QuarterObligations(p.PlanID, 2026, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(q4ok) != 0 {
		t.Fatalf("미완료여도 일정 확정이면 Q4 충족, got %v", q4ok)
	}
}

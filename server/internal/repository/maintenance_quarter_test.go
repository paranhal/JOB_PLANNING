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
	if !repo.HasVisitInCalendarQuarter(p.PlanID, "c1", "KLAS", 2026, 4) {
		t.Fatal("Q4 10월 방문이 분기 방문으로 안 잡힌다")
	}
	if repo.HasVisitInCalendarQuarter(p.PlanID, "c1", "KLAS", 2026, 1) {
		t.Fatal("Q4 방문이 Q1로 잡히면 안 된다")
	}
	if len(q4ok) != 0 {
		t.Fatalf("미완료여도 일정 확정이면 Q4 충족, got %v", q4ok)
	}
}

func TestListOpenUnassignedSlotsHidesQuarterUntilLastMonth(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "open.db"))
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

	oct, err := repo.ListOpenUnassignedSlots(p.PlanID, 2026, 10)
	if err != nil {
		t.Fatal(err)
	}
	if n := countSlotCustomer(oct, "c1"); n != 0 {
		t.Fatalf("10월 미배정에 분기 슬롯이 있으면 안 됨: %v", oct)
	}
	if n := countSlotCustomer(oct, "c2"); n != 1 {
		t.Fatalf("10월 월간 미배정=%d want 1", n)
	}
	pending, err := repo.PendingQuarterSlots(p.PlanID, 2026, 4)
	if err != nil || len(pending) != 1 {
		t.Fatalf("Q4 확인 대기=%d err=%v", len(pending), err)
	}

	dec, err := repo.ListOpenUnassignedSlots(p.PlanID, 2026, 12)
	if err != nil {
		t.Fatal(err)
	}
	if n := countSlotCustomer(dec, "c1"); n != 1 {
		t.Fatalf("12월에는 분기 미확정이 올라가야 함: %v", dec)
	}
	var qSlot model.UnassignedMonthSlot
	for _, s := range dec {
		if s.CustomerID == "c1" {
			qSlot = s
		}
	}
	if !qSlot.UnconfirmedQuarter {
		t.Fatal("12월 분기 슬롯은 UnconfirmedQuarter")
	}

	if err := repo.SkipQuarterSlot(p.PlanID, "c1", "klas", "이번 분기는 방문 안 함", "테스터", 2026, 4); err != nil {
		t.Fatal(err)
	}
	if err := repo.SkipQuarterSlot(p.PlanID, "c1", "klas", "사유 수정", "테스터", 2026, 4); err != nil {
		t.Fatal(err)
	}
	after, err := repo.ListOpenUnassignedSlots(p.PlanID, 2026, 12)
	if err != nil {
		t.Fatal(err)
	}
	if n := countSlotCustomer(after, "c1"); n != 0 {
		t.Fatalf("건너뜀 후 12월 미배정에 남음: %v", after)
	}
	pending2, err := repo.PendingQuarterSlots(p.PlanID, 2026, 4)
	if err != nil || len(pending2) != 0 {
		t.Fatalf("건너뜀 후 확인 대기=%d err=%v", len(pending2), err)
	}

	applyMaintenanceQuarterDecisionsV76(db)
	applyMaintenanceQuarterDecisionsV76(db)
}

func countSlotCustomer(slots []model.UnassignedMonthSlot, customerID string) int {
	n := 0
	for _, s := range slots {
		if s.CustomerID == customerID {
			n++
		}
	}
	return n
}

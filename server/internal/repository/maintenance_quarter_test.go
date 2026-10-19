package repository

import (
	"path/filepath"
	"strings"
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

func TestQuarterUnassignedDisplayMonthsAndCycles(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "disp.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('cq','분기관','분기관',1,'O01'),('cm','월간관','월간관',1,'O01'),
		       ('cb','격월관','격월관',1,'O01'),('cx','타조직','타조직',1,'O02')`); err != nil {
		t.Fatal(err)
	}
	repo := NewMaintenanceRepo(db)
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "cq", ShortName: "분기관", HasKlas: true, InspectionCycle: "quarterly",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "cm", ShortName: "월간", HasKlas: true, InspectionCycle: "monthly",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "cb", ShortName: "격월", HasKlas: true, InspectionCycle: "odd_bimonthly",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "cx", ShortName: "타조직", HasKlas: true, InspectionCycle: "quarterly",
	}); err != nil {
		t.Fatal(err)
	}
	p, err := repo.CreatePlan(2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	wb := NewWorkBoardRepo(db)

	for _, month := range []int{1, 2} {
		open, err := repo.ListOpenUnassignedSlots(p.PlanID, 2026, month)
		if err != nil {
			t.Fatal(err)
		}
		if n := countSlotCustomer(open, "cq"); n != 0 {
			t.Fatalf("%d월 분기 미배정=%d", month, n)
		}
		if n := countSlotCustomer(open, "cm"); n != 1 {
			t.Fatalf("%d월 월별 미배정=%d", month, n)
		}
		pending, err := repo.PendingQuarterSlots(p.PlanID, 2026, 1)
		if err != nil || countSlotCustomer(pending, "cq") != 1 {
			t.Fatalf("%d월 QuarterPending cq=%v err=%v", month, pending, err)
		}
		alerts, err := wb.CollectQuarterUnassignedAlerts("O01", 2026, month)
		if err != nil || len(alerts) != 0 {
			t.Fatalf("%d월 알람=%d err=%v", month, len(alerts), err)
		}
		oddDue := month%2 == 1
		if n := countSlotCustomer(open, "cb"); (n == 1) != oddDue {
			t.Fatalf("%d월 격월 미배정=%d due=%v", month, n, oddDue)
		}
	}

	mar, err := repo.ListOpenUnassignedSlots(p.PlanID, 2026, 3)
	if err != nil {
		t.Fatal(err)
	}
	if n := countSlotCustomer(mar, "cq"); n != 1 {
		t.Fatalf("3월 분기 미배정=%d", n)
	}
	alerts, err := wb.CollectQuarterUnassignedAlerts("O01", 2026, 3)
	if err != nil || len(alerts) != 1 {
		t.Fatalf("3월 알람=%d err=%v", len(alerts), err)
	}
	if !strings.Contains(alerts[0].Title, "분기 점검 미확정") {
		t.Fatalf("알람 제목=%s", alerts[0].Title)
	}
	otherAlerts, err := wb.CollectQuarterUnassignedAlerts("O01", 2026, 3)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range otherAlerts {
		if it.RefID == "cx:klas" {
			t.Fatalf("다른 조직 알람 누수 %+v", it)
		}
	}
	o2, err := wb.CollectQuarterUnassignedAlerts("O02", 2026, 3)
	if err != nil || len(o2) != 1 {
		t.Fatalf("O02 알람=%d err=%v", len(o2), err)
	}

	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-01-15", CustomerID: "cq", ProductType: "KLAS",
	}); err != nil {
		t.Fatal(err)
	}
	afterVisit, err := repo.ListOpenUnassignedSlots(p.PlanID, 2026, 3)
	if err != nil {
		t.Fatal(err)
	}
	if n := countSlotCustomer(afterVisit, "cq"); n != 0 {
		t.Fatalf("1월 방문 후 3월 미배정=%d", n)
	}

	p2, err := repo.CreatePlan(2027, "t2")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SkipQuarterSlot(p2.PlanID, "cq", "klas", "이번 분기 건너뜀", "테스터", 2027, 1); err != nil {
		t.Fatal(err)
	}
	skipped, err := repo.ListOpenUnassignedSlots(p2.PlanID, 2027, 3)
	if err != nil {
		t.Fatal(err)
	}
	if n := countSlotCustomer(skipped, "cq"); n != 0 {
		t.Fatalf("건너뜀 후 미배정=%d", n)
	}
	skipAlerts, err := wb.CollectQuarterUnassignedAlerts("O01", 2027, 3)
	if err != nil || len(skipAlerts) != 0 {
		t.Fatalf("건너뜀 후 알람=%d", len(skipAlerts))
	}
}

func TestPeriodUnassignedSemiAndYearly(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "period.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('cs','반기관','반기관',1,'O01'),('cy','연간관','연간관',1,'O01')`); err != nil {
		t.Fatal(err)
	}
	repo := NewMaintenanceRepo(db)
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "cs", ShortName: "반기관", HasKlas: true, InspectionCycle: "semi",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "cy", ShortName: "연간관", HasKlas: true, InspectionCycle: "yearly",
	}); err != nil {
		t.Fatal(err)
	}
	p, err := repo.CreatePlan(2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	jan, err := repo.ListOpenUnassignedSlots(p.PlanID, 2026, 1)
	if err != nil {
		t.Fatal(err)
	}
	if countSlotCustomer(jan, "cs") != 0 || countSlotCustomer(jan, "cy") != 0 {
		t.Fatalf("1월 미배정에 반기/연간이 있으면 안 됨: %v", jan)
	}
	pend, err := repo.PendingPeriodSlots(p.PlanID, 2026, 1)
	if err != nil {
		t.Fatal(err)
	}
	if countSlotCustomer(pend, "cs") != 1 || countSlotCustomer(pend, "cy") != 1 {
		t.Fatalf("1월 대기는 보여주기만: %v", pend)
	}
	jun, err := repo.ListOpenUnassignedSlots(p.PlanID, 2026, 6)
	if err != nil {
		t.Fatal(err)
	}
	if countSlotCustomer(jun, "cs") != 1 {
		t.Fatalf("6월 반기 미확정=%v", jun)
	}
	if countSlotCustomer(jun, "cy") != 0 {
		t.Fatalf("6월에 연간이 승격되면 안 됨")
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-02-10", CustomerID: "cs", ProductType: "KLAS",
	}); err != nil {
		t.Fatal(err)
	}
	jun2, err := repo.ListOpenUnassignedSlots(p.PlanID, 2026, 6)
	if err != nil {
		t.Fatal(err)
	}
	if countSlotCustomer(jun2, "cs") != 0 {
		t.Fatalf("2월 방문이면 상반기 충족: %v", jun2)
	}
	dec, err := repo.ListOpenUnassignedSlots(p.PlanID, 2026, 12)
	if err != nil {
		t.Fatal(err)
	}
	if countSlotCustomer(dec, "cy") != 1 {
		t.Fatalf("12월 연간 미확정=%v", dec)
	}
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

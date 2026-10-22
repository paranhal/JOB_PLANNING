package repository

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func setupMnt9514(t *testing.T, name string) (*MaintenanceRepo, *sql.DB) {
	t.Helper()
	db, err := InitDB(filepath.Join(t.TempDir(), name))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c14','해미도서관','해미도서관',1,?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	return NewMaintenanceRepo(db), db
}

func TestMnt95_14SameMonthDifferentRoundAllowed(t *testing.T) {
	repo, _ := setupMnt9514(t, "r1.db")
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "c14", ShortName: "해미", HasKlas: true, InspectionCycle: "monthly", RoundsPerPeriod: 2,
	}); err != nil {
		t.Fatal(err)
	}
	p, err := repo.CreatePlan(2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-08-13", CustomerID: "c14", ProductType: "KLAS", RoundNo: 1,
	}); err != nil {
		t.Fatal(err)
	}
	err = repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-08-20", CustomerID: "c14", ProductType: "KLAS", RoundNo: 2,
	})
	if err != nil {
		t.Fatalf("회차가 다르면 같은 달 허용, got %v", err)
	}
}

func TestMnt95_14SameDayDifferentProductAllowed(t *testing.T) {
	repo, _ := setupMnt9514(t, "r2.db")
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "c14", ShortName: "해미", HasKlas: true, HasRfid: true, InspectionCycle: "monthly",
	}); err != nil {
		t.Fatal(err)
	}
	p, err := repo.CreatePlan(2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-08-13", CustomerID: "c14", ProductType: "KLAS",
	}); err != nil {
		t.Fatal(err)
	}
	err = repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-08-13", CustomerID: "c14", ProductType: "앤로보틱스",
	})
	if err != nil {
		t.Fatalf("같은 날 다른 제품군 허용, got %v", err)
	}
}

func TestMnt95_14QuarterSecondRoundOtherMonth(t *testing.T) {
	repo, _ := setupMnt9514(t, "r3.db")
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "c14", ShortName: "해미", HasKlas: true, InspectionCycle: "quarterly", RoundsPerPeriod: 2,
	}); err != nil {
		t.Fatal(err)
	}
	p, err := repo.CreatePlan(2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-07-10", CustomerID: "c14", ProductType: "KLAS", RoundNo: 1,
	}); err != nil {
		t.Fatal(err)
	}
	slots, err := repo.QuarterObligations(p.PlanID, 2026, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) == 0 {
		t.Fatal("분기 회차 2인데 1건만 있으면 주기 미충족이어야 함")
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-08-15", CustomerID: "c14", ProductType: "KLAS", RoundNo: 2,
	}); err != nil {
		t.Fatalf("분기 내 다른 달 2회차 허용, got %v", err)
	}
	slots, _ = repo.QuarterObligations(p.PlanID, 2026, 3)
	if len(slots) != 0 {
		t.Fatalf("두 회차면 주기 충족, got %v", slots)
	}
}

func TestMnt95_14CompletedThenSameMonthNewRound(t *testing.T) {
	repo, _ := setupMnt9514(t, "r4.db")
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "c14", ShortName: "해미", HasKlas: true, InspectionCycle: "monthly", RoundsPerPeriod: 2,
	}); err != nil {
		t.Fatal(err)
	}
	p, err := repo.CreatePlan(2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-08-13", CustomerID: "c14", ProductType: "KLAS", Completed: true, RoundNo: 1,
	}); err != nil {
		t.Fatal(err)
	}
	err = repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-08-27", CustomerID: "c14", ProductType: "KLAS", RoundNo: 2,
	})
	if err != nil {
		t.Fatalf("완료 후 같은 달 다음 회차 허용, got %v", err)
	}
}

func TestMnt95_14SameRoundRejectedWithLink(t *testing.T) {
	repo, _ := setupMnt9514(t, "r5.db")
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "c14", ShortName: "해미", HasKlas: true, InspectionCycle: "monthly", RoundsPerPeriod: 2,
	}); err != nil {
		t.Fatal(err)
	}
	p, err := repo.CreatePlan(2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-08-13", CustomerID: "c14", ProductType: "KLAS", RoundNo: 1,
	}); err != nil {
		t.Fatal(err)
	}
	err = repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-08-20", CustomerID: "c14", ProductType: "KLAS", RoundNo: 1,
	})
	var md MonthDupError
	if !errors.As(err, &md) {
		t.Fatalf("같은 회차 거부, got %v", err)
	}
	msg := err.Error()
	if md.Existing == nil || (!strings.Contains(msg, "월에 이미") && !strings.Contains(msg, "회차")) {
		t.Fatalf("안내 문구 %v", err)
	}
}

func TestMnt95_14ExceedRoundsNeedsReason(t *testing.T) {
	repo, _ := setupMnt9514(t, "r6.db")
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "c14", ShortName: "해미", HasKlas: true, InspectionCycle: "monthly", RoundsPerPeriod: 1,
	}); err != nil {
		t.Fatal(err)
	}
	p, err := repo.CreatePlan(2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-08-13", CustomerID: "c14", ProductType: "KLAS",
	}); err != nil {
		t.Fatal(err)
	}
	err = repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-08-20", CustomerID: "c14", ProductType: "KLAS", RoundNo: 2,
	})
	if err == nil {
		t.Fatal("회차 수 초과는 사유 없이 거부")
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-08-20", CustomerID: "c14", ProductType: "KLAS", RoundNo: 2, DupReason: "임시 점검",
	}); err != nil {
		t.Fatalf("사유 있으면 초과 허용: %v", err)
	}
}

func TestMnt95_14LegacyRoundNoIsOne(t *testing.T) {
	repo, _ := setupMnt9514(t, "r7.db")
	p, err := repo.CreatePlan(2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-08-13", CustomerID: "c14", ProductType: "KLAS",
	}); err != nil {
		t.Fatal(err)
	}
	visits, err := repo.ListVisits(p.PlanID)
	if err != nil || len(visits) != 1 {
		t.Fatalf("visits %v %v", visits, err)
	}
	if visits[0].RoundNo != 1 {
		t.Fatalf("기존 방문은 1회차로 해석 got %d", visits[0].RoundNo)
	}
}

func TestMnt95_14PeriodCloseWhenRoundsDone(t *testing.T) {
	repo, _ := setupMnt9514(t, "r8.db")
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "c14", ShortName: "해미", HasKlas: true, InspectionCycle: "quarterly", RoundsPerPeriod: 2,
	}); err != nil {
		t.Fatal(err)
	}
	p, err := repo.CreatePlan(2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-07-10", CustomerID: "c14", ProductType: "KLAS", RoundNo: 1, Completed: true,
	}); err != nil {
		t.Fatal(err)
	}
	if repo.PeriodClosed(p.PlanID, "c14", "KLAS", 2026, 7) {
		t.Fatal("한 회차만 완료면 주기 미마감")
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-08-10", CustomerID: "c14", ProductType: "KLAS", RoundNo: 2, Completed: true,
	}); err != nil {
		t.Fatal(err)
	}
	if !repo.PeriodClosed(p.PlanID, "c14", "KLAS", 2026, 7) {
		t.Fatal("두 회차 완료면 주기 마감")
	}
}

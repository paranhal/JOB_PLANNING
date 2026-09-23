package repository

import (
	"errors"
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestMonthDuplicateWarnAndReason(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "mnt_dup.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, _ = db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?,?,?,1)`,
		"c1", "해미도서관", "해미도서관")
	repo := NewMaintenanceRepo(db)
	p, err := repo.CreatePlan(2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	v := model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-08-13", CustomerID: "c1", ProductType: "K-LAS", Completed: true,
	}
	if err := repo.InsertVisitFull(v); err != nil {
		t.Fatal(err)
	}
	dup, err := repo.MonthDuplicate(p.PlanID, "c1", "KLAS", "2026-08-20", "")
	if err != nil || dup == nil {
		t.Fatalf("dup=%v err=%v", dup, err)
	}
	err = repo.AssignSlot(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-08-20", CustomerID: "c1", ProductType: "K-LAS",
	})
	var md MonthDupError
	if !errors.As(err, &md) {
		t.Fatalf("want MonthDupError got %v", err)
	}
	if err := repo.AssignSlot(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-08-20", CustomerID: "c1", ProductType: "K-LAS",
		DupReason: "연 2회 계약",
	}); err != nil {
		t.Fatal(err)
	}
	visits, _ := repo.ListVisits(p.PlanID)
	found := false
	for _, it := range visits {
		if it.VisitDate == "2026-08-20" && it.DupReason == "연 2회 계약" {
			found = true
		}
	}
	if !found {
		t.Fatal("사유 저장 실패")
	}
	if d, _ := repo.MonthDuplicate(p.PlanID, "c1", "앤로보틱스", "2026-08-20", ""); d != nil {
		t.Fatal("다른 제품군은 경고 없음")
	}
}

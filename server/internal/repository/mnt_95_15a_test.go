package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestMnt95_15AUnassignedKeepsEmptyRegion(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "u15a.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('u15e','빈월','빈월',1,?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	repo := NewMaintenanceRepo(db)
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "u15e", ShortName: "빈월", HasKlas: true, InspectionCycle: "monthly", Region: "",
	}); err != nil {
		t.Fatal(err)
	}
	p, err := repo.CreatePlan(2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	slots, err := repo.ListOpenUnassignedSlots(p.PlanID, 2026, 8)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range slots {
		if s.CustomerID == "u15e" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("지역 없는 월 사이트가 미배정에서 빠짐 n=%d", len(slots))
	}
}

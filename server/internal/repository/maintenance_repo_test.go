package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestMaintenanceRepo_PlanAndVisits(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?,?,?,1)`,
		"cust_a", "Alpha Library", "Alpha Library")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?,?,?,1)`,
		"cust_b", "Beta School", "Beta School")
	if err != nil {
		t.Fatal(err)
	}

	repo := NewMaintenanceRepo(db)
	cfg := &model.MaintenanceSiteConfig{
		CustomerID: "cust_a", ShortName: "알파", Region: "A권역",
		HasKlas: true, EntryCategory: "normal",
	}
	if err := repo.UpsertSiteConfig(cfg); err != nil {
		t.Fatal(err)
	}
	cfg2 := &model.MaintenanceSiteConfig{
		CustomerID: "cust_b", ShortName: "베타", Region: "B권역",
		HasRfid: true, EntryCategory: "fixed", FixedRule: "LAST_MONDAY_OF_MONTH",
	}
	if err := repo.UpsertSiteConfig(cfg2); err != nil {
		t.Fatal(err)
	}

	list, err := repo.ListSiteConfigs()
	if err != nil || len(list) != 2 {
		t.Fatalf("ListSiteConfigs: len=%d err=%v", len(list), err)
	}

	p, err := repo.CreatePlan(2027, "테스트 계획")
	if err != nil {
		t.Fatal(err)
	}
	if p.PlanYear != 2027 {
		t.Fatalf("plan year: %d", p.PlanYear)
	}

	_, err = repo.CreatePlan(2027, "dup")
	if err == nil {
		t.Fatal("expected duplicate year error")
	}

	if err := repo.InsertVisit(p.PlanID, "2027-03-15", "cust_a", 0, 0, "normal", "수동"); err != nil {
		t.Fatal(err)
	}

	visits, err := repo.ListVisits(p.PlanID)
	if err != nil || len(visits) != 1 {
		t.Fatalf("ListVisits: len=%d err=%v", len(visits), err)
	}
	if visits[0].ShortName != "알파" {
		t.Fatalf("short name: %q", visits[0].ShortName)
	}

	mv, err := repo.ListVisitsByMonth(p.PlanID, 2027, 3)
	if err != nil || len(mv) != 1 {
		t.Fatalf("ListVisitsByMonth: len=%d err=%v", len(mv), err)
	}

	if err := repo.DeleteVisit(visits[0].VisitID); err != nil {
		t.Fatal(err)
	}
	visits, _ = repo.ListVisits(p.PlanID)
	if len(visits) != 0 {
		t.Fatalf("after delete: len=%d", len(visits))
	}

	if err := repo.DeletePlan(p.PlanID); err != nil {
		t.Fatal(err)
	}
}

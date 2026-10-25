package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestMnt95_47VisitTargetsReplaceAndIdempotent(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "t47.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	applyMaintenanceVisitActionV90(db)
	applyMaintenanceVisitActionV90(db)
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c47','서산고','서산고',1,?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_plans (plan_id, plan_year, title, status)
		VALUES ('MP47', 2026, 'p', 'approved')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits (visit_id, plan_id, visit_date, customer_id, sort_order, completed)
		VALUES ('v47','MP47','2026-10-12','c47',0,0)`); err != nil {
		t.Fatal(err)
	}
	repo := NewMaintenanceRepo(db)
	err = repo.ReplaceVisitTargets("v47", model.OrgIDLibrary, []model.MaintenanceVisitTarget{
		{Name: "A"}, {Name: "B"}, {Name: "C"}, {Name: "D"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.ListVisitTargets("v47")
	if err != nil || len(got) != 4 {
		t.Fatalf("%d %v", len(got), err)
	}
	if line := model.FormatVisitTargetsLine(got); line != "A, B, C 외 1대" {
		t.Fatalf("%q", line)
	}
	too := make([]model.MaintenanceVisitTarget, model.MaxVisitTargets+1)
	for i := range too {
		too[i] = model.MaintenanceVisitTarget{Name: "x"}
	}
	if err := repo.ReplaceVisitTargets("v47", model.OrgIDLibrary, too); err == nil {
		t.Fatal("50 초과 허용")
	}
}

func TestMnt95_47VisitTimesRoundTrip(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "t47t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c47','서산고','서산고',1,?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_plans (plan_id, plan_year, title, status)
		VALUES ('MP47', 2026, 'p', 'approved')`); err != nil {
		t.Fatal(err)
	}
	repo := NewMaintenanceRepo(db)
	id, err := repo.InsertVisitFullID(model.MaintenanceVisit{
		PlanID: "MP47", VisitDate: "2026-10-12", CustomerID: "c47", Assignee: "최혜영", ProductType: "KLAS",
	})
	if err != nil {
		t.Fatal(err)
	}
	v, err := repo.GetVisit(id)
	if err != nil {
		t.Fatal(err)
	}
	v.StartTime, v.EndTime, v.TravelMin, v.DurationMin = "10:00", "11:30", 40, 90
	if err := repo.UpdateVisit(*v); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetVisit(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.StartTime != "10:00" || got.EndTime != "11:30" || got.TravelMin != 40 || got.DurationMin != 90 {
		t.Fatalf("%+v", got)
	}
}

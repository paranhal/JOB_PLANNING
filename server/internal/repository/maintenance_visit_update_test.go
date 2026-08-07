package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestUpdateVisitFields(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "mnt-upd.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(
		`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?,?,?,1)`,
		"c1", "가나도서관", "가나도서관"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?,?,?,1)`,
		"c2", "다라도서관", "다라도서관"); err != nil {
		t.Fatal(err)
	}

	repo := NewMaintenanceRepo(db)
	p, err := repo.CreatePlan(2026, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-08-10", CustomerID: "c1",
		ProductType: "KLAS", Assignee: "양기헌", EntryCategory: "normal",
	}); err != nil {
		t.Fatal(err)
	}
	visits, err := repo.ListVisits(p.PlanID)
	if err != nil || len(visits) != 1 {
		t.Fatalf("setup: %v len=%d", err, len(visits))
	}
	v := visits[0]

	v.VisitDate = "2026-08-15"
	v.CustomerID = "c2"
	v.ProductType = "RFID"
	v.Assignee = "최혜영"
	v.Notes = "일정 변경"
	v.Completed = true
	v.CompletedDate = "2026-08-14"
	if err := repo.UpdateVisit(v); err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetVisit(v.VisitID)
	if err != nil || got == nil {
		t.Fatalf("get: %v", err)
	}
	if got.VisitDate != "2026-08-15" || got.CustomerID != "c2" {
		t.Fatalf("date/customer: %+v", got)
	}
	if got.ProductType != "RFID" || got.Assignee != "최혜영" {
		t.Fatalf("product/assignee: %+v", got)
	}
	if !got.Completed || got.CompletedDate != "2026-08-14" || got.Notes != "일정 변경" {
		t.Fatalf("completed/notes: %+v", got)
	}
}

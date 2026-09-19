package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestFillCustomerSalesView(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "cust-sales-view.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('C1','도서관','도서관',1), ('C2','학교','학교',1)`); err != nil {
		t.Fatal(err)
	}
	repo := NewSalesRepo(db)
	open := &model.SalesProject{
		Name: "진행", CustomerID: "C1", Stage: model.SalesStageLead, Status: model.SalesStatusActive,
	}
	if err := repo.Create(open); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO sales_projects (sales_id, name, stage, status, customer_id, expected_amount)
		VALUES ('SP-WON','수주','won','active','C1',1500)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO sales_activities (activity_id, sales_id, activity_date, activity_type, title, created_by)
		VALUES ('SA1', ?, '2026-09-01', 'call', '통화', 't'),
		       ('SA2', 'SP-WON', '2026-09-10', 'visit', '방문', 't')`, open.SalesID); err != nil {
		t.Fatal(err)
	}

	items := []model.CustomerListItem{
		{CustomerID: "C1", OrgName: "도서관"},
		{CustomerID: "C2", OrgName: "학교"},
	}
	if err := repo.FillCustomerSalesView(items); err != nil {
		t.Fatal(err)
	}
	if items[0].SalesOpenCount != 1 {
		t.Fatalf("진행 중=%d", items[0].SalesOpenCount)
	}
	if items[0].SalesWonAmount != 1500 {
		t.Fatalf("수주액=%d", items[0].SalesWonAmount)
	}
	if items[0].SalesLastActivity != "2026-09-10" {
		t.Fatalf("마지막 활동=%q", items[0].SalesLastActivity)
	}
	if items[1].SalesOpenCount != 0 || items[1].SalesWonAmount != 0 || items[1].SalesLastActivity != "" {
		t.Fatalf("다른 고객에 값이 붙었다: %+v", items[1])
	}
}

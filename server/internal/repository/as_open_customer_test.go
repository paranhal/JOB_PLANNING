package repository

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestListOpenByCustomerIncludesPartialAndExcludesOtherOrg(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "open.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c1','도서관','도서관',1,'O01'),('c2','다른','다른',1,'O99')`); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err = db.Exec(`INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, status, assigned_to, org_id, symptom)
		VALUES
		('A1','R1','c1',?,'in_progress','김기술','O01','카드 안 나옴'),
		('A2','R2','c1',?,'partial_complete','김기술','O01','검색 오류'),
		('A3','R3','c1',?,'completed','김기술','O01','끝난 건'),
		('B1','R4','c1',?,'in_progress','이기술','O99','타조직')`, now, now, now, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_plans (plan_id, plan_year, title, status) VALUES ('P1',2026,'계획','active')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits (visit_id, plan_id, visit_date, customer_id, completed, assignee)
		VALUES ('V1','P1','2026-10-09','c1',0,'이기술'),('V2','P1','2026-10-01','c1',1,'이기술')`); err != nil {
		t.Fatal(err)
	}

	asRepo := NewASRepo(db)
	open, err := asRepo.ListOpenByCustomer(model.OrgIDLibrary, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 2 {
		t.Fatalf("open=%d want 2 (부분완료 포함, 완료·타조직 제외)", len(open))
	}
	zero, err := asRepo.ListOpenByCustomer(model.OrgIDLibrary, "c2")
	if err != nil {
		t.Fatal(err)
	}
	if len(zero) != 0 {
		t.Fatalf("다른 기관=%d", len(zero))
	}
	visits, err := NewMaintenanceRepo(db).ListOpenByCustomer(model.OrgIDLibrary, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if len(visits) != 1 {
		t.Fatalf("점검 미완료=%d want 1", len(visits))
	}
}

package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestListBySiteCustomerActiveAndIncludeClosed(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "ps-list.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	org := model.OrgIDLibrary
	if _, err := db.Exec(`INSERT INTO work_projects (project_id, name, status, org_id, is_test, is_paid, plan_year)
		VALUES ('WP-A','사업A','active',?,0,1,2026),
		       ('WP-B','사업B','active',?,0,1,2025),
		       ('WP-C','사업C','active',?,0,1,2026),
		       ('WP-OLD','닫힌','complete',?,0,1,2024)`, org, org, org, org); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('CX','사이트X','사이트X',1,?)`, org); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO project_sites(id,project_id,customer_id,site_name,status,org_id,is_active)
		VALUES ('S-A','WP-A','CX','사이트X','in_progress',?,1),
		       ('S-B','WP-B','CX','사이트X','in_progress',?,1),
		       ('S-OLD','WP-OLD','CX','사이트X','in_progress',?,1)`, org, org, org); err != nil {
		t.Fatal(err)
	}
	r := NewProjectRepo(db)
	list, rec, err := r.ListBySiteCustomer("CX", "WP-OLD")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, p := range list {
		got[p.ProjectID] = true
	}
	if !got["WP-A"] || !got["WP-B"] || !got["WP-OLD"] {
		t.Fatalf("list=%v rec=%v", got, rec)
	}
	if got["WP-C"] {
		t.Fatal("미등록 C")
	}
	ok, err := r.CustomerOnActiveSite("CX", "WP-A")
	if err != nil || !ok {
		t.Fatalf("A on site: %v %v", ok, err)
	}
	ok, err = r.CustomerOnActiveSite("CX", "WP-C")
	if err != nil || ok {
		t.Fatalf("C not on site: %v %v", ok, err)
	}
}

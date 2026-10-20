package repository

import (
	"path/filepath"
	"strconv"
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

func TestSearchCustomersWithoutConfig_OrgLimitAndPending(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "siteq.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O03','O03','SI', 'business',1,30)`); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 21; i++ {
		id := "gy" + strconv.Itoa(i)
		name := "계룡학교" + strconv.Itoa(i)
		if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id, addr_sido)
			VALUES (?,?,?,1,'O03','충남')`, id, name, name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('gy-done','계룡고등학교', '계룡고등학교',1,'O03'),
		       ('gy-o01','계룡도서관','계룡도서관',1,'O01'),
		       ('gy-off','계룡휴면','계룡휴면',0,'O03'),
		       ('gy-pt','계룡협력','계룡협력',1,'O03')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE customers SET party_kind='partner' WHERE customer_id='gy-pt'`); err != nil {
		t.Fatal(err)
	}
	repo := NewMaintenanceRepo(db)
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "gy-done", ShortName: "계룡고", InspectionCycle: "monthly",
	}); err != nil {
		t.Fatal(err)
	}

	empty, err := repo.SearchCustomersWithoutConfig("O03", "계", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("1글자 %d", len(empty))
	}
	allOrg, err := repo.SearchCustomersWithoutConfig(OrgAll, "계룡", 20)
	if err != nil || len(allOrg) != 0 {
		t.Fatalf("OrgAll %d %v", len(allOrg), err)
	}
	hits, err := repo.SearchCustomersWithoutConfig("O03", "계룡", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 20 {
		t.Fatalf("limit %d", len(hits))
	}
	for _, h := range hits {
		if h.ID == "gy-done" || h.ID == "gy-o01" || h.ID == "gy-off" || h.ID == "gy-pt" {
			t.Fatalf("제외되어야 함 %s", h.ID)
		}
	}
	reg, err := repo.SearchCustomersWithoutConfig("O03", "충남", 20)
	if err != nil || len(reg) == 0 {
		t.Fatalf("지역 %d %v", len(reg), err)
	}
	idHit, err := repo.SearchCustomersWithoutConfig("O03", "gy1", 20)
	if err != nil || len(idHit) == 0 || idHit[0].ID != "gy1" {
		t.Fatalf("id %+v %v", idHit, err)
	}
	o01, err := repo.SearchCustomersWithoutConfig("O01", "계룡", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(o01) != 1 || o01[0].ID != "gy-o01" {
		t.Fatalf("O01 %+v", o01)
	}
}

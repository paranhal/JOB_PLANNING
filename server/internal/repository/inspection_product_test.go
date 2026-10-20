package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func inspRepo(t *testing.T) *MaintenanceRepo {
	t.Helper()
	db, err := InitDB(filepath.Join(t.TempDir(), "insp.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O02','O02','미디어', 'business',1,20),('O03','O03','SI','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	return NewMaintenanceRepo(db)
}

func TestInspectionProductOrgIsolationAndImport(t *testing.T) {
	r := inspRepo(t)
	if n := r.CountInspectionProducts(model.OrgIDLibrary); n != 0 {
		t.Fatalf("시드 %d", n)
	}
	if _, err := r.db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c01','도서관','도서관',1,'O01'),('c03','학교','학교',1,'O03')`); err != nil {
		t.Fatal(err)
	}
	if err := r.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "c01", ShortName: "도서관", HasKlas: true, HasRfid: true, InspectionCycle: "monthly",
	}); err != nil {
		t.Fatal(err)
	}
	if err := r.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "c03", ShortName: "학교", InspectionCycle: "monthly",
	}); err != nil {
		t.Fatal(err)
	}
	cfgs, _ := r.ListSiteConfigs()
	var lib, si model.MaintenanceSiteConfig
	for _, c := range cfgs {
		if c.CustomerID == "c01" {
			lib = c
		}
		if c.CustomerID == "c03" {
			si = c
		}
	}
	if lib.Products != nil {
		t.Fatalf("O01 레거시 Products=%v", lib.Products)
	}
	if si.Products == nil {
		t.Fatal("O03 신 모드 Products nil")
	}
	o03Slots := model.SiteDueProductSlots(si, 8)
	if len(o03Slots) != 1 || o03Slots[0].SlotKey != "generic" {
		t.Fatalf("O03 slots %+v", o03Slots)
	}

	created, err := r.ImportLegacyLibraryProducts()
	if err != nil || created != 2 {
		t.Fatalf("import created=%d err=%v", created, err)
	}
	again, err := r.ImportLegacyLibraryProducts()
	if err != nil || again != 0 {
		t.Fatalf("재실행 created=%d err=%v", again, err)
	}
	libP, _ := r.ListInspectionProducts(model.OrgIDLibrary, true)
	if len(libP) != 2 {
		t.Fatalf("lib products %d", len(libP))
	}
	siP, _ := r.ListInspectionProducts("O03", true)
	if len(siP) != 0 {
		t.Fatal("O03에 도서관 제품")
	}
	net := &model.InspectionProduct{OrgID: "O03", Code: "NET", Name: "네트워크장비", IsActive: true}
	if err := r.CreateInspectionProduct(net); err != nil {
		t.Fatal(err)
	}
	if err := r.ReplaceSiteProducts("c03", "O03", []string{net.ProductID}); err != nil {
		t.Fatal(err)
	}
	if err := r.ReplaceSiteProducts("c03", "O03", []string{libP[0].ProductID}); err == nil || !IsOtherOrgInspectionProduct(err) {
		t.Fatalf("타 조직 연결 err=%v", err)
	}
	got, _ := r.GetSiteConfig("c03")
	slots := model.SiteDueProductSlots(*got, 8)
	if len(slots) != 1 || slots[0].ProductType != "네트워크장비" {
		t.Fatalf("slots %+v", slots)
	}
	o01after, _ := r.ListInspectionProducts(model.OrgIDLibrary, true)
	for _, p := range o01after {
		if p.Name == "네트워크장비" {
			t.Fatal("O01에 O03 제품")
		}
	}
}

func TestInspectionProductCodeLockedWhenInUse(t *testing.T) {
	r := inspRepo(t)
	p := &model.InspectionProduct{OrgID: "O03", Code: "NET", Name: "네트워크", IsActive: true}
	if err := r.CreateInspectionProduct(p); err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('cx','x','x',1,'O03')`); err != nil {
		t.Fatal(err)
	}
	if err := r.ReplaceSiteProducts("cx", "O03", []string{p.ProductID}); err != nil {
		t.Fatal(err)
	}
	p.Code = "NET2"
	if err := r.UpdateInspectionProduct(p); err == nil {
		t.Fatal("사용 중 코드 변경")
	}
}

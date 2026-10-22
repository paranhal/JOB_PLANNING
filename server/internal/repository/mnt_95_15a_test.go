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

func TestMnt95_15ABulkSetSkipsOtherOrgAndLeavesVisits(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "r15a.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id, addr_sido)
		VALUES ('r15a','가','가',1,?,'충청남도'), ('r15b','나','나',1,?,'')`, model.OrgIDLibrary, "O03"); err != nil {
		t.Fatal(err)
	}
	repo := NewMaintenanceRepo(db)
	_ = repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{CustomerID: "r15a", ShortName: "가", InspectionCycle: "monthly", Region: ""})
	_ = repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{CustomerID: "r15b", ShortName: "나", InspectionCycle: "monthly", Region: ""})
	_, _ = db.Exec(`UPDATE maintenance_site_config SET org_id=? WHERE customer_id='r15a'`, model.OrgIDLibrary)
	_, _ = db.Exec(`UPDATE maintenance_site_config SET org_id='O03' WHERE customer_id='r15b'`)
	p, err := repo.CreatePlan(2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-08-10", CustomerID: "r15a", ProductType: "KLAS", Notes: "keep-region",
	}); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, model.SiteBulkMax+1)
	for i := range ids {
		ids[i] = "x"
	}
	if _, err := repo.BulkSetSiteRegions(model.OrgIDLibrary, ids, "충남"); err == nil {
		t.Fatal("3000 초과")
	}
	res, err := repo.BulkSetSiteRegions(model.OrgIDLibrary, []string{"r15a", "r15b", "no-such", "r15a"}, "충남")
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed != 1 {
		t.Fatalf("변경 %d skip %+v", res.Changed, res.Skipped)
	}
	again, err := repo.BulkSetSiteRegions(model.OrgIDLibrary, []string{"r15a"}, "충남")
	if err != nil || again.Changed != 0 {
		t.Fatalf("멱등 %+v %v", again, err)
	}
	cfg, _ := repo.GetSiteConfig("r15b")
	if cfg != nil && cfg.Region == "충남" {
		t.Fatal("타 조직")
	}
	vs, _ := repo.ListVisits(p.PlanID)
	if len(vs) != 1 || vs[0].Notes != "keep-region" || vs[0].VisitDate != "2026-08-10" {
		t.Fatalf("방문 소급 %+v", vs)
	}
	fill, err := repo.FillSiteRegionsFromAddress(model.OrgIDLibrary, []string{"r15a"})
	if err != nil || fill.Changed != 0 {
		t.Fatalf("이미 있으면 채우지 않음 %+v %v", fill, err)
	}
}

func TestMnt95_15AFallbackOnlyEmptyRows(t *testing.T) {
	repo, _ := setupSiteBulk(t)
	res, err := repo.BulkRegisterSites(model.SiteBulkRegisterInput{
		OrgID: model.OrgIDLibrary, IndustryKey: "학교", Cycle: "semi",
		CustomerIDs: []string{"b-sch"}, FallbackRegion: "세종",
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := repo.GetSiteConfig("b-sch")
	if cfg == nil || cfg.Region != "충남" {
		t.Fatalf("주소 있는 행은 대체 금지 %+v", cfg)
	}
	if res.BatchID == "" {
		t.Fatal("batch")
	}
}

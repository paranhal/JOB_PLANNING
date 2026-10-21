package repository

import (
	"fmt"
	"testing"

	"customer-support/internal/model"
)

func TestSiteProgressExcludeDenomAndAllDone(t *testing.T) {
	r, pid := testEquipRepo(t)
	cr := NewCustomerRepo(r.db)
	parent := &model.Customer{OrgName: model.SchoolParentOfficialName, OfficialName: model.SchoolParentOfficialName, IsActive: true, PartyKind: "customer", OrgID: model.OrgIDLibrary}
	if err := cr.Create(parent); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, 80)
	for i := 0; i < 80; i++ {
		c := &model.Customer{
			OrgName: fmt.Sprintf("학교%03d", i), OfficialName: fmt.Sprintf("학교%03d", i),
			HasParent: true, ParentCustomerID: parent.CustomerID, AddrSigungu: "계룡시",
			IsActive: true, PartyKind: "customer", OrgID: model.OrgIDLibrary,
		}
		if err := cr.Create(c); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, c.CustomerID)
	}
	n, err := r.AddSitesFromCustomers(pid, model.OrgIDLibrary, "u", ids[:50])
	if err != nil || n != 50 {
		t.Fatalf("add n=%d err=%v", n, err)
	}
	again, err := r.AddSitesFromCustomers(pid, model.OrgIDLibrary, "u", ids[:5])
	if err != nil || again != 0 {
		t.Fatalf("dup n=%d err=%v", again, err)
	}
	sites, _, err := r.ListSites(pid, "", "", "", 1, 100)
	if err != nil || len(sites) != 50 {
		t.Fatalf("list %d err=%v", len(sites), err)
	}
	excl := []string{sites[0].ID, sites[1].ID, sites[2].ID}
	if err := r.BulkSiteStatus(pid, "u", model.SiteExcluded, "2026-10-21", "범위 밖", excl); err != nil {
		t.Fatal(err)
	}
	inst := []string{sites[3].ID, sites[4].ID}
	if err := r.BulkSiteStatus(pid, "u", model.SiteInstalled, "2026-10-21", "", inst); err != nil {
		t.Fatal(err)
	}
	p, err := r.SiteProgress(pid)
	if err != nil {
		t.Fatal(err)
	}
	if p.Denom != 47 || p.Installed != 2 || p.Inspected != 0 || p.Excluded != 3 || p.AllDone {
		t.Fatalf("%+v", p)
	}
	rest := make([]string, 0, 47)
	for _, s := range sites[3:] {
		rest = append(rest, s.ID)
	}
	if err := r.BulkSiteStatus(pid, "u", model.SiteInspected, "2026-10-21", "", rest); err != nil {
		t.Fatal(err)
	}
	p, err = r.SiteProgress(pid)
	if err != nil {
		t.Fatal(err)
	}
	if p.Denom != 47 || p.Inspected != 47 || !p.AllDone {
		t.Fatalf("all done %+v", p)
	}
	var st string
	if err := r.db.QueryRow(`SELECT status FROM work_projects WHERE project_id=?`, pid).Scan(&st); err != nil {
		t.Fatal(err)
	}
	if st == "complete" {
		t.Fatal("사업 자동 완료 금지")
	}
	var ev int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM project_site_events WHERE project_id=?`, pid).Scan(&ev)
	if ev < 50 {
		t.Fatalf("events=%d", ev)
	}
}

func TestSiteSchoolImportMatchAndMissing(t *testing.T) {
	r, pid := testEquipRepo(t)
	cr := NewCustomerRepo(r.db)
	p := &model.Customer{OrgName: model.SchoolParentOfficialName, OfficialName: model.SchoolParentOfficialName, IsActive: true, PartyKind: "customer", OrgID: model.OrgIDLibrary}
	if err := cr.Create(p); err != nil {
		t.Fatal(err)
	}
	c := &model.Customer{OrgName: "계룡고등학교", OfficialName: "계룡고등학교", HasParent: true, ParentCustomerID: p.CustomerID, OrgCode: "G001", IsActive: true, PartyKind: "customer", OrgID: model.OrgIDLibrary}
	if err := cr.Create(c); err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.Exec(`UPDATE customers SET org_code=? WHERE customer_id=?`, "G001", c.CustomerID); err != nil {
		t.Fatal(err)
	}
	xlsx, err := BuildSiteSchoolDataWorkbook([][]string{
		siteSchoolHdr,
		{model.SchoolParentOfficialName, "G001", "계룡고등학교", "", "", "", ""},
		{model.SchoolParentOfficialName, "", "없는학교", "", "", "", ""},
	})
	if err != nil {
		t.Fatal(err)
	}
	prev, err := r.PreviewSiteSchool(pid, model.OrgIDLibrary, xlsx, "add", false)
	if err != nil {
		t.Fatal(err)
	}
	if prev.SitesNew != 1 || prev.ErrN != 1 {
		t.Fatalf("preview new=%d err=%d %+v", prev.SitesNew, prev.ErrN, prev.Rows)
	}
	app, err := r.ApplySiteSchool(pid, model.OrgIDLibrary, "u", "a.xlsx", "add", xlsx, false)
	if err != nil {
		t.Fatal(err)
	}
	if app.SitesNew != 1 {
		t.Fatalf("apply %+v", app)
	}
	again, err := r.PreviewSiteSchool(pid, model.OrgIDLibrary, xlsx, "add", false)
	if err != nil {
		t.Fatal(err)
	}
	if again.Rows[0].Action != "skip" {
		t.Fatalf("skip 기대 %+v", again.Rows[0])
	}
}


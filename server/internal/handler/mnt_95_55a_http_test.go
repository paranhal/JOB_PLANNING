package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_55ListSiteConfigsHandlerCallersFilterOrg(t *testing.T) {
	allow := map[string]bool{
		"ListSiteConfigs":          true,
		"attachPeriodGroupRegions": true,
	}
	dir := filepath.Join(findTemplateRoot(t), "..", "..", "internal", "handler")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fnRe := regexp.MustCompile(`(?m)^func (?:\([^)]+\) )?([A-Za-z0-9_]+)\(`)
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".go") || strings.HasSuffix(ent.Name(), "_test.go") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, ent.Name()))
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		locs := fnRe.FindAllStringSubmatchIndex(src, -1)
		for i, loc := range locs {
			name := src[loc[2]:loc[3]]
			end := len(src)
			if i+1 < len(locs) {
				end = locs[i+1][0]
			}
			body := src[loc[0]:end]
			if !strings.Contains(body, "ListSiteConfigs()") {
				continue
			}
			if allow[name] {
				continue
			}
			if !strings.Contains(body, "FilterSiteConfigsByOrg") {
				t.Errorf("%s.%s 가 ListSiteConfigs 후 FilterSiteConfigsByOrg 를 쓰지 않습니다", ent.Name(), name)
			}
		}
	}
}

func newMnt955ACrossOrgApp(t *testing.T) (*echo.Echo, *repository.MaintenanceRepo, map[string]*model.MaintenancePlan, map[string]*model.User) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "55a.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O01','O01','도서관','business',1,10),
		('O02','O02','미디어','business',1,20),
		('O03','O03','SI사업팀','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	users.EnsureAdmin(HashPassword("admin"))
	lib := &model.User{Username: "lib55", PasswordHash: HashPassword("x"), FullName: "도서관장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	med := &model.User{Username: "med55", PasswordHash: HashPassword("x"), FullName: "미디어장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O02", ProfileDone: true}
	si := &model.User{Username: "si55", PasswordHash: HashPassword("x"), FullName: "에스아이장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O03", ProfileDone: true}
	for _, u := range []*model.User{lib, med, si} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id) VALUES
		('c-lib','해미도서관','해미도서관',1,?),
		('c-med','미디어센터','미디어센터',1,'O02'),
		('c-si','SI학교','SI학교',1,'O03')`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewMaintenanceRepo(db)
	for _, cfg := range []model.MaintenanceSiteConfig{
		{CustomerID: "c-lib", ShortName: "해미도서관", Region: "충남", HasKlas: true, InspectionCycle: "monthly", OrgID: model.OrgIDLibrary},
		{CustomerID: "c-med", ShortName: "미디어센터", Region: "충남", InspectionCycle: "monthly", OrgID: "O02"},
		{CustomerID: "c-si", ShortName: "SI학교", Region: "충남", InspectionCycle: "monthly", OrgID: "O03"},
	} {
		c := cfg
		if err := repo.UpsertSiteConfig(&c); err != nil {
			t.Fatal(err)
		}
	}
	plans := map[string]*model.MaintenancePlan{}
	for _, org := range []string{model.OrgIDLibrary, "O02", "O03"} {
		p, err := repo.CreatePlanForOrg(org, 2026, org+" 계획")
		if err != nil {
			t.Fatal(err)
		}
		plans[org] = p
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/maintenance/:id", h.Maintenance.ShowPlan)
	g.GET("/maintenance/:id/manage", h.Maintenance.ManagePlan)
	g.POST("/maintenance/:id/assign-slots", h.Maintenance.AssignUnassignedSlots, h.Auth.RequireMaintenanceEdit)
	g.POST("/maintenance/:id/assign-slots/bulk", h.Maintenance.AssignUnassignedSlotsBulk, h.Auth.RequireMaintenanceEdit)
	g.POST("/maintenance/:id/assign-slots/preview", h.Maintenance.PreviewAssignUnassignedSlotsBulk, h.Auth.RequireMaintenanceEdit)
	g.POST("/maintenance/:id/month-targets", h.Maintenance.RegisterMonthTargets, h.Auth.RequireMaintenanceEdit)
	g.GET("/maintenance/:id/month-targets", h.Maintenance.MonthTargetPanelJSON)
	g.POST("/workboard/mnt-quick-register", h.Workboard.QuickRegisterMaintenance)
	g.GET("/workboard/mnt-candidates", h.Workboard.MntCandidates)
	g.POST("/workboard/tasks", h.Workboard.CreateTask)
	g.GET("/workboard/new-work/form", h.Workboard.NewWorkForm)
	return e, repo, plans, map[string]*model.User{"O01": lib, "O02": med, "O03": si}
}

func Test95_55CrossOrgUnassignedAndAssignSlots(t *testing.T) {
	e, repo, plans, users := newMnt955ACrossOrgApp(t)
	siPlan := plans["O03"]
	libPlan := plans[model.OrgIDLibrary]
	siCK := jwtCookieUser(t, users["O03"])
	libCK := jwtCookieUser(t, users["O01"])
	medCK := jwtCookieUser(t, users["O02"])

	rec := manageGet(t, e, "/maintenance/"+siPlan.PlanID+"/manage", siCK)
	if rec.Code != http.StatusOK {
		t.Fatalf("SI manage status=%d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "SI학교") {
		t.Fatal("SI 사이트 없음")
	}
	if strings.Contains(body, "해미도서관") || strings.Contains(body, "미디어센터") {
		t.Fatal("SI 관리 화면에 타 조직 사이트가 보입니다")
	}

	libManage := manageGet(t, e, "/maintenance/"+libPlan.PlanID+"/manage", libCK)
	if libManage.Code != http.StatusOK {
		t.Fatalf("도서관 manage=%d", libManage.Code)
	}
	lb := libManage.Body.String()
	if !strings.Contains(lb, "해미도서관") {
		t.Fatal("도서관 사이트 없음")
	}
	if strings.Contains(lb, "SI학교") || strings.Contains(lb, "미디어센터") {
		t.Fatal("도서관 화면에 타 조직 사이트")
	}

	cross := manageGet(t, e, "/maintenance/"+siPlan.PlanID+"/manage", libCK)
	if cross.Code != http.StatusNotFound && cross.Code != http.StatusForbidden {
		t.Fatalf("도서관이 SI 계획 접근 status=%d", cross.Code)
	}
	cross2 := manageGet(t, e, "/maintenance/"+libPlan.PlanID+"/manage", siCK)
	if cross2.Code != http.StatusNotFound && cross2.Code != http.StatusForbidden {
		t.Fatalf("SI가 도서관 계획 접근 status=%d", cross2.Code)
	}
	medPlan := plans["O02"]
	cross3 := manageGet(t, e, "/maintenance/"+medPlan.PlanID+"/manage", siCK)
	if cross3.Code != http.StatusNotFound && cross3.Code != http.StatusForbidden {
		t.Fatalf("SI가 미디어 계획 접근 status=%d", cross3.Code)
	}
	_ = medCK

	slots, err := repo.ListOpenUnassignedSlotsForOrg(siPlan.PlanID, 2026, 10, "O03")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range slots {
		if s.CustomerID != "c-si" {
			t.Fatalf("SI 슬롯에 타 조직 %s", s.CustomerID)
		}
	}

	post := managePost(t, e, "/maintenance/"+siPlan.PlanID+"/assign-slots", url.Values{
		"customer_id":  {"c-lib", "c-si"},
		"product_type": {"KLAS", "점검"},
		"visit_date":   {"2026-10-15", "2026-10-16"},
		"assignee":     {"에스아이장"},
	}, siCK)
	if post.Code != http.StatusSeeOther {
		t.Fatalf("assign status=%d %s", post.Code, post.Body.String())
	}
	loc := post.Header().Get("Location")
	if !strings.Contains(loc, "다른 조직") && !strings.Contains(url.QueryEscape("다른 조직"), "다른") {
		if !strings.Contains(loc, "%") && !strings.Contains(loc, "건너") {
			t.Logf("location=%s", loc)
		}
	}
	visits, err := repo.ListVisits(siPlan.PlanID)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range visits {
		if v.CustomerID == "c-lib" {
			t.Fatal("도서관 사이트가 SI 계획에 배정됨")
		}
	}
	foundSI := false
	for _, v := range visits {
		if v.CustomerID == "c-si" && strings.HasPrefix(v.VisitDate, "2026-10-16") {
			foundSI = true
		}
	}
	if !foundSI {
		t.Fatalf("SI 배정 없음 visits=%v loc=%s", visits, loc)
	}

	foreign := managePost(t, e, "/maintenance/"+libPlan.PlanID+"/assign-slots", url.Values{
		"customer_id":  {"c-lib"},
		"product_type": {"KLAS"},
		"visit_date":   {"2026-10-20"},
		"assignee":     {"에스아이장"},
	}, siCK)
	if foreign.Code != http.StatusNotFound && foreign.Code != http.StatusForbidden {
		t.Fatalf("SI가 도서관 계획 assign-slots status=%d", foreign.Code)
	}
}

func Test95_55IntegrationSIThenOtherOrgs(t *testing.T) {
	e, repo, plans, users := newMnt955ACrossOrgApp(t)
	for _, org := range []string{"O03", "O02", model.OrgIDLibrary} {
		p := plans[org]
		u := users[org]
		if org == model.OrgIDLibrary {
			u = users["O01"]
		}
		ck := jwtCookieUser(t, u)
		cid := map[string]string{"O03": "c-si", "O02": "c-med", model.OrgIDLibrary: "c-lib"}[org]
		if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
			CustomerID: cid, ShortName: cid, InspectionCycle: "semi", OrgID: org, Region: "충남",
		}); err != nil {
			t.Fatal(err)
		}
		reg := managePost(t, e, "/maintenance/"+p.PlanID+"/month-targets", url.Values{
			"month": {"10"}, "source": {"site"}, "customer_id": {cid},
		}, ck)
		if reg.Code != http.StatusSeeOther && reg.Code != http.StatusOK {
			t.Fatalf("%s 월대상 status=%d %s", org, reg.Code, reg.Body.String())
		}
		mg := manageGet(t, e, "/maintenance/"+p.PlanID+"/manage?um=10", ck)
		if mg.Code != http.StatusOK {
			t.Fatalf("%s manage=%d", org, mg.Code)
		}
		foreign := map[string][]string{
			"O03": {"해미도서관", "미디어센터"},
			"O02": {"해미도서관", "SI학교"},
			model.OrgIDLibrary: {"SI학교", "미디어센터"},
		}[org]
		for _, name := range foreign {
			if strings.Contains(mg.Body.String(), name) {
				t.Fatalf("%s 화면에 타 조직 %s", org, name)
			}
		}
		slots, err := repo.ListOpenUnassignedSlotsForOrg(p.PlanID, 2026, 10, org)
		if err != nil {
			t.Fatal(err)
		}
		form := url.Values{"month": {"10"}, "mode": {"same_date"}, "visit_date": {"2026-10-20"}, "assignee": {u.FullName}}
		for _, s := range slots {
			if s.CustomerID == cid {
				form.Add("slot_key", slotKeyOf(s))
			}
		}
		if len(form["slot_key"]) == 0 {
			t.Fatalf("%s 슬롯 없음", org)
		}
		bulk := managePost(t, e, "/maintenance/"+p.PlanID+"/assign-slots/bulk", form, ck)
		if bulk.Code != http.StatusOK && bulk.Code != http.StatusSeeOther {
			t.Fatalf("%s bulk=%d %s", org, bulk.Code, bulk.Body.String())
		}
		show := manageGet(t, e, "/maintenance/"+p.PlanID, ck)
		if show.Code != http.StatusOK {
			t.Fatalf("%s show=%d", org, show.Code)
		}
		cand := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://localhost/workboard/mnt-candidates?q=", nil)
		req.AddCookie(ck)
		e.ServeHTTP(cand, req)
		if cand.Code != http.StatusOK {
			t.Fatalf("%s candidates=%d %s", org, cand.Code, cand.Body.String())
		}
	}
}

func Test95_55PlanShowOpenAddPresent(t *testing.T) {
	e, _, plans, users := newMnt955ACrossOrgApp(t)
	p := plans["O03"]
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/maintenance/"+p.PlanID, nil)
	req.AddCookie(jwtCookieUser(t, users["O03"]))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("show=%d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `openAdd()`) {
		t.Fatal("openAdd 없음")
	}
}

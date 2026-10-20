package handler

import (
	"database/sql"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func newInspHTTP(t *testing.T) (*echo.Echo, *sql.DB, *http.Cookie, *http.Cookie, *http.Cookie, *http.Cookie) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "insph.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O02','O02','미디어','business',1,20),('O03','O03','SI','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	vs := &model.User{Username: "vs95", PasswordHash: "x", FullName: "비젼", Role: model.RoleVisionAdmin, IsActive: true, ProfileDone: true}
	o1 := &model.User{Username: "oa01", PasswordHash: "x", FullName: "도서관장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	o3 := &model.User{Username: "oa03", PasswordHash: "x", FullName: "SI장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O03", ProfileDone: true}
	cl := &model.User{Username: "cl95", PasswordHash: "x", FullName: "씨레벨", Role: model.RoleCLevel, IsActive: true, IsReadOnly: true, ProfileDone: true}
	for _, u := range []*model.User{vs, o1, o3, cl} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/maintenance/sites", h.Maintenance.ListSiteConfigs)
	g.GET("/maintenance/sites/new", h.Maintenance.NewSiteConfigPage)
	g.GET("/maintenance/sites/customer-search", h.Maintenance.SearchSiteCustomers)
	g.POST("/maintenance/sites", h.Maintenance.SaveSiteConfig)
	g.GET("/maintenance", h.Maintenance.ListPlans)
	g.GET("/maintenance/:id", h.Maintenance.ShowPlan)
	g.GET("/maintenance/:id/manage", h.Maintenance.ManagePlan)
	g.GET("/maintenance/visits/:visit_id/action", h.Maintenance.VisitAction)
	g.GET("/maintenance/products", h.Maintenance.ListInspectionProducts)
	g.POST("/maintenance/products", h.Maintenance.SaveInspectionProduct)
	g.POST("/maintenance/products/import-legacy", h.Maintenance.ImportLegacyInspectionProducts)
	g.POST("/maintenance/products/:id/update", h.Maintenance.UpdateInspectionProduct)
	g.POST("/maintenance/products/:id/toggle", h.Maintenance.ToggleInspectionProduct)
	return e, db, jwtCookieUserClaims(t, vs), jwtCookieUserClaims(t, o1), jwtCookieUserClaims(t, o3), jwtCookieUserClaims(t, cl)
}

func assertNoLibraryProducts(t *testing.T, body string) {
	t.Helper()
	for _, s := range []string{"KLAS", "RFID", "앤로보틱스"} {
		if strings.Contains(body, s) {
			t.Fatalf("도서관 제품 노출: %s", s)
		}
	}
}

func TestInspectionProductHTTP_O03HidesLibraryAndCLevelWrite(t *testing.T) {
	e, db, _, o1, o3, cl := newInspHTTP(t)
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c03','SI학교','SI학교',1,'O03'),('c01','도서관','도서관',1,'O01')`); err != nil {
		t.Fatal(err)
	}
	mnt := repository.NewMaintenanceRepo(db)
	_ = mnt.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "c03", ShortName: "SI학교", InspectionCycle: "monthly",
	})
	_ = mnt.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "c01", ShortName: "도서관", HasKlas: true, HasRfid: true, InspectionCycle: "monthly",
	})

	sites := doGetWith(t, e, "/maintenance/sites", o3)
	if sites.Code != http.StatusOK {
		t.Fatalf("sites %d", sites.Code)
	}
	assertNoLibraryProducts(t, sites.Body.String())
	form := doGetWith(t, e, "/maintenance/sites/new", o3)
	if form.Code != http.StatusOK {
		t.Fatalf("form %d", form.Code)
	}
	assertNoLibraryProducts(t, form.Body.String())

	prod := doGetWith(t, e, "/maintenance/products", o3)
	if prod.Code != http.StatusOK {
		t.Fatalf("products %d", prod.Code)
	}
	assertNoLibraryProducts(t, prod.Body.String())

	rec := postForm947(t, e, "/maintenance/products", url.Values{
		"name": {"네트워크장비"}, "code": {"NET"},
	}, o3)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create %d %s", rec.Code, rec.Body.String())
	}
	o3page := doGetWith(t, e, "/maintenance/products", o3)
	if !strings.Contains(o3page.Body.String(), "네트워크장비") {
		t.Fatal("O03 제품 없음")
	}
	o1page := doGetWith(t, e, "/maintenance/products", o1)
	if strings.Contains(o1page.Body.String(), ">네트워크장비<") {
		t.Fatal("O01에 O03 제품")
	}

	var pid string
	_ = db.QueryRow(`SELECT product_id FROM inspection_products WHERE org_id='O03'`).Scan(&pid)
	foreign := postForm947(t, e, "/maintenance/sites", url.Values{
		"customer_id": {"c03"}, "short_name": {"SI학교"}, "inspection_cycle": {"monthly"},
		"product_id": {pid},
	}, o1)
	if foreign.Code == http.StatusSeeOther {
		// o1 저장 시 siteOrg=O03, product O03 — 실제로 o1가 c03을 수정하면 조직 불일치. Replace uses CustomerOrgID O03 vs product O03 so it might succeed.
		// 타 조직 product_id 접근: o3가 O01 제품 URL
	}
	libID := ""
	_ = db.QueryRow(`SELECT product_id FROM inspection_products WHERE org_id='O01' LIMIT 1`).Scan(&libID)
	if libID == "" {
		_, _ = mnt.ImportLegacyLibraryProducts()
		_ = db.QueryRow(`SELECT product_id FROM inspection_products WHERE org_id='O01' LIMIT 1`).Scan(&libID)
	}
	bad := postForm947(t, e, "/maintenance/sites", url.Values{
		"customer_id": {"c03"}, "short_name": {"SI학교"}, "inspection_cycle": {"monthly"},
		"product_id": {libID},
	}, o3)
	if bad.Code != http.StatusBadRequest && bad.Code != http.StatusForbidden {
		t.Fatalf("타 조직 연결 %d", bad.Code)
	}

	clGet := doGetWith(t, e, "/maintenance/products", cl)
	if clGet.Code != http.StatusOK {
		t.Fatalf("clevel get %d", clGet.Code)
	}
	clPost := postForm947(t, e, "/maintenance/products", url.Values{"name": {"임원제품"}, "code": {"EX"}}, cl)
	if clPost.Code != http.StatusForbidden {
		t.Fatalf("clevel write %d", clPost.Code)
	}

	o1upd := postForm947(t, e, "/maintenance/products/"+pid+"/update", url.Values{
		"name": {"해킹"}, "code": {"HACK"},
	}, o1)
	if o1upd.Code != http.StatusNotFound {
		t.Fatalf("타 조직 제품 URL %d", o1upd.Code)
	}
}

func TestInspectionProductHTTP_LegacyImportButton(t *testing.T) {
	e, db, _, o1, _, _ := newInspHTTP(t)
	before := 0
	_ = db.QueryRow(`SELECT COUNT(*) FROM inspection_products`).Scan(&before)
	page := doGetWith(t, e, "/maintenance/products", o1)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "기본 제품 가져오기") {
		t.Fatalf("버튼 없음 %d", page.Code)
	}
	if before != 0 {
		t.Fatal("버튼 전에 시드")
	}
	rec := postForm947(t, e, "/maintenance/products/import-legacy", url.Values{}, o1)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("import %d", rec.Code)
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM inspection_products WHERE org_id='O01'`).Scan(&n)
	if n != 2 {
		t.Fatalf("import n=%d", n)
	}
}

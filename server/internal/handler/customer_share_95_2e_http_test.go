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

type share2ECookies struct {
	vs, cl, oa01, oa03, sales, obs, partner *http.Cookie
}

func newCustomerShare2EHTTP(t *testing.T) (*echo.Echo, *sql.DB, *repository.CustomerRepo, share2ECookies) {
	t.Helper()
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "c95-2e.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O02','O02','미디어사업팀','business',1,20),('O03','O03','SI사업팀','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	users.EnsureAdmin(HashPassword("admin"))
	vs := &model.User{Username: "vs-2e", PasswordHash: "x", FullName: "비젼", Role: model.RoleVisionAdmin, IsActive: true, ProfileDone: true}
	oa01 := &model.User{Username: "oa01-2e", PasswordHash: "x", FullName: "도서관장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	oa03 := &model.User{Username: "oa03-2e", PasswordHash: "x", FullName: "SI장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O03", ProfileDone: true}
	cl := &model.User{Username: "cl-2e", PasswordHash: "x", FullName: "씨레벨", Role: model.RoleCLevel, IsActive: true, IsReadOnly: true, ProfileDone: true}
	sales := &model.User{Username: "sa-2e", PasswordHash: "x", FullName: "영업", Role: model.RoleSales, IsActive: true, OrgID: "O03", ProfileDone: true}
	obs := &model.User{Username: "ob-2e", PasswordHash: "x", FullName: "열람", Role: model.RoleObserver, IsActive: true, IsReadOnly: true, OrgID: "O02", ProfileDone: true}
	pt := &model.User{Username: "pt-2e", PasswordHash: "x", FullName: "협력", Role: "partner", IsActive: true, OrgID: "O03", ProfileDone: true}
	for _, u := range []*model.User{vs, oa01, oa03, cl, sales, obs, pt} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	AttachHTTPErrorHandler(e)
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/customers", h.Customer.List)
	g.GET("/customers/export.xlsx", h.Customer.ExportExcel)
	g.GET("/customers/new", h.Customer.New)
	g.POST("/customers", h.Customer.Create, h.Auth.RequireMasterWrite)
	g.GET("/customers/:id", h.Customer.Show)
	g.POST("/customers/:id/update", h.Customer.Update, h.Auth.RequireMasterWrite)
	g.POST("/customers/:id/delete", h.Customer.Delete)
	g.GET("/projects/registry", h.Project.Registry)
	repo := repository.NewCustomerRepo(db)
	ck := share2ECookies{
		vs: jwtCookieUserClaims(t, vs), cl: jwtCookieUserClaims(t, cl),
		oa01: jwtCookieUserClaims(t, oa01), oa03: jwtCookieUserClaims(t, oa03),
		sales: jwtCookieUserClaims(t, sales), obs: jwtCookieUserClaims(t, obs),
		partner: jwtCookieUserClaims(t, pt),
	}
	return e, db, repo, ck
}

func seedCust2E(t *testing.T, db *sql.DB, id, name, org string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO customers(customer_id,org_name,official_name,is_active,org_id,party_kind)
		VALUES(?,?,?,1,?,?)`, id, name, name, org, model.PartyKindCustomer); err != nil {
		t.Fatal(err)
	}
}

func TestCustomerShare2EAllStaffSeeSix(t *testing.T) {
	e, db, _, ck := newCustomerShare2EHTTP(t)
	seedCust2E(t, db, "C2E-O01A", "2E-O01-A", "O01")
	seedCust2E(t, db, "C2E-O01B", "2E-O01-B", "O01")
	seedCust2E(t, db, "C2E-O02A", "2E-O02-A", "O02")
	seedCust2E(t, db, "C2E-O02B", "2E-O02-B", "O02")
	seedCust2E(t, db, "C2E-O03A", "2E-O03-A", "O03")
	seedCust2E(t, db, "C2E-O03B", "2E-O03-B", "O03")
	for _, pair := range []struct {
		name string
		ck   *http.Cookie
	}{
		{"oa03", ck.oa03}, {"oa01", ck.oa01}, {"vs", ck.vs}, {"cl", ck.cl}, {"sales", ck.sales}, {"obs", ck.obs},
	} {
		rec := getHTMLWith(t, e, "/customers?search=2E-O", pair.ck)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s %d", pair.name, rec.Code)
		}
		b := rec.Body.String()
		for _, name := range []string{"2E-O01-A", "2E-O01-B", "2E-O02-A", "2E-O02-B", "2E-O03-A", "2E-O03-B"} {
			if !strings.Contains(b, name) {
				t.Fatalf("%s 목록에 %s 없음", pair.name, name)
			}
		}
		if strings.Contains(b, `id="customer-org-filter"`) || strings.Contains(b, ">소속 조직<") {
			t.Fatalf("%s 고객 목록에 조직 필터/컬럼", pair.name)
		}
	}
}

func TestCustomerShare2ESalesCreateAndDupWarn(t *testing.T) {
	e, db, repo, ck := newCustomerShare2EHTTP(t)
	created := postFormWith(t, e, "/customers", url.Values{
		"org_name": {"2E신규학교"}, "official_name": {"2E신규학교"},
	}, ck.sales, "text/html", "/customers/new")
	if created.Code != http.StatusSeeOther {
		t.Fatalf("등록 %d %s", created.Code, created.Body.String())
	}
	oa01 := getHTMLWith(t, e, "/customers?search="+url.QueryEscape("2E신규학교"), ck.oa01)
	if !strings.Contains(oa01.Body.String(), "2E신규학교") {
		t.Fatal("다른 조직에서 신규 고객 안 보임")
	}
	dup := postFormWith(t, e, "/customers", url.Values{
		"org_name": {"2E신규학교"}, "official_name": {"2E신규학교"},
	}, ck.oa03, "text/html", "/customers/new")
	if dup.Code != http.StatusOK {
		t.Fatalf("동명 경고 status=%d", dup.Code)
	}
	if !strings.Contains(dup.Body.String(), "같은 이름") {
		t.Fatal("동명 경고 문구 없음")
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM customers WHERE org_name='2E신규학교'`).Scan(&n)
	if n != 1 {
		t.Fatalf("경고만인데 저장됨 n=%d", n)
	}
	ok := postFormWith(t, e, "/customers", url.Values{
		"org_name": {"2E신규학교"}, "official_name": {"2E신규학교"}, "confirm_dup": {"1"},
	}, ck.oa03, "text/html", "/customers/new")
	if ok.Code != http.StatusSeeOther {
		t.Fatalf("확인 후 등록 %d", ok.Code)
	}
	items, _, err := repo.List(repository.OrgAll, "2E신규학교", "", "", "", "", 1, 20, false, "", false)
	if err != nil || len(items) < 2 {
		t.Fatalf("확인 후 건수 %d %v", len(items), err)
	}
	cl := postFormWith(t, e, "/customers", url.Values{
		"org_name": {"임원등록"}, "official_name": {"임원등록"},
	}, ck.cl, "text/html", "/customers/new")
	if cl.Code != http.StatusForbidden && cl.Code != http.StatusSeeOther {
		t.Fatalf("clevel %d", cl.Code)
	}
	if cl.Code == http.StatusSeeOther && strings.Contains(cl.Header().Get("Location"), "/customers") {
		var cn int
		_ = db.QueryRow(`SELECT COUNT(*) FROM customers WHERE org_name='임원등록'`).Scan(&cn)
		if cn != 0 {
			t.Fatal("c_level 등록이 저장됨")
		}
	}
	obs := postFormWith(t, e, "/customers", url.Values{
		"org_name": {"열람등록"}, "official_name": {"열람등록"},
	}, ck.obs, "text/html", "/customers/new")
	if obs.Code != http.StatusForbidden && obs.Code != http.StatusSeeOther {
		t.Fatalf("observer %d", obs.Code)
	}
}

func TestCustomerShare2EDeleteAdminOnly(t *testing.T) {
	e, db, _, ck := newCustomerShare2EHTTP(t)
	seedCust2E(t, db, "C2E-DEL", "2E삭제대상", "O01")
	deny := postFormWith(t, e, "/customers/C2E-DEL/delete", url.Values{}, ck.sales, "text/html", "/customers")
	if deny.Code != http.StatusSeeOther && deny.Code != http.StatusForbidden {
		t.Fatalf("영업 삭제 %d", deny.Code)
	}
	var active int
	_ = db.QueryRow(`SELECT is_active FROM customers WHERE customer_id='C2E-DEL'`).Scan(&active)
	if active != 1 {
		t.Fatal("일반 쓰기가 비활성화함")
	}
	ok := postFormWith(t, e, "/customers/C2E-DEL/delete", url.Values{}, ck.oa03, "text/html", "/customers")
	if ok.Code != http.StatusSeeOther {
		t.Fatalf("조직관리자 삭제 %d", ok.Code)
	}
	_ = db.QueryRow(`SELECT is_active FROM customers WHERE customer_id='C2E-DEL'`).Scan(&active)
	if active != 0 {
		t.Fatal("관리자 비활성 실패")
	}
}

func TestPartnerDeniedCustomerAndProject(t *testing.T) {
	e, _, _, ck := newCustomerShare2EHTTP(t)
	cust := getHTMLWith(t, e, "/customers", ck.partner)
	if cust.Code != http.StatusForbidden {
		t.Fatalf("partner 고객 %d", cust.Code)
	}
	proj := getHTMLWith(t, e, "/projects/registry", ck.partner)
	if proj.Code != http.StatusForbidden {
		t.Fatalf("partner 사업 %d", proj.Code)
	}
}

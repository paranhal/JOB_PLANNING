package handler

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func newCustomer947Server(t *testing.T) (*echo.Echo, *sql.DB, *model.User, *model.User, *model.User) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "cust947.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O02','O02','미디어사업팀','business',1,20)`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	vs := &model.User{Username: "vs1", PasswordHash: "x", FullName: "비젼관리자", Role: model.RoleVisionAdmin, IsActive: true, ProfileDone: true}
	oa := &model.User{Username: "oa01", PasswordHash: "x", FullName: "도서관관리자", Role: model.RoleOrgAdmin, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	cl := &model.User{Username: "cl1", PasswordHash: "x", FullName: "씨레벨", Role: model.RoleCLevel, IsActive: true, IsReadOnly: true, ProfileDone: true}
	for _, u := range []*model.User{vs, oa, cl} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/customers", h.Customer.List)
	g.GET("/customers/new", h.Customer.New)
	g.POST("/customers", h.Customer.Create, h.Auth.RequireMasterWrite)
	g.GET("/maintenance/sites", h.Maintenance.ListSiteConfigs)
	g.GET("/maintenance/sites/new", h.Maintenance.NewSiteConfigPage)
	g.POST("/maintenance/sites", h.Maintenance.SaveSiteConfig, h.Auth.RequireMasterWrite)
	g.POST("/maintenance", h.Maintenance.CreatePlan)
	return e, db, vs, oa, cl
}

func postForm947(t *testing.T, e *echo.Echo, path string, form url.Values, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost"+path, strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	if ck != nil {
		req.AddCookie(ck)
	}
	e.ServeHTTP(rec, req)
	return rec
}

func TestCustomer947VisionAllOrgRequiresPick(t *testing.T) {
	e, db, vs, _, _ := newCustomer947Server(t)
	ck := jwtCookieViewOrg(t, vs, repository.OrgAll)
	list := doGetWith(t, e, "/customers", ck)
	if list.Code != http.StatusOK {
		t.Fatalf("list %d", list.Code)
	}
	form := doGetWith(t, e, "/customers/new", ck)
	if form.Code != http.StatusOK {
		t.Fatalf("new %d", form.Code)
	}
	if !strings.Contains(form.Body.String(), `id="customer-org-id"`) {
		t.Fatal("전체 보기 조직 드롭다운 없음")
	}
	miss := postForm947(t, e, "/customers", url.Values{
		"org_name": {"전체보기기관"}, "official_name": {"전체보기기관"},
	}, ck)
	if miss.Code == http.StatusInternalServerError {
		t.Fatal("미선택 500")
	}
	if miss.Code != http.StatusOK || !strings.Contains(miss.Body.String(), errNeedInsertOrg) {
		t.Fatalf("미선택 status=%d body=%s", miss.Code, miss.Body.String()[:minInt(400, miss.Body.Len())])
	}
	ok := postForm947(t, e, "/customers", url.Values{
		"org_name": {"전체보기기관"}, "official_name": {"전체보기기관"}, "org_id": {"O02"},
	}, ck)
	if ok.Code != http.StatusSeeOther {
		t.Fatalf("선택 저장 %d %s", ok.Code, ok.Body.String()[:minInt(300, ok.Body.Len())])
	}
	var org string
	if err := db.QueryRow(`SELECT org_id FROM customers WHERE org_name='전체보기기관'`).Scan(&org); err != nil || org != "O02" {
		t.Fatalf("org=%q err=%v", org, err)
	}
}

func TestCustomer947VisionViewO02NoPick(t *testing.T) {
	e, db, vs, _, _ := newCustomer947Server(t)
	ck := jwtCookieViewOrg(t, vs, "O02")
	page := doGetWith(t, e, "/customers/new", ck)
	if strings.Contains(page.Body.String(), `id="customer-org-id"`) {
		t.Fatal("특정 보기인데 드롭다운")
	}
	rec := postForm947(t, e, "/customers", url.Values{
		"org_name": {"O02기관"}, "official_name": {"O02기관"},
	}, ck)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d", rec.Code)
	}
	var org string
	if err := db.QueryRow(`SELECT org_id FROM customers WHERE org_name='O02기관'`).Scan(&org); err != nil || org != "O02" {
		t.Fatalf("org=%q err=%v", org, err)
	}
}

func TestCustomer947OrgAdminLockedAndRejectsOther(t *testing.T) {
	e, db, _, oa, _ := newCustomer947Server(t)
	ck := jwtCookieUserClaims(t, oa)
	page := doGetWith(t, e, "/customers/new", ck)
	if strings.Contains(page.Body.String(), `id="customer-org-id"`) {
		t.Fatal("조직관리자 드롭다운")
	}
	ok := postForm947(t, e, "/customers", url.Values{
		"org_name": {"본인조직기관"}, "official_name": {"본인조직기관"},
	}, ck)
	if ok.Code != http.StatusSeeOther {
		t.Fatalf("본인 저장 %d", ok.Code)
	}
	var org string
	if err := db.QueryRow(`SELECT org_id FROM customers WHERE org_name='본인조직기관'`).Scan(&org); err != nil || org != model.OrgIDLibrary {
		t.Fatalf("org=%q err=%v", org, err)
	}
	deny := postForm947(t, e, "/customers", url.Values{
		"org_name": {"조작기관"}, "official_name": {"조작기관"}, "org_id": {"O02"},
	}, ck)
	if deny.Code == http.StatusInternalServerError || deny.Code == http.StatusSeeOther {
		t.Fatalf("타 조직 조작 status=%d", deny.Code)
	}
	if !strings.Contains(deny.Body.String(), "자기 조직만") {
		t.Fatalf("거부 메시지 없음 %s", deny.Body.String()[:minInt(300, deny.Body.Len())])
	}
}

func TestCustomer947CLevelForbidden(t *testing.T) {
	e, _, _, _, cl := newCustomer947Server(t)
	ck := jwtCookieUserClaims(t, cl)
	rec := postForm947(t, e, "/customers", url.Values{
		"org_name": {"임원기관"}, "official_name": {"임원기관"}, "org_id": {model.OrgIDLibrary},
	}, ck)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("clevel %d", rec.Code)
	}
}

func TestCustomer947SiteConfigNoOrgOnInsert(t *testing.T) {
	e, db, vs, _, cl := newCustomer947Server(t)
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c947','사이트기관','사이트기관',1,'O02')`); err != nil {
		t.Fatal(err)
	}
	ck := jwtCookieViewOrg(t, vs, repository.OrgAll)
	list := doGetWith(t, e, "/maintenance/sites", ck)
	if list.Code != http.StatusOK {
		t.Fatalf("sites list %d", list.Code)
	}
	page := doGetWith(t, e, "/maintenance/sites/new", ck)
	if page.Code != http.StatusOK {
		t.Fatalf("sites new %d", page.Code)
	}
	miss := postForm947(t, e, "/maintenance/sites", url.Values{"short_name": {"표시"}}, ck)
	if miss.Code == http.StatusInternalServerError {
		t.Fatal("사이트 미입력 500")
	}
	if miss.Code != http.StatusOK || !strings.Contains(miss.Body.String(), "고객과 표시명은 필수") {
		t.Fatalf("사이트 미입력 %d", miss.Code)
	}
	ok := postForm947(t, e, "/maintenance/sites", url.Values{
		"customer_id": {"c947"}, "short_name": {"사이트표시"}, "inspection_cycle": {"monthly"},
	}, ck)
	if ok.Code != http.StatusSeeOther {
		t.Fatalf("사이트 저장 %d %s", ok.Code, ok.Body.String()[:minInt(200, ok.Body.Len())])
	}
	clCK := jwtCookieUserClaims(t, cl)
	forbid := postForm947(t, e, "/maintenance/sites", url.Values{
		"customer_id": {"c947"}, "short_name": {"임원"},
	}, clCK)
	if forbid.Code != http.StatusForbidden {
		t.Fatalf("clevel site %d", forbid.Code)
	}
}

func TestCreatePlan947VisionAllOrgNeedsPick(t *testing.T) {
	e, _, vs, _, _ := newCustomer947Server(t)
	ck := jwtCookieViewOrg(t, vs, repository.OrgAll)
	miss := postForm947(t, e, "/maintenance", url.Values{"plan_year": {"2027"}, "title": {"27년"}}, ck)
	if miss.Code == http.StatusInternalServerError {
		t.Fatal("계획 500")
	}
	if miss.Code != http.StatusBadRequest {
		t.Fatalf("계획 미선택 %d", miss.Code)
	}
	ok := postForm947(t, e, "/maintenance", url.Values{
		"plan_year": {"2027"}, "title": {"27년"}, "org_id": {"O02"},
	}, ck)
	if ok.Code != http.StatusSeeOther {
		t.Fatalf("계획 저장 %d %s", ok.Code, ok.Body.String()[:minInt(200, ok.Body.Len())])
	}
}

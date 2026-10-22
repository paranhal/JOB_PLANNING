package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func newSiteBulkHTTP(t *testing.T) (*echo.Echo, *repository.MaintenanceRepo, *http.Cookie, *http.Cookie, *http.Cookie, *model.User) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "h15.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	vs := &model.User{Username: "vs15", PasswordHash: HashPassword("x"), FullName: "비젼", Role: model.RoleVisionAdmin, IsActive: true, ProfileDone: true}
	oa := &model.User{Username: "oa15", PasswordHash: HashPassword("x"), FullName: "도서관장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	cl := &model.User{Username: "cl15", PasswordHash: HashPassword("x"), FullName: "씨레벨", Role: model.RoleCLevel, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	tech := &model.User{Username: "t15", PasswordHash: HashPassword("x"), FullName: "기술", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	for _, u := range []*model.User{vs, oa, cl, tech} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, party_kind, industry, org_id, addr_sido)
		VALUES ('h15a','일광고','일광고',1,'customer','학교',?,'충남'),
		       ('h15b','이광고','이광고',1,'customer','학교',?,'충남')`, model.OrgIDLibrary, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	AttachHTTPErrorHandler(e)
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.Use(h.Auth.InjectViewAs)
	adminOnly := h.Auth.RequireAdminMW
	ms := g.Group("/maintenance/sites", adminOnly)
	ms.GET("/bulk", h.Maintenance.SiteBulkPage)
	ms.POST("/bulk", h.Maintenance.SiteBulkRegister)
	ms.GET("/bulk/skips.xlsx", h.Maintenance.SiteBulkSkipExcel)
	return e, repository.NewMaintenanceRepo(db), jwtCookieUserClaims(t, vs), jwtCookieUserClaims(t, oa), jwtCookieUserClaims(t, cl), tech
}

func TestMnt95_15HTTPVisionNeedsOrgThenRegister(t *testing.T) {
	e, repo, vs, oa, _, _ := newSiteBulkHTTP(t)
	page := doGetWith(t, e, "/maintenance/sites/bulk", vs)
	if page.Code != http.StatusOK {
		t.Fatalf("vsadmin %d", page.Code)
	}
	body := page.Body.String()
	if !strings.Contains(body, "소속 조직을 선택하세요") && !strings.Contains(body, "조직을 먼저") {
		snip := body
		if len(snip) > 400 {
			snip = snip[:400]
		}
		t.Fatalf("조직 선택 안내 없음 %s", snip)
	}
	rec := doFormUser(t, e, "/maintenance/sites/bulk", url.Values{
		"industry":          {"학교"},
		"inspection_cycle":  {"quarterly"},
		"rounds_per_period": {"1"},
		"customer_id":       {"h15a"},
		"short_h15a":        {"일광표시"},
	}, oa)
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusOK {
		t.Fatalf("등록 %d %s", rec.Code, rec.Body.String())
	}
	cfg, _ := repo.GetSiteConfig("h15a")
	if cfg == nil {
		t.Fatal("등록 안 됨")
	}
	if other, _ := repo.GetSiteConfig("h15b"); other != nil {
		t.Fatal("선택 안 한 건")
	}
}

func TestMnt95_15HTTPForbiddenAndOrgAdminOtherOrg(t *testing.T) {
	e, _, _, oa, cl, tech := newSiteBulkHTTP(t)
	rec := doFormUser(t, e, "/maintenance/sites/bulk", url.Values{
		"industry": {"학교"}, "inspection_cycle": {"monthly"}, "customer_id": {"h15a"}, "org_id": {"O03"},
	}, oa)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("타 조직은 403 got %d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
	if rec := doFormUser(t, e, "/maintenance/sites/bulk", url.Values{
		"industry": {"학교"}, "inspection_cycle": {"monthly"}, "customer_id": {"h15a"},
	}, cl); rec.Code != http.StatusForbidden {
		t.Fatalf("C-Level %d", rec.Code)
	}
	req := httptest.NewRequest(http.MethodPost, "http://localhost/maintenance/sites/bulk", strings.NewReader(url.Values{
		"industry": {"학교"}, "inspection_cycle": {"monthly"}, "customer_id": {"h15a"},
	}.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(oa)
	req.AddCookie(&http.Cookie{Name: "view_as_user_id", Value: tech.UserID, Path: "/"})
	view := httptest.NewRecorder()
	e.ServeHTTP(view, req)
	if view.Code != http.StatusForbidden && view.Code != http.StatusSeeOther {
		t.Fatalf("view-as %d", view.Code)
	}
	if view.Code != http.StatusForbidden {
		t.Fatalf("view-as 쓰기는 403 got %d", view.Code)
	}
}

func TestMnt95_15HTTPEmptySelectionAndSkipExcel(t *testing.T) {
	e, _, _, oa, _, _ := newSiteBulkHTTP(t)
	empty := doFormUser(t, e, "/maintenance/sites/bulk", url.Values{
		"industry": {"학교"}, "inspection_cycle": {"monthly"},
	}, oa)
	if empty.Code != http.StatusBadRequest && empty.Code != http.StatusOK && empty.Code != http.StatusSeeOther {
		t.Fatalf("전체 해제 %d", empty.Code)
	}
	if empty.Code == http.StatusOK || empty.Code == http.StatusBadRequest {
		if !strings.Contains(empty.Body.String(), "선택") {
			t.Fatal("선택 안내 없음")
		}
	}
	noCycle := doFormUser(t, e, "/maintenance/sites/bulk", url.Values{
		"industry": {"학교"}, "customer_id": {"h15a"},
	}, oa)
	if noCycle.Code == http.StatusSeeOther && noCycle.Header().Get("Location") == "/maintenance/sites" {
		t.Fatal("주기 없이 등록되면 안 됨")
	}
	xlsx := doGetWith(t, e, "/maintenance/sites/bulk/skips.xlsx", oa)
	if xlsx.Code != http.StatusOK {
		t.Fatalf("xlsx %d", xlsx.Code)
	}
	ct := xlsx.Header().Get("Content-Type")
	if !strings.Contains(ct, "spreadsheet") && !strings.Contains(ct, "excel") && !strings.Contains(ct, "octet") {
		t.Fatalf("xlsx type %s", ct)
	}
	if len(xlsx.Body.Bytes()) > 0 {
		assertExcelizeRGB8(t, xlsx.Body.Bytes())
	}
}

func TestMnt95_15BulkHTMLUILintZero(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("web", "templates", "maintenance", "bulk.html"))
	if err != nil {
		b, err = os.ReadFile(filepath.Join("..", "..", "web", "templates", "maintenance", "bulk.html"))
	}
	if err != nil {
		t.Fatal(err)
	}
	got := countUILint(string(b))
	if !got.zero() {
		t.Fatalf("bulk.html ui_lint %+v", got)
	}
}

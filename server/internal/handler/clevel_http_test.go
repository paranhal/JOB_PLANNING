package handler

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func newCLevelServer(t *testing.T) (*echo.Echo, *model.User, *model.User) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "clevel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	_ = users.EnsureAdmin(HashPassword("adminadmin"))
	ceo := &model.User{
		Username: "ceo1", PasswordHash: HashPassword("password10"), FullName: "대표",
		Role: model.RoleCLevel, IsActive: true, IsReadOnly: true, ProfileDone: true,
	}
	if err := users.Create(ceo); err != nil {
		t.Fatal(err)
	}
	orgAdm := &model.User{
		Username: "oa1", PasswordHash: HashPassword("password10"), FullName: "팀장",
		Role: model.RoleOrgAdmin, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true,
	}
	if err := users.Create(orgAdm); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order)
		VALUES('O04','O04','경영지원','support',1,40)`); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.Use(h.Auth.GuardCLevelHidden)
	g.Use(h.Auth.RequireActiveRole)
	g.GET("/", h.Dashboard)
	g.GET("/exec", h.Exec.Dashboard, h.Auth.RequireExecView)
	g.GET("/exec/work", h.Exec.Work, h.Auth.RequireExecView)
	g.GET("/exec/org/:id", h.Exec.OrgSoon, h.Auth.RequireExecView)
	g.GET("/admin/users", h.Auth.UserList, h.Auth.RequireAdminMW)
	g.GET("/workboard/register", h.Workboard.Register)
	g.GET("/work", h.Work.List)
	g.GET("/account", h.Auth.AccountPage)
	g.POST("/workboard/tasks", h.Workboard.CreateTask)
	return e, ceo, orgAdm
}

func TestCLevelHiddenGET403(t *testing.T) {
	e, ceo, _ := newCLevelServer(t)
	ck := jwtCookieUser(t, ceo)
	for _, path := range []string{"/admin/users", "/workboard/register", "/work"} {
		rec := doGetWith(t, e, path, ck)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s → %d want 403", path, rec.Code)
		}
	}
}

func TestCLevelExecOKAndHomeRedirect(t *testing.T) {
	e, ceo, _ := newCLevelServer(t)
	ck := jwtCookieUser(t, ceo)
	home := doGetWith(t, e, "/", ck)
	if home.Code != http.StatusSeeOther || home.Header().Get("Location") != "/exec" {
		t.Fatalf("홈 %d loc=%s", home.Code, home.Header().Get("Location"))
	}
	page := doGetWith(t, e, "/exec", ck)
	if page.Code != http.StatusOK {
		t.Fatalf("/exec %d", page.Code)
	}
	if !strings.Contains(page.Body.String(), "경영 통합 대시보드") {
		t.Fatal("/exec 제목 없음")
	}
	work := doGetWith(t, e, "/exec/work", ck)
	if work.Code != http.StatusOK {
		t.Fatalf("/exec/work %d", work.Code)
	}
	soon := doGetWith(t, e, "/exec/org/O04", ck)
	if soon.Code != http.StatusOK {
		t.Fatalf("/exec/org/O04 %d", soon.Code)
	}
}

func TestOrgAdminExec403(t *testing.T) {
	e, _, oa := newCLevelServer(t)
	ck := jwtCookieUser(t, oa)
	for _, path := range []string{"/exec", "/exec/work"} {
		rec := doGetWith(t, e, path, ck)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s → %d want 403", path, rec.Code)
		}
	}
}

func TestCLevelWritePOST403(t *testing.T) {
	e, ceo, _ := newCLevelServer(t)
	ck := jwtCookieUser(t, ceo)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/workboard/tasks", strings.NewReader("title=x"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST %d", rec.Code)
	}
}

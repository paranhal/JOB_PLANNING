package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/repository"
)

func TestOrgAdminListCreateUpdateAndSwitch(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "org-http.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/admin/orgs", h.Org.List, h.Auth.RequireVisionOnly)
	g.POST("/admin/orgs", h.Org.Create, h.Auth.RequireVisionOnly)
	g.POST("/admin/orgs/update", h.Org.Update, h.Auth.RequireVisionOnly)
	g.POST("/admin/orgs/switch", h.Org.Switch, h.Auth.RequireVisionOnly)

	ck := jwtCookie(t)
	get := func() *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin/orgs", nil)
		req.AddCookie(ck)
		e.ServeHTTP(rec, req)
		return rec
	}
	list := get()
	if list.Code != http.StatusOK {
		t.Fatalf("list %d %s", list.Code, list.Body.String())
	}
	body := list.Body.String()
	if !strings.Contains(body, "도서관사업팀") || !strings.Contains(body, "계정") {
		t.Fatalf("목록에 건수가 없다: %s", body)
	}
	if !strings.Contains(body, "조직 관리") {
		t.Fatal("사이드바 조직 관리가 없다")
	}

	form := url.Values{"org_name": {"다른팀"}, "short_name": {"이팀"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/orgs", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create %d %s", rec.Code, rec.Body.String())
	}
	list = get()
	if !strings.Contains(list.Body.String(), "O02") || !strings.Contains(list.Body.String(), "다른팀") {
		t.Fatalf("생성 후 목록: %s", list.Body.String())
	}

	upd := url.Values{"org_id": {"O02"}, "org_name": {"바꾼이름"}, "is_active": {"1"}}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/admin/orgs/update", strings.NewReader(upd.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	list = get()
	if !strings.Contains(list.Body.String(), "바꾼이름") {
		t.Fatalf("수정 후: %s", list.Body.String())
	}

	sw := url.Values{"view_org_id": {"*"}, "return": {"/admin/orgs"}}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/admin/orgs/switch", strings.NewReader(sw.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("switch %d", rec.Code)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "token" && c.Value != "" {
			ck = c
		}
	}
	list = get()
	if !strings.Contains(list.Body.String(), "전 조직") || !strings.Contains(list.Body.String(), "bg-amber-600") {
		t.Fatalf("전 조직 띠가 없다: %s", list.Body.String())
	}
}

func TestOrgAdminForbiddenForTech(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "org-tech.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/admin/orgs", h.Org.List, h.Auth.RequireVisionOnly)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/orgs", nil)
	req.AddCookie(jwtCookieRole(t, "tech"))
	e.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), "조직 만들기") {
		t.Fatal("기술 계정이 조직 관리에 들어갔다")
	}
}

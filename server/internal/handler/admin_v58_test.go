package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestAdminV58MembersAndPermissions(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "v58.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	_ = users.EnsureAdmin(HashPassword("admin"))
	_ = users.Create(&model.User{
		Username: "lee", PasswordHash: HashPassword("x"), FullName: "이기술",
		Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary,
	})

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/admin/orgs", h.Org.List, h.Auth.RequireCanViewOrgs)
	g.GET("/admin/my-org/members", h.Auth.MembersList, h.Auth.RequireAdminMW)
	g.POST("/admin/my-org/members/:id", h.Auth.MemberUpdate, h.Auth.RequireAdminMW)
	g.GET("/admin/permissions", h.Auth.PermissionsPage, h.Auth.RequireVisionOnly)
	g.POST("/admin/permissions/save", h.Auth.PermissionsSave, h.Auth.RequireVisionOnly)

	ck := jwtVisionCookie(t)
	get := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(ck)
		e.ServeHTTP(rec, req)
		return rec
	}
	orgs := get("/admin/orgs")
	if orgs.Code != http.StatusOK || !strings.Contains(orgs.Body.String(), "전체 조직 관리") {
		t.Fatalf("orgs %d", orgs.Code)
	}
	if !strings.Contains(orgs.Body.String(), "소속 인원 관리") || !strings.Contains(orgs.Body.String(), "역할 및 권한") {
		t.Fatal("사이드바 메뉴 없음")
	}
	mem := get("/admin/my-org/members")
	if mem.Code != http.StatusOK {
		t.Fatalf("members %d %s", mem.Code, mem.Body.String())
	}
	perms := get("/admin/permissions?tab=work")
	if perms.Code != http.StatusOK || !strings.Contains(perms.Body.String(), "업무별 권한") {
		t.Fatalf("perms %d", perms.Code)
	}
	if !strings.Contains(perms.Body.String(), "본인 것만") {
		t.Fatal("네 단계 선택이 없다")
	}

	lee, _ := users.GetByUsername("lee")
	rec := httptest.NewRecorder()
	form := url.Values{"full_name": {"이기술"}, "job": {"sales"}, "is_active": {"1"}}
	req := httptest.NewRequest(http.MethodPost, "/admin/my-org/members/"+lee.UserID, strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusSeeOther {
		t.Fatalf("vision member update %d", rec.Code)
	}
}

func TestAdminV58OrgAdminForeignMemberIs404(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "v58-org.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	oa := &model.User{
		Username: "oa1", PasswordHash: HashPassword("x"), FullName: "조직장",
		Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01",
	}
	other := &model.User{
		Username: "xx", PasswordHash: HashPassword("x"), FullName: "다른팀",
		Role: model.RoleTech, IsActive: true, OrgID: "O02",
	}
	if err := users.Create(oa); err != nil {
		t.Fatal(err)
	}
	if err := users.Create(other); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.POST("/admin/my-org/members/:id", h.Auth.MemberUpdate, h.Auth.RequireAdminMW)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": oa.UserID, "username": oa.Username, "role": model.RoleOrgAdmin,
		"name": oa.FullName, "org_id": "O01", "exp": time.Now().Add(time.Hour).Unix(),
	})
	s, err := token.SignedString([]byte("cs-system-jwt-secret-2026"))
	if err != nil {
		t.Fatal(err)
	}
	ck := &http.Cookie{Name: "token", Value: s}
	rec := httptest.NewRecorder()
	form := url.Values{"full_name": {"해킹"}, "job": {"sales"}}
	req := httptest.NewRequest(http.MethodPost, "/admin/my-org/members/"+other.UserID, strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
}

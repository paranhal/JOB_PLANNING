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
	"customer-support/internal/passwd"
	"customer-support/internal/repository"
)

func v58UsersApp(t *testing.T) (*echo.Echo, *repository.UserRepo) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "v58-users.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,sort_order) VALUES('O02','O02','이팀',2)`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	_ = users.EnsureAdmin(HashPassword("adminadmin"))
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/admin/users", h.Auth.UserList)
	g.GET("/users", h.Auth.UsersLegacyRedirect)
	g.POST("/users/:id/update", h.Auth.UserUpdate)
	g.POST("/admin/my-org/members", h.Auth.MemberCreate)
	g.POST("/admin/my-org/members/:id/delete", h.Auth.MemberDelete)
	g.GET("/admin/permissions", h.Auth.PermissionsPage, h.Auth.RequireVisionOnly)
	return e, users
}

func jwtOrgCookie(t *testing.T, u *model.User) *http.Cookie {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": u.UserID, "username": u.Username, "role": u.Role,
		"name": u.FullName, "org_id": u.OrgID, "exp": time.Now().Add(time.Hour).Unix(),
	})
	s, err := token.SignedString([]byte("cs-system-jwt-secret-2026"))
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: "token", Value: s, Path: "/"}
}

func TestUsersPathRedirectsToAdminUsers(t *testing.T) {
	e, _ := v58UsersApp(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users?ok=saved", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status=%d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if loc != "/admin/users?ok=saved" {
		t.Fatalf("loc=%s", loc)
	}
	page := httptest.NewRecorder()
	preq := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
	preq.AddCookie(jwtCookie(t))
	e.ServeHTTP(page, preq)
	if page.Code != http.StatusOK {
		t.Fatalf("admin users %d", page.Code)
	}
	if !strings.Contains(page.Body.String(), `href="/admin/users"`) {
		t.Fatal("사이드바가 /admin/users 가 아니다")
	}
}

func TestOrgAdminUsersListAndForeignUpdate404(t *testing.T) {
	e, users := v58UsersApp(t)
	oa := &model.User{
		Username: "oa58", PasswordHash: HashPassword("password10"), FullName: "일팀장",
		Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01", ProfileDone: true,
	}
	home := &model.User{
		Username: "home58", PasswordHash: HashPassword("password10"), FullName: "일팀원",
		Role: model.RoleTech, IsActive: true, OrgID: "O01", ProfileDone: true,
	}
	away := &model.User{
		Username: "away58", PasswordHash: HashPassword("password10"), FullName: "이팀숨김",
		Role: model.RoleTech, IsActive: true, OrgID: "O02", ProfileDone: true,
	}
	for _, u := range []*model.User{oa, home, away} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	ck := jwtOrgCookie(t, oa)
	list := httptest.NewRecorder()
	lreq := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
	lreq.AddCookie(ck)
	e.ServeHTTP(list, lreq)
	if list.Code != http.StatusOK {
		t.Fatalf("list %d", list.Code)
	}
	body := list.Body.String()
	if !strings.Contains(body, "일팀원") {
		t.Fatal("자기 조직원이 목록에 없다")
	}
	if strings.Contains(body, "이팀숨김") || strings.Contains(body, "away58") {
		t.Fatal("다른 조직원이 목록에 보인다")
	}

	upd := httptest.NewRecorder()
	form := url.Values{
		"full_name": {"해킹"}, "admin_grade": {"none"}, "job": {"tech"},
		"org_id": {"O02"}, "is_active": {"1"}, "prev_role": {model.RoleTech},
	}
	ureq := httptest.NewRequest(http.MethodPost, "/users/"+away.UserID+"/update", strings.NewReader(form.Encode()))
	ureq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	ureq.AddCookie(ck)
	e.ServeHTTP(upd, ureq)
	if upd.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d loc=%s", upd.Code, upd.Header().Get("Location"))
	}
}

func TestMemberCreateAndDeleteUseSingleAccountFuncs(t *testing.T) {
	e, users := v58UsersApp(t)
	oa := &model.User{
		Username: "oa-add", PasswordHash: HashPassword("password10"), FullName: "추가장",
		Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01", ProfileDone: true,
	}
	if err := users.Create(oa); err != nil {
		t.Fatal(err)
	}
	ck := jwtOrgCookie(t, oa)
	pw := strings.Repeat("p", passwd.MinLen)
	rec := httptest.NewRecorder()
	form := url.Values{
		"username": {"newmem58"}, "full_name": {"신규소속"}, "password": {pw},
		"admin_grade": {"none"}, "job": {"tech"}, "is_active": {"1"},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/my-org/members", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("create loc=%s", rec.Header().Get("Location"))
	}
	got, _ := users.GetByUsername("newmem58")
	if got != nil {
		t.Fatal("조직관리자가 계정을 바로 만들었다")
	}

	del := httptest.NewRecorder()
	dreq := httptest.NewRequest(http.MethodPost, "/admin/my-org/members/x/delete", nil)
	dreq.AddCookie(ck)
	e.ServeHTTP(del, dreq)
	if del.Code != http.StatusForbidden && del.Code != http.StatusOK {
		t.Fatalf("삭제는 막혀야 한다 code=%d", del.Code)
	}
}

func TestPermissionsForbiddenForOrgAdmin(t *testing.T) {
	e, users := v58UsersApp(t)
	oa := &model.User{
		Username: "oa-perm", PasswordHash: HashPassword("password10"), FullName: "권한장",
		Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01",
	}
	if err := users.Create(oa); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/permissions", nil)
	req.AddCookie(jwtOrgCookie(t, oa))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d", rec.Code)
	}
}

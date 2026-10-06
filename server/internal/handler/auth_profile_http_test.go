package handler

import (
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

func newUserProfileApp(t *testing.T) (*echo.Echo, *repository.UserRepo) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "profile.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	users.EnsureAdmin(HashPassword("admin"))
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/users", h.Auth.UserList)
	g.POST("/users", h.Auth.UserCreate)
	g.POST("/users/:id/update", h.Auth.UserUpdate)
	g.GET("/account/complete", h.Auth.AccountCompleteForm)
	g.POST("/account/complete", h.Auth.AccountComplete)
	g.POST("/login", h.Auth.Login)
	g.GET("/", h.Dashboard)
	return e, users
}

func TestUserCreateAllowsTechWithoutMobile(t *testing.T) {
	e, _ := newUserProfileApp(t)
	form := url.Values{
		"username": {"tech1"}, "password": {"password10"}, "full_name": {"기술일"},
		"role": {"tech"}, "org_id": {model.OrgIDLibrary},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("연락처는 첫 로그인에 채운다 loc=%s", rec.Header().Get("Location"))
	}
}

func TestUserCreateAllowsOrgAdminWithoutEmail(t *testing.T) {
	e, _ := newUserProfileApp(t)
	form := url.Values{
		"username": {"adm2"}, "password": {"password10"}, "full_name": {"관리이"},
		"role": {"org_admin"}, "org_id": {model.OrgIDLibrary},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("이메일은 첫 로그인에 채운다 loc=%s", rec.Header().Get("Location"))
	}
}

func TestLoginIncompleteProfileGoesToComplete(t *testing.T) {
	e, users := newUserProfileApp(t)
	u := &model.User{
		Username: "techx", PasswordHash: HashPassword("pw"), FullName: "기술엑스",
		Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary,
	}
	if err := users.Create(u); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	form := url.Values{"username": {"techx"}, "password": {"pw"}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/account/complete?ok=1" {
		t.Fatalf("내 정보 채우기로 안 감 loc=%s", rec.Header().Get("Location"))
	}

	home := httptest.NewRecorder()
	hreq := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, ck := range rec.Result().Cookies() {
		hreq.AddCookie(ck)
	}
	e.ServeHTTP(home, hreq)
	if home.Code != http.StatusSeeOther || home.Header().Get("Location") != "/account/complete" {
		t.Fatalf("홈이 막히지 않음 code=%d loc=%s", home.Code, home.Header().Get("Location"))
	}
}

func TestUserListShowsContactFields(t *testing.T) {
	e, _ := newUserProfileApp(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"핸드폰", "전화", "이메일", "조직", "사인 저장", "직접 그리기", "dSigPad", "novalidate"} {
		if !strings.Contains(body, want) {
			t.Fatalf("%s 칸이 없다", want)
		}
	}
}

func TestUserUpdateAllowsEmptyContactAndOrg(t *testing.T) {
	e, users := newUserProfileApp(t)
	u := &model.User{
		Username: "sparse", PasswordHash: HashPassword("pw"), FullName: "빈칸",
		Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, Mobile: "010-1",
	}
	if err := users.Create(u); err != nil {
		t.Fatal(err)
	}
	form := url.Values{"full_name": {"빈칸"}, "admin_grade": {"none"}, "job": {"tech"}, "is_active": {"1"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/users/"+u.UserID+"/update", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("빈 연락처 저장 거부 loc=%s", rec.Header().Get("Location"))
	}
	got, _ := users.GetByID(u.UserID)
	if got == nil || strings.TrimSpace(got.Mobile) != "" || strings.TrimSpace(got.OrgID) != "" {
		t.Fatalf("비운 값이 안 남았다: %+v", got)
	}
}

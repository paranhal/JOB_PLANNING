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

func sessionCookieApp(t *testing.T) (*echo.Echo, *Handler, *repository.UserRepo) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "sess35.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	if err := users.EnsureAdmin(HashPassword("admin")); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	e.POST("/login", h.Auth.Login)
	e.GET("/logout", h.Auth.Logout)
	g.GET("/", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})
	g.POST("/admin/orgs/switch", h.Org.Switch, h.Auth.RequireVisionOnly)
	g.POST("/account/password", h.Auth.AccountChangePassword)
	return e, h, users
}

func tokenSetCookieLine(rec *httptest.ResponseRecorder) string {
	for _, h := range rec.Result().Header.Values("Set-Cookie") {
		if strings.HasPrefix(h, "token=") {
			return h
		}
	}
	return ""
}

func tokenCookieFrom(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == "token" {
			return ck
		}
	}
	return nil
}

func assertSessionTokenSetCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	line := tokenSetCookieLine(rec)
	if line == "" {
		t.Fatalf("Set-Cookie token 없음 headers=%v", rec.Result().Header.Values("Set-Cookie"))
	}
	low := strings.ToLower(line)
	if strings.Contains(low, "max-age") {
		t.Fatalf("세션 쿠키에 Max-Age 가 있다: %s", line)
	}
	if strings.Contains(low, "expires=") {
		t.Fatalf("세션 쿠키에 Expires 가 있다: %s", line)
	}
	if !strings.Contains(low, "httponly") {
		t.Fatalf("HttpOnly 없음: %s", line)
	}
	ck := tokenCookieFrom(rec)
	if ck == nil || ck.Value == "" {
		t.Fatal("token 값이 없다")
	}
	if ck.MaxAge != 0 {
		t.Fatalf("MaxAge=%d want 0", ck.MaxAge)
	}
	if !ck.Expires.IsZero() {
		t.Fatalf("Expires=%v want zero", ck.Expires)
	}
	if ck.Path != "/" || !ck.HttpOnly || ck.SameSite != http.SameSiteLaxMode {
		t.Fatalf("쿠키 속성 %+v", ck)
	}
	return ck
}

func assertDeletedTokenCookie(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	line := tokenSetCookieLine(rec)
	if line == "" {
		t.Fatal("삭제 Set-Cookie 없음")
	}
	low := strings.ToLower(line)
	if !strings.Contains(low, "max-age=0") {
		t.Fatalf("삭제 쿠키 Max-Age=0 없음: %s", line)
	}
	ck := tokenCookieFrom(rec)
	if ck == nil {
		t.Fatal("token 쿠키 없음")
	}
	if ck.MaxAge >= 0 && ck.Value != "" {
		t.Fatalf("삭제되지 않음 %+v line=%s", ck, line)
	}
}

func jwtExpUnix(t *testing.T, tokenStr string) int64 {
	t.Helper()
	tok, _, err := jwt.NewParser().ParseUnverified(tokenStr, jwt.MapClaims{})
	if err != nil {
		t.Fatal(err)
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("claims")
	}
	switch v := claims["exp"].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	default:
		t.Fatalf("exp %T", claims["exp"])
		return 0
	}
}

func TestSessionCookieLoginHasNoMaxAge(t *testing.T) {
	e, _, _ := sessionCookieApp(t)
	rec := postForm(t, e, "/login", url.Values{"username": {"admin"}, "password": {"admin"}}, nil)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("login %d", rec.Code)
	}
	ck := assertSessionTokenSetCookie(t, rec)
	exp := jwtExpUnix(t, ck.Value)
	want := time.Now().Add(24 * time.Hour).Unix()
	if exp < want-60 || exp > want+120 {
		t.Fatalf("JWT exp=%d want ~%d", exp, want)
	}
}

func TestSessionCookieOrgSwitchAndRefresh(t *testing.T) {
	e, _, users := sessionCookieApp(t)
	admin, _ := users.GetByUsername("admin")
	if admin == nil {
		t.Fatal("admin")
	}
	admin.Role = model.RoleVisionAdmin
	if err := users.Update(admin); err != nil {
		t.Fatal(err)
	}
	login := postForm(t, e, "/login", url.Values{"username": {"admin"}, "password": {"admin"}}, nil)
	ck := assertSessionTokenSetCookie(t, login)

	sw := postForm(t, e, "/admin/orgs/switch", url.Values{"view_org_id": {"*"}, "return": {"/"}}, ck)
	if sw.Code != http.StatusSeeOther {
		t.Fatalf("switch %d", sw.Code)
	}
	assertSessionTokenSetCookie(t, sw)

	pw := postForm(t, e, "/account/password", url.Values{
		"current_password": {"admin"}, "password": {"password10"}, "password_confirm": {"password10"},
	}, ck)
	if pw.Code != http.StatusSeeOther && pw.Code != http.StatusOK {
		t.Fatalf("password %d %s", pw.Code, pw.Body.String())
	}
	if tokenSetCookieLine(pw) != "" {
		assertSessionTokenSetCookie(t, pw)
	}
}

func TestSessionCookieRevalidateIssuesSessionCookie(t *testing.T) {
	e, h, users := sessionCookieApp(t)
	admin, _ := users.GetByUsername("admin")
	if admin == nil {
		t.Fatal("admin")
	}
	claims := sessionClaims(admin, false)
	claims["verified_at"] = time.Now().Add(-6 * time.Minute).Unix()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := token.SignedString(h.Auth.jwtSecret)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: s, Path: "/"})
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("revalidate %d %s", rec.Code, rec.Body.String())
	}
	assertSessionTokenSetCookie(t, rec)
}

func TestLogoutDeletesTokenCookie(t *testing.T) {
	e, _, _ := sessionCookieApp(t)
	login := postForm(t, e, "/login", url.Values{"username": {"admin"}, "password": {"admin"}}, nil)
	ck := tokenCookieFrom(login)
	if ck == nil {
		t.Fatal("login cookie")
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/logout", nil)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login" {
		t.Fatalf("logout %d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
	assertDeletedTokenCookie(t, rec)
}

func TestInactiveUserRevalidateClearsCookie(t *testing.T) {
	e, h, users := sessionCookieApp(t)
	u := &model.User{
		Username: "gone35", PasswordHash: HashPassword("password10"),
		FullName: "비활성", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true,
	}
	if err := users.Create(u); err != nil {
		t.Fatal(err)
	}
	got, _ := users.GetByUsername("gone35")
	if got == nil {
		t.Fatal("user")
	}
	claims := sessionClaims(got, false)
	claims["verified_at"] = time.Now().Add(-6 * time.Minute).Unix()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := token.SignedString(h.Auth.jwtSecret)
	if err != nil {
		t.Fatal(err)
	}
	got.IsActive = false
	if err := users.Update(got); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: s, Path: "/"})
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login" {
		t.Fatalf("inactive %d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
	assertDeletedTokenCookie(t, rec)
}

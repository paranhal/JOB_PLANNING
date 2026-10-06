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

	"customer-support/internal/repository"
)

func newAdminUnlockApp(t *testing.T) (*echo.Echo, *sql.DB) {
	t.Helper()
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "admin-unlock.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	settings := repository.NewSettingsRepo(db)
	if err := settings.Set(repository.SettingVisionAdminPassword, HashPassword("vision-lock")); err != nil {
		t.Fatal(err)
	}
	if err := settings.Set(repository.SettingASCompletedEditPassword, HashPassword("as-edit")); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	sec := h.Auth.RequireAdminSection
	adminOnly := h.Auth.RequireAdminMW
	g.GET("/admin/unlock", h.Auth.AdminUnlockForm)
	g.POST("/admin/unlock", h.Auth.AdminUnlock)
	g.GET("/users", h.Auth.UserList, sec, adminOnly)
	g.GET("/codes", h.Code.List, sec, adminOnly)
	return e, db
}

func TestAdminSectionLockedUntilVisionPassword(t *testing.T) {
	e, db := newAdminUnlockApp(t)
	ck := jwtCookie(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "/admin/unlock") {
		t.Fatalf("잠금 전 %d loc=%s", rec.Code, rec.Header().Get("Location"))
	}

	bad := httptest.NewRecorder()
	form := url.Values{"unlock_password": {"as-edit"}, "return": {"/users"}}
	breq := httptest.NewRequest(http.MethodPost, "/admin/unlock", strings.NewReader(form.Encode()))
	breq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	breq.AddCookie(ck)
	e.ServeHTTP(bad, breq)
	if bad.Code != http.StatusSeeOther || !strings.Contains(bad.Header().Get("Location"), "err=") {
		t.Fatalf("AS 비밀번호가 통했다 loc=%s", bad.Header().Get("Location"))
	}

	ok := httptest.NewRecorder()
	form = url.Values{"unlock_password": {"vision-lock"}, "return": {"/users"}}
	oreq := httptest.NewRequest(http.MethodPost, "/admin/unlock", strings.NewReader(form.Encode()))
	oreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	oreq.AddCookie(ck)
	e.ServeHTTP(ok, oreq)
	if ok.Code != http.StatusSeeOther || strings.Contains(ok.Header().Get("Location"), "err=") {
		t.Fatalf("해제 실패 loc=%s", ok.Header().Get("Location"))
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/users", nil)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "사용자") {
		t.Fatalf("해제 후 %d", rec.Code)
	}

	var fails, oks int
	if err := db.QueryRow(`SELECT COALESCE(SUM(CASE WHEN success=0 THEN 1 ELSE 0 END),0), COALESCE(SUM(CASE WHEN success=1 THEN 1 ELSE 0 END),0) FROM admin_unlock_log`).Scan(&fails, &oks); err != nil {
		t.Fatal(err)
	}
	if fails < 1 || oks < 1 {
		t.Fatalf("로그 fail=%d ok=%d", fails, oks)
	}
}

func TestAdminSectionObserverSkipsUnlock(t *testing.T) {
	e, _ := newAdminUnlockApp(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	req.AddCookie(jwtCookieReadOnly(t, "tech"))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("옵저버가 잠긴다 %d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
}

func TestAdminSectionTesterSkipsUnlock(t *testing.T) {
	e, _ := newAdminUnlockApp(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	req.AddCookie(jwtCookieRole(t, "tester"))
	e.ServeHTTP(rec, req)
	if rec.Code == http.StatusSeeOther && strings.Contains(rec.Header().Get("Location"), "/admin/unlock") {
		t.Fatal("테스터에게 관리 비밀번호를 물었다")
	}
}

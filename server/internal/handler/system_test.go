package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/repository"
)

func TestSystemPageAdminOnly(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "sys.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	repository.NewUserRepo(db).EnsureObserver(HashPassword("1234"))
	SetBuildInfo("v2.37", "a1b2c3d", "2026-09-13 14:22", time.Date(2026, 9, 13, 14, 25, 0, 0, time.Local))
	h := New(db)
	e := echo.New()
	e.Renderer = NewRenderer()
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/admin/system", h.System.Page, h.Auth.RequireAdminOnly)

	admin := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/system", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(admin, req)
	if admin.Code != http.StatusOK {
		t.Fatalf("admin status=%d body=%s", admin.Code, admin.Body.String())
	}
	body := admin.Body.String()
	if !strings.Contains(body, "시스템 정보") {
		t.Fatal("제목 없음")
	}
	if !strings.Contains(body, "a1b2c3d") {
		t.Fatal("관리자 화면에 커밋이 없다")
	}

	obs := httptest.NewRecorder()
	oreq := httptest.NewRequest(http.MethodGet, "/admin/system", nil)
	oreq.AddCookie(jwtCookieRole(t, "observer"))
	e.ServeHTTP(obs, oreq)
	if obs.Code != http.StatusForbidden {
		t.Fatalf("observer status=%d body=%s", obs.Code, obs.Body.String())
	}
}

func TestVersionHasNoHistory(t *testing.T) {
	SetBuildInfo("v2.37", "a1b2c3d", "2026-09-13 14:22", time.Date(2026, 9, 13, 14, 25, 0, 0, time.Local))
	e := echo.New()
	e.GET("/version", VersionJSON)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	raw := rec.Body.String()
	for _, ban := range []string{"history", "first_started", "start_count", "app_versions"} {
		if strings.Contains(raw, ban) {
			t.Fatalf("/version 에 %s 가 있다: %s", ban, raw)
		}
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["version"]; !ok {
		t.Fatalf("json=%v", got)
	}
	if _, ok := got["history"]; ok {
		t.Fatal("이력이 있다")
	}
}

func TestSchemaStaleBannerOnLogin(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "stale.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := repository.SetAppliedSchemaVersionForTest(db, 38); err != nil {
		t.Fatal(err)
	}
	h := New(db)
	e := echo.New()
	e.Renderer = NewRenderer()
	e.GET("/login", h.Auth.LoginPage)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	want := "DB 스키마가 낡았습니다 — " + repository.SchemaNo(repository.AppSchemaVersion) + " 필요, 현재 038"
	if !strings.Contains(rec.Body.String(), want) {
		t.Fatalf("띠 없음: %s", rec.Body.String())
	}
}

func TestSystemHistoryShowsRollback(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "rb.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	_ = repository.RecordAppStart(db, repository.AppStart{
		Version: "v2.36", Commit: "ccccccc", BuiltAt: "2026-09-13 14:22", StartedAt: "2026-09-13 14:25",
	})
	_ = repository.RecordAppStart(db, repository.AppStart{
		Version: "v2.35", Commit: "ddddddd", BuiltAt: "2026-09-10 18:40", StartedAt: "2026-09-13 16:00",
	})
	h := New(db)
	e := echo.New()
	e.Renderer = NewRenderer()
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/admin/system", h.System.Page, h.Auth.RequireAdminOnly)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/system", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "🔙 롤백") {
		t.Fatalf("롤백 표시 없음: %s", rec.Body.String())
	}
}

func TestSidebarHasSystemInfoForAdmin(t *testing.T) {
	body, err := os.ReadFile("web/templates/layout/base.html")
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	if !strings.Contains(s, `href="/admin/system"`) || !strings.Contains(s, "시스템 정보") {
		t.Fatal("사이드바에 시스템 정보가 없다")
	}
	if !strings.Contains(s, `id="schema-mismatch-banner"`) {
		t.Fatal("스키마 경고 띠가 없다")
	}
}

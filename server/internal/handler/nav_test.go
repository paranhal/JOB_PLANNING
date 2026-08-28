package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/repository"
)

func TestNavConstantsMatchSidebar(t *testing.T) {
	body, err := os.ReadFile("web/templates/layout/base.html")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`eq \$a "([a-z_]+)"`)
	found := map[string]struct{}{}
	for _, m := range re.FindAllSubmatch(body, -1) {
		found[string(m[1])] = struct{}{}
	}
	if len(found) == 0 {
		t.Fatal("사이드바 Active 키가 없다")
	}
	have := map[string]struct{}{}
	for _, k := range navSidebarKeys() {
		have[k] = struct{}{}
		if _, ok := found[k]; !ok {
			t.Errorf("상수 %q 가 base.html 사이드바에 없다", k)
		}
	}
	for k := range found {
		if _, ok := have[k]; !ok {
			t.Errorf("사이드바 키 %q 에 대응하는 상수가 없다", k)
		}
	}
}

func TestWorkListSidebarHighlightsWork(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "work-nav.db"))
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
	g.GET("/work", h.Work.List)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/work", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, `href="/" class="bg-blue-600`) {
		t.Fatal("대시보드가 강조되고 있다")
	}
	if !strings.Contains(body, `href="/work" class="bg-blue-600`) {
		t.Fatal("오늘 내 업무가 강조되지 않는다")
	}
}

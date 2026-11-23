package handler

import (
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-219: 마일스톤 리드타임 추이를 실제 브라우저(1440/820/390)로 확인한다.
func Test95_219_Browser(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "ms219b.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	seedLead219(t, db)
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	e.GET("/stats/milestones", h.Stats.Milestones, h.Auth.AuthMiddleware)
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	oa := jwtCookieOrg(t, model.RoleOrgAdmin, model.OrgIDLibrary)
	shot := os.Getenv("SCREENSHOT_DIR")
	if shot == "" {
		shot, _ = filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-219"))
	}
	cmd := exec.Command("node", "95_219_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(), "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1", "BASE_URL="+srv.URL, "AUTH_COOKIE="+oa.Value, "SCREENSHOT_DIR="+shot)
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		so := string(out)
		if strings.Contains(so, "Executable doesn't exist") || strings.Contains(so, "browserType.launch") {
			t.Skip(so)
		}
		t.Fatalf("browser: %v", err)
	}
}

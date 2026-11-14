package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

// 95-198: 배포 뒤 옛 weekly-link.js 캐시 시나리오를 실제 브라우저(HTTP 캐시 유지)로 본다.
// /static/ 응답은 운영과 같이 Cache-Control: public, max-age=86400 을 준다.
// 옛 빌드(?v=OLD)에는 95-197 이전 동작(활동을 골라도 버튼이 안 켜짐)의 스크립트를, 새 빌드에는 현재 스크립트를 내려 준다.
func Test95_198_BrowserStaleScriptCache(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	v := seed197(t)
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	cur, err := os.ReadFile(filepath.Join(staticDir, "js", "weekly-link.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cur), "var ok = !!proj || !!act;") {
		t.Fatal("현재 weekly-link.js 에서 95-197 동작을 찾지 못했다")
	}
	oldJS := strings.Replace(string(cur), "var ok = !!proj || !!act;", "var ok = !!proj;", 1)

	SetBuildInfo("95-197", "abc1234", "2026-11-14 22:00", time.Now())
	oldQ := AssetQuery()
	var mu sync.Mutex
	hits := map[string]int{}

	v.e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if strings.HasPrefix(c.Request().URL.Path, "/static/") {
				c.Response().Header().Set("Cache-Control", "public, max-age=86400") // cmd/server/main.go 와 같다
			}
			return next(c)
		}
	})
	v.e.GET("/static/js/weekly-link.js", func(c echo.Context) error {
		q := c.QueryParam("v")
		mu.Lock()
		hits[q]++
		mu.Unlock()
		body := string(cur)
		if q == oldQ {
			body = oldJS
		}
		return c.Blob(http.StatusOK, "application/javascript", []byte(body))
	})
	v.e.GET("/__hits", func(c echo.Context) error {
		mu.Lock()
		defer mu.Unlock()
		return c.JSON(http.StatusOK, hits)
	})
	v.e.GET("/__build", func(c echo.Context) error { return c.JSON(http.StatusOK, map[string]string{"q": AssetQuery()}) })
	v.e.GET("/__setbuild", func(c echo.Context) error {
		SetBuildInfo("95-198", "def5678", "2026-11-15 07:00", time.Now())
		return c.JSON(http.StatusOK, map[string]string{"q": AssetQuery()})
	})
	v.e.Static("/static", staticDir)
	srv := httptest.NewServer(v.e)
	t.Cleanup(srv.Close)

	shot, _ := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-198"))
	cmd := exec.Command("node", "95_198_cache_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(), "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1", "BASE_URL="+srv.URL,
		"AUTH_COOKIE="+jwtCookieOrg(t, model.RoleOrgAdmin, "O03").Value, "SCREENSHOT_DIR="+shot)
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		s := string(out)
		if strings.Contains(s, "Executable doesn't exist") || strings.Contains(s, "browserType.launch") {
			t.Skip(s)
		}
		t.Fatalf("browser: %v", err)
	}
}

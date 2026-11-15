package handler

import (
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// 95-206: 실제 브라우저(1440/1280/820/390)로 사용자 목록·수정 진입·서랍 저장을 누른다. TEST 계정만.
func Test95_206_BrowserUsers(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	v := setup206(t)
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	v.e.Static("/static", staticDir)
	srv := httptest.NewServer(v.e)
	t.Cleanup(srv.Close)
	shot, _ := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-206"))
	cmd := exec.Command("node", "95_206_users_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(), "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1", "BASE_URL="+srv.URL,
		"AUTH_COOKIE="+visionCookie(t, v.vs).Value, "SCREENSHOT_DIR="+shot)
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

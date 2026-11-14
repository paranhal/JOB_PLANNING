package handler

import (
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// 95-195: 사업 등록 관리를 실제 브라우저로 본다(임시 DB, 390/820/1440).
func Test95_195_BrowserRegistry(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	e, vision, _, _ := seed195(t)
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	shot, _ := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-195"))
	cmd := exec.Command("node", "95_195_registry_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(), "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1", "BASE_URL="+srv.URL,
		"AUTH_COOKIE="+vision.Value, "SCREENSHOT_DIR="+shot)
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

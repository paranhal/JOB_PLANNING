package handler

import (
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// 95-221: 정기점검 일일업무 편집 화면을 실제 브라우저(1440/820/390)로 확인한다.
func Test95_221_Browser(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	shot := os.Getenv("SCREENSHOT_DIR")
	if shot == "" {
		shot, _ = filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-221"))
	}
	for _, vp := range []string{"1440", "820", "390"} {
		v := setup221(t) // 화면 크기마다 새 데이터(담당 양기헌, 11/19)로 시작한다
		v.e.Static("/static", staticDir)
		srv := httptest.NewServer(v.e)
		cmd := exec.Command("node", "95_221_browser.mjs")
		cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
		cmd.Env = append(os.Environ(), "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1", "BASE_URL="+srv.URL, "AUTH_COOKIE="+v.oa.Value, "SCREENSHOT_DIR="+filepath.Join(shot, vp),
			"EDIT_PATH=/workboard/tasks/"+v.tid+"/edit", "NEW_DATE=2026-11-20", "ONLY_VP="+vp)
		out, err := cmd.CombinedOutput()
		srv.Close()
		t.Log(vp, string(out))
		if err != nil {
			so := string(out)
			if strings.Contains(so, "Executable doesn't exist") || strings.Contains(so, "browserType.launch") {
				t.Skip(so)
			}
			t.Fatalf("browser %s: %v", vp, err)
		}
	}
}

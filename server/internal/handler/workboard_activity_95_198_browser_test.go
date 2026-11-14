package handler

import (
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

// 95-198: 업무 등록·수정 화면의 활동 분류 칸을 실제 브라우저로 본다(임시 DB, 390/1440).
func Test95_198_BrowserActivityField(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	v := setup191(t)
	if _, _, err := v.wb.SetTaskActivity(model.OrgIDLibrary, v.progress, "service"); err != nil {
		t.Fatal(err)
	}
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	v.e.Static("/static", staticDir)
	srv := httptest.NewServer(v.e)
	t.Cleanup(srv.Close)
	shot, _ := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-198"))
	cmd := exec.Command("node", "95_198_activity_field_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(), "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1", "BASE_URL="+srv.URL,
		"AUTH_COOKIE="+jwtCookieVerified(t, v.admin).Value, "ASSIGNEE="+v.owner.FullName, "EDIT_ID="+v.progress, "SCREENSHOT_DIR="+shot)
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		s := string(out)
		if strings.Contains(s, "Executable doesn't exist") || strings.Contains(s, "browserType.launch") {
			t.Skip(s)
		}
		t.Fatalf("browser: %v", err)
	}
	for _, title := range []string{"브라우저 활동 등록 390", "브라우저 활동 등록 1440"} {
		id := taskIDByTitle198(t, v, title)
		if id == "" {
			t.Fatalf("%s 업무가 저장되지 않았다", title)
		}
		want := map[string]string{"브라우저 활동 등록 390": "service", "브라우저 활동 등록 1440": "mgmt"}[title]
		if got := v.wb.TaskActivityKind(id); got != want {
			t.Fatalf("%s 분류 %q (기대 %q)", title, got, want)
		}
	}
}

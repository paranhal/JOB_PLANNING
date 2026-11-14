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

// 95-194: 사업 연결 창을 실제 브라우저로 연다(임시 DB, 390/1440). 고른 대표 사업으로 연결이 저장되는지도 확인한다.
func Test95_194_BrowserModalDedupe(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	v := setup190(t, false)
	seed193Weekly(t, v)
	seed194Projects(t, v)
	v.e.POST("/stats/company-weekly/tasks/project", v.h.Stats.LinkWeeklyTasksProject, v.h.Auth.AuthMiddleware)
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	v.e.Static("/static", staticDir)
	srv := httptest.NewServer(v.e)
	t.Cleanup(srv.Close)
	shot, _ := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-194"))
	cmd := exec.Command("node", "95_194_modal_dedupe_browser.mjs")
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
	var pid string
	_ = v.h.Stats.repo.DB().QueryRow(`SELECT COALESCE(project_id,'') FROM work_tasks WHERE task_id='T193-M'`).Scan(&pid)
	if pid != "WPT-A2" {
		t.Fatalf("브라우저로 고른 대표 사업(WPT-A2)으로 연결되지 않았다: %q", pid)
	}
}

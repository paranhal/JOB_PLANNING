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

// 95-212·95-213: 마감 표시·안내 띠(서버 시각 기준 카운트다운)와 업무 상태별 줄을 실제 브라우저(1440/820/390)로 확인한다.
func Test95_212_213_Browser(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	v := setup205(t)
	for _, q := range []string{
		`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,work_date) VALUES ('Q1','admin','서버 점검','in_progress','WP-A','O03','2026-10-21'),('Q2','admin','납품 협의','cancelled','WP-A','O03','2026-10-21')`,
		`INSERT INTO work_activities(activity_id,task_id,activity_type,content,created_at) VALUES ('YA','Q1','other','현장 방문하여  로그 확인','2026-10-21 10:00:00'),('YB','Q2','other','고객 요청으로 중단','2026-10-22 10:00:00')`,
		`INSERT INTO work_actions(action_id,task_id,title,status,scheduled_date) VALUES ('WA','Q1','재방문','todo','2026-10-28')`,
	} {
		if _, err := v.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	oa := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	v.e.Static("/static", staticDir)
	srv := httptest.NewServer(v.e)
	t.Cleanup(srv.Close)
	shot := os.Getenv("SCREENSHOT_DIR")
	if shot == "" {
		shot, _ = filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-212-213"))
	}
	cmd := exec.Command("node", "95_212_213_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(), "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1", "BASE_URL="+srv.URL, "AUTH_COOKIE="+oa.Value,
		"SCREENSHOT_DIR="+shot, "PAGE_PATH_213="+w205+"&edit=0", "PAGE_PATH_212=/stats/company-weekly?org=O03&edit=0")
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

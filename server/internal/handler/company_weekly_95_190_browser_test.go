package handler

import (
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-190: 주간업무보고 편집 화면을 실제 브라우저로 누른다(임시 DB, 390/820/1440).
func Test95_190_BrowserWeeklyEdit(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	v := setup190(t, false)
	var keys []string
	for i := 1; i <= 6; i++ {
		k, err := v.h.Stats.repo.InsertWeeklyReportRow(repository.WeeklyReportRowInput{OrgID: "O03", DisplayName: "시험 사업 " + strconv.Itoa(i), Kind: "manual", SortOrder: i, Active: true})
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, k)
	}
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	v.e.Static("/static", staticDir)
	srv := httptest.NewServer(v.e)
	t.Cleanup(srv.Close)
	shot, _ := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-190"))
	cmd := exec.Command("node", "95_190_weekly_edit_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(), "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1", "BASE_URL="+srv.URL,
		"AUTH_COOKIE="+jwtCookieOrg(t, model.RoleOrgAdmin, "O03").Value, "ROW_KEYS="+strings.Join(keys, ","), "SCREENSHOT_DIR="+shot)
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

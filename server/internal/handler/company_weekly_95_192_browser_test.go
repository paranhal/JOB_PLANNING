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

// 95-192: 주간업무보고 팀 전체 금주·차주 입력을 실제 브라우저로 누른다(임시 DB, 390/820/1440).
func Test95_192_BrowserWeeklyTeam(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	v := setup190(t, false)
	for i := 1; i <= 3; i++ {
		if _, err := v.h.Stats.repo.InsertWeeklyReportRow(repository.WeeklyReportRowInput{OrgID: "O03", DisplayName: "시험 사업 " + strconv.Itoa(i), Kind: "manual", SortOrder: i, Active: true}); err != nil {
			t.Fatal(err)
		}
	}
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	v.e.Static("/static", staticDir)
	srv := httptest.NewServer(v.e)
	t.Cleanup(srv.Close)
	shot, _ := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-192"))
	cmd := exec.Command("node", "95_192_weekly_team_browser.mjs")
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

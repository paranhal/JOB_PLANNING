package handler

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

// 95-215: 도서관 일정표·칸반에 타 팀 담당자 이름이 나오지 않는지 실제 브라우저(1440/820/390)로 확인한다.
func Test95_215_AssigneeOrg_Browser(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	e, users, wb := newAssigneeOrgServer(t)
	if err := wb.CreateTask(&model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "타팀담당업무", Assignee: "SI담당C",
		OrgID: model.OrgIDLibrary, Status: model.WBTaskWaiting,
		WorkDate: "2026-10-19", DueDate: "2026-10-19", StartTime: "09:00", EndTime: "09:30", DurationMin: 30,
	}); err != nil {
		t.Fatal(err)
	}
	oa := mustUserByName(t, users, "oa01")
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	shot := os.Getenv("SCREENSHOT_DIR")
	if shot == "" {
		shot, _ = filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-215"))
	}
	pages, _ := json.Marshal(map[string]string{
		"register-day": "/workboard/register?view=day&date=2026-10-19",
	})
	cmd := exec.Command("node", "95_215_assignee_org_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(), "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1", "BASE_URL="+srv.URL, "AUTH_COOKIE="+jwtCookieUser(t, oa).Value,
		"SCREENSHOT_DIR="+shot, "PAGES="+string(pages), "FOREIGN_NAME=SI담당C", "CARD_TITLE=타팀담당업무")
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

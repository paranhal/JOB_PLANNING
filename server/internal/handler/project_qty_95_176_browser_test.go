package handler

import (
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

// 수량만 하부 업무: 실제 브라우저에서 목표 저장 → 처리 수량 입력 → N/M 표시 → 목표 초과 거절, 390/820/1440 폭.
func Test95_176_BrowserQtyOnly(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	e, repo, _, _ := newProjectServerWithDB(t)
	rec := doForm(t, e, "/projects", url.Values{
		"name": {"수량브라우저"}, "plan_year": {"2026"}, "status": {"active"}, "project_kind": {"build"},
		"start_date": {"2026-01-01"}, "end_date": {"2026-12-31"}, "color": {"#3B82F6"},
	})
	id := strings.TrimPrefix(strings.Split(rec.Header().Get("Location"), "?")[0], "/projects/")
	if err := repo.CreatePhase(&model.ProjectPhase{ProjectID: id, OrgID: model.OrgIDLibrary, Name: "전처리 업무", Status: model.PhaseActive}); err != nil {
		t.Fatal(err)
	}
	phases, _ := repo.ListPhases(id)
	if _, err := repo.CreatePackage(phases[0].PhaseID, "환경조사", model.TrackModePhase); err != nil {
		t.Fatal(err)
	}
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	shot, _ := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-176"))
	cmd := exec.Command("node", "95_176_qty_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(),
		"PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1",
		"BASE_URL="+srv.URL,
		"AUTH_COOKIE="+jwtCookie(t).Value,
		"PROJECT_ID="+id,
		"SCREENSHOT_DIR="+shot,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		s := string(out)
		if strings.Contains(s, "Executable doesn't exist") || strings.Contains(s, "browserType.launch") || (strings.Contains(s, "playwright") && strings.Contains(s, "install")) {
			t.Skip(s)
		}
		t.Fatalf("browser: %v\n%s", err, s)
	}
}

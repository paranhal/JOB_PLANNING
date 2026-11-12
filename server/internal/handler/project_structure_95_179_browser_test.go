package handler

import (
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-179 수행 구조: 대시보드 마감 임박 칩, 수정 화면 사이트 카드, 1·2단계 사이트 등록, 배지·변경내역 — 390/820/1440 폭.
func Test95_179_BrowserStructure(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	e, repo, wb, _ := newProjectServerWithDB(t)
	rec := doForm(t, e, "/projects", url.Values{
		"name": {"구조브라우저"}, "plan_year": {"2026"}, "status": {"active"}, "project_kind": {"build"},
		"start_date": {"2026-01-01"}, "end_date": {"2026-12-31"}, "color": {"#3B82F6"},
		"org_id": {model.OrgIDLibrary}, "is_paid": {"1"}, "contract_amount": {"1500000"},
	})
	id := strings.TrimPrefix(strings.Split(rec.Header().Get("Location"), "?")[0], "/projects/")
	for i, n := range []string{"홍성고등학교", "홍천초등학교", "횡성중학교"} {
		if _, err := wb.DB().Exec(`INSERT INTO project_sites (id, project_id, site_name, region, status, is_active, org_id) VALUES (?,?,?,?,'planned',1,?)`,
			fmt.Sprintf("PS-B%d", i), id, n, "강원", model.OrgIDLibrary); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.CreatePhase(&model.ProjectPhase{ProjectID: id, OrgID: model.OrgIDLibrary, Name: "전처리 업무", Status: model.PhaseActive, PlanEnd: "2020-01-05"}); err != nil {
		t.Fatal(err)
	}
	phases, _ := repo.ListPhases(id)
	pkgID, err := repo.CreatePackage(phases[0].PhaseID, "GBIC 수량 산정", model.TrackModeSite)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdatePackageMeta(id, pkgID, repository.PackageMeta{Badge: "사전GB"}, repository.DateChangeCtx{Actor: "t"}); err != nil {
		t.Fatal(err)
	}
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	shot, _ := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-179"))
	cmd := exec.Command("node", "95_179_structure_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(),
		"PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1",
		"BASE_URL="+srv.URL,
		"AUTH_COOKIE="+jwtCookie(t).Value,
		"PROJECT_ID="+id,
		"PHASE_ID="+phases[0].PhaseID,
		"PKG_ID="+pkgID,
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

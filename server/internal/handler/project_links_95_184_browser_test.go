package handler

import (
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

// 95-184: 링크를 버튼으로 + 「선택한 사이트 등록」 위치를 실제 클릭으로 확인한다(임시 DB, 390/820/1440).
func Test95_184_BrowserLinksAndAddSelected(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	v, ph2 := twoPhases185(t)
	// 이름 검색(「홍」)·지역 필터를 시험하도록 사이트를 더 만든다. 구축 단계(ph2)에는 아직 등록된 사이트가 없다.
	for i, n := range []string{"홍길동초", "홍천중", "횡성고"} {
		if _, err := v.wb.DB().Exec(`INSERT INTO project_sites (id, project_id, site_name, region, status, is_active, org_id) VALUES (?,?,?,?,'planned',1,?)`,
			fmt.Sprintf("PS-Y%d", i), v.id, n, "충남", model.OrgIDLibrary); err != nil {
			t.Fatal(err)
		}
	}
	// 2단계 하부 업무 사이트 등록 화면용: 1단계(전처리)에 이미 등록된 사이트 4곳이 후보가 되는 새 하부 업무
	newPkg, err := v.repo.CreatePackage(v.phase, "새 하부 업무", model.TrackModeSite)
	if err != nil {
		t.Fatal(err)
	}
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	v.e.Static("/static", staticDir)
	srv := httptest.NewServer(v.e)
	t.Cleanup(srv.Close)
	shot, _ := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-184"))
	cmd := exec.Command("node", "95_184_links_buttons_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(),
		"PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1",
		"BASE_URL="+srv.URL,
		"AUTH_COOKIE="+v.admin.Value,
		"PROJECT_ID="+v.id, "PHASE2_ID="+ph2, "NEW_PKG_ID="+newPkg, "PLAN_ID="+v.plan,
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
	t.Log(string(out))
}

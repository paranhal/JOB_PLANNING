package handler

import (
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/repository"
)

// 작업·행정/지원 등록: 실제 브라우저에서 사업 선택 → 하부 업무(필수) → 그 하부 업무 사이트만 유형·시도·시군·검색으로 골라 담기. 95-182.
func Test95_182_BrowserPackagePick(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	e, db, me, _, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "p182br.db"))
	for _, q := range []string{
		`INSERT INTO work_projects (project_id, name, status, org_id, is_test, is_paid) VALUES ('WP-O01','도서관사업','active','O01',0,1)`,
		`INSERT INTO work_projects (project_id, name, status, org_id, is_test, is_paid) VALUES ('WP-O02','구조없는사업','active','O01',0,1)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	cids := seedBadgeData(t, db, "WP-O01", "O01")
	for i, sg := range []string{"홍성군", "천안시"} {
		if _, err := db.Exec(`UPDATE customers SET industry='학교', addr_sido='충청남도', addr_sigungu=? WHERE customer_id=?`, sg, cids[i]); err != nil {
			t.Fatal(err)
		}
	}
	_ = repository.NewProjectRepo(db)
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	shot, _ := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-182"))
	cmd := exec.Command("node", "95_182_package_pick_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(),
		"PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1",
		"BASE_URL="+srv.URL,
		"AUTH_COOKIE="+jwtCookieVerified(t, me).Value,
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

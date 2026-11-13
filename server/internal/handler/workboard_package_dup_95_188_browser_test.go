package handler

import (
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-188: 작업 등록 모달(project_work)에서 수행 단계 패널과 하부 업무 선택이 package_id 를 둘 다 보내
// 빈 값이 먼저 읽혀 「하부 업무를 선택하세요」 오류가 나던 문제. MODE=assert 로 수정 후 기대값을 확인한다.
func Test95_188_BrowserPackageIDSingle(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	e, db, me, _, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "p188.db"))
	for _, q := range []string{
		`INSERT INTO work_projects (project_id, name, status, org_id, is_test, is_paid) VALUES ('WP-O01','도서관사업','active','O01',0,1)`,
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
	// 수행 단계 패널(plan_pick)이 나오도록 계획을 하나 둔다(실제 운영 사업처럼).
	pr := repository.NewProjectRepo(db)
	choices, err := pr.PackageChoices("WP-O01")
	if err != nil || len(choices) == 0 {
		t.Fatalf("하부 업무 없음 %v", err)
	}
	if _, err := pr.CreatePlan("WP-O01", repository.PlanDraft{PackageID: choices[0].ID, Name: "1차 계획", PlanStart: "2027-04-01", PlanEnd: "2027-05-01", Basis: model.BasisSite, Actor: "t"}); err != nil {
		t.Fatal(err)
	}
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	mode := os.Getenv("MODE")
	if mode == "" {
		mode = "assert"
	}
	cmd := exec.Command("node", "95_188_package_dup_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(), "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1", "BASE_URL="+srv.URL,
		"AUTH_COOKIE="+jwtCookieVerified(t, me).Value, "MODE="+mode)
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

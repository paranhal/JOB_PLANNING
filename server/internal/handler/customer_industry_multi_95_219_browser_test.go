package handler

import (
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// 95-219: 업종 다중 선택 컴포넌트를 실제 브라우저(1440/820/390)로 확인한다.
func Test95_219_IndustryMulti_Browser(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	e, db, _, ck := newCustomerShare2EHTTP(t)
	h2 := New(db)
	e.GET("/customers/:id/edit", h2.Customer.Edit, h2.Auth.AuthMiddleware)
	_, _ = db.Exec(`UPDATE users SET profile_done=1, mobile='010-0000-0000', email='t@example.test'`) // 시험 계정이 「정보 보완」 화면으로 넘어가지 않게
	if _, err := db.Exec(`INSERT INTO customers(customer_id,org_name,official_name,is_active,org_id,party_kind,industry) VALUES('CUST-MULTI','시험다중고객','시험다중고객',1,'O01','customer','도서관,학교')`); err != nil {
		t.Fatal(err)
	}
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	shot := os.Getenv("SCREENSHOT_DIR")
	if shot == "" {
		shot, _ = filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-219-industry"))
	}
	cmd := exec.Command("node", "95_219_industry_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(), "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1", "BASE_URL="+srv.URL, "AUTH_COOKIE="+ck.oa01.Value, "SCREENSHOT_DIR="+shot, "SEED_ID=CUST-MULTI")
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

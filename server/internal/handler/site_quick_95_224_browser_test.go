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

// 95-224 §2: 업무 등록 중 사이트 간이 팝업을 실제 브라우저로 본다(열기→취소→등록→복귀, 390/1440).
func Test95_224_BrowserSiteQuick(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	e, db, _, _, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "q224.db"))
	admin, err := repository.NewUserRepo(db).GetByUsername("oa01")
	if err != nil || admin == nil {
		t.Fatal("oa01")
	}
	for _, q := range []string{
		`INSERT INTO customers(customer_id,org_name,official_name,industry,party_kind,is_active) VALUES ('C224A','A교육청','A교육청','교육청','customer',1),('C224B','B교육청','B교육청','교육청','customer',1)`,
		`INSERT INTO customers(customer_id,org_name,official_name,industry,parent_customer_id,party_kind,is_active,has_parent,addr_sido) VALUES
			('C224A1','A가도서관','A가도서관','도서관','C224A','customer',1,1,'충청남도'),('C224A2','A나도서관','A나도서관','도서관','C224A','customer',1,1,'충청남도'),
			('C224B1','B가도서관','B가도서관','도서관','C224B','customer',1,1,'충청남도'),('C224B2','B나도서관','B나도서관','도서관','C224B','customer',1,1,'충청남도')`,
		`INSERT INTO work_projects(project_id,name,status,org_id,customer_id) VALUES ('P224A','간이팝업 사업A','active','O01','C224A'),('P224B','간이팝업 사업B','active','O01','C224B')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/projects/:id/sites/quick", h.Project.SitesQuick)
	g.POST("/projects/:id/sites/add-schools", h.Project.SitesAddSchools)
	g.POST("/projects/:id/sites/unlock", h.Project.SiteAddUnlock)
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	shot, _ := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-224"))
	cmd := exec.Command("node", "95_224_site_quick_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(), "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1", "BASE_URL="+srv.URL,
		"AUTH_COOKIE="+jwtCookieVerified(t, admin).Value, "SCREENSHOT_DIR="+shot)
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		s := string(out)
		if strings.Contains(s, "Executable doesn't exist") || strings.Contains(s, "browserType.launch") {
			t.Skip(s)
		}
		t.Fatalf("browser: %v", err)
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM project_sites WHERE project_id IN ('P224A','P224B') AND is_active=1`).Scan(&n)
	if n != 2 {
		t.Fatalf("등록된 사이트 %d곳 want 2(사업별 1곳, 취소분은 저장 안 됨)", n)
	}
}

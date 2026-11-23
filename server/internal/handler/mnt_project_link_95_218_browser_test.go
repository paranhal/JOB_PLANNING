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

// 95-218: 조치 완료 화면의 사업 후보 안내와 주간 보고서 저장 반복을 실제 브라우저(1440/820/390)로 확인한다.
func Test95_218_Browser(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	v := setup218(t)
	ex := func(q string, a ...interface{}) {
		if _, err := v.db.Exec(q, a...); err != nil {
			t.Fatal(err)
		}
	}
	v.e.GET("/maintenance/visits/:visit_id/action", v.h.Maintenance.VisitAction, v.h.Auth.AuthMiddleware)
	ex(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id) VALUES ('c1','고운남측도서관','고운남측도서관',1,'O03'),('c2','한솔도서관','한솔도서관',1,'O03'),('c3','태안도서관','태안도서관',1,'O03')`)
	ex(`INSERT INTO work_projects(project_id,name,status,org_id,project_kind,is_test) VALUES ('WP-M','정기점검 사업 A','active','O03','project',0),('WP-M2','정기점검 사업 B','active','O03','project',0)`)
	ex(`INSERT INTO project_sites(id,project_id,customer_id,site_name,status,sort_order,is_active,org_id) VALUES ('S1','WP-M','c1','고운남측도서관','planned',1,1,'O03'),('S2','WP-M','c2','한솔도서관','planned',2,1,'O03'),('S3','WP-M2','c2','한솔도서관','planned',3,1,'O03')`)
	ex(`INSERT INTO maintenance_plans(plan_id,plan_year,title,status,org_id) VALUES ('MPX',2026,'p','approved','O03')`)
	for i, c := range []string{"c1", "c2", "c3"} {
		ex(`INSERT INTO maintenance_visits(visit_id,plan_id,visit_date,customer_id,sort_order,entry_category,assignee,product_type,completed) VALUES (?,'MPX',?,?,0,'normal','양기헌','KLAS',0)`,
			"V"+string(rune('1'+i)), "2026-10-2"+string(rune('1'+i)), c)
	}
	oa := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	v.e.Static("/static", staticDir)
	srv := httptest.NewServer(v.e)
	t.Cleanup(srv.Close)
	shot := os.Getenv("SCREENSHOT_DIR")
	if shot == "" {
		shot, _ = filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-218"))
	}
	checks, _ := json.Marshal([]map[string]interface{}{
		{"name": "complete-1cand", "path": "/maintenance/visits/V1/action", "absent": []string{"[data-visit-project-none]", "[data-visit-project-many]"}},
		{"name": "complete-2cand", "path": "/maintenance/visits/V2/action", "present": []string{"[data-visit-project-many]"}, "text": []string{"2개"}},
		{"name": "complete-0cand", "path": "/maintenance/visits/V3/action", "present": []string{"[data-visit-project-none]"}, "text": []string{"사업 미연결"}},
	})
	cmd := exec.Command("node", "95_218_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(), "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1", "BASE_URL="+srv.URL, "AUTH_COOKIE="+oa.Value, "SCREENSHOT_DIR="+shot,
		"CHECKS="+string(checks), "WEEKLY_PATH="+w205+"&edit=1")
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

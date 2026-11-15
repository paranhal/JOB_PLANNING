package handler

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// 95-201: 조치 화면 영향 장비를 실제 브라우저로 본다(임시 DB, 390/820/1440).
func Test95_201_BrowserAffectedAssets(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	v := setup201(t)
	// 이 시험에서는 /api/assets/:customer_id (장비 검색)와 정적 파일이 필요하다
	g := v.e.Group("")
	g.Use(v.h.Auth.AuthMiddleware)
	g.GET("/api/assets/:customer_id", v.h.Asset.APIAssetsByCustomer)
	g.POST("/as/:id/action-sets", v.h.AS.SaveActionSet)
	cases := map[string]map[string]string{}
	for _, vp := range []string{"390", "820", "1440"} {
		id := "R201-" + vp
		insert151AS(t, v.db, id, "C151", "in_progress", "", "", false)
		if _, err := v.db.Exec(`UPDATE as_receipts SET asset_id='A201-MAIN' WHERE as_id=?`, id); err != nil {
			t.Fatal(err)
		}
		cases[vp] = map[string]string{"id": id, "show": asLegacyHref(id), "asNumber": id}
	}
	raw, _ := json.Marshal(cases)
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	v.e.Static("/static", staticDir)
	srv := httptest.NewServer(v.e)
	t.Cleanup(srv.Close)
	shot, _ := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-201"))
	cmd := exec.Command("node", "95_201_affected_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(), "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1", "BASE_URL="+srv.URL,
		"AUTH_COOKIE="+v.oa.Value, "CASES="+string(raw), "SCREENSHOT_DIR="+shot)
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		s := string(out)
		if strings.Contains(s, "Executable doesn't exist") || strings.Contains(s, "browserType.launch") {
			t.Skip(s)
		}
		t.Fatalf("browser: %v", err)
	}
	for _, vp := range []string{"390", "820", "1440"} {
		if got := v.affected(t, "R201-"+vp); len(got) != 3 {
			t.Fatalf("%s 저장된 영향 장비 %v", vp, got)
		}
	}
}

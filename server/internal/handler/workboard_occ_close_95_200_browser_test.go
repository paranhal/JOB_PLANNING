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

// 95-200: 「업무 모두 종료」를 실제 브라우저로 본다(임시 DB, 390/820/1440).
func Test95_200_BrowserCloseAll(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	v := setup191(t)
	cases := map[string]map[string][2]string{}
	for _, vp := range []string{"390", "820", "1440"} {
		a, b := "PBA"+vp, "PBB"+vp
		aIDs := mkRecurring(t, v, a, "manual", fifteen())         // 3번째에서 종료(이전 2건·이후 12건)
		bIDs := mkRecurring(t, v, b, "require_result", fifteen()) // 3번째에서 종료(「함께 제외」 해제)
		cases[vp] = map[string][2]string{"a": {a, aIDs[2]}, "b": {b, bIDs[2]}}
	}
	raw, _ := json.Marshal(cases)
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	v.e.Static("/static", staticDir)
	srv := httptest.NewServer(v.e)
	t.Cleanup(srv.Close)
	shot, _ := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-200"))
	cmd := exec.Command("node", "95_200_close_all_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(), "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1", "BASE_URL="+srv.URL,
		"AUTH_COOKIE="+jwtCookieVerified(t, v.admin).Value, "CASES="+string(raw), "SCREENSHOT_DIR="+shot)
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		s := string(out)
		if strings.Contains(s, "Executable doesn't exist") || strings.Contains(s, "browserType.launch") {
			t.Skip(s)
		}
		t.Fatalf("browser: %v", err)
	}
	// 화면 조작의 결과가 DB 에 그대로 남았다
	db := v.wb.DB()
	for _, vp := range []string{"390", "820", "1440"} {
		if s := stateOf(t, db, "PBA"+vp); s.Status != "complete" {
			t.Fatalf("%s A 상위 %+v", vp, s)
		}
		if s := stateOf(t, db, "PBB"+vp); s.Status == "complete" {
			t.Fatalf("%s B 상위가 완료됐다(이전 미완료가 남아야 한다)", vp)
		}
		if n, _ := v.wb.CountOpenOccurrences("PBB" + vp); n != 2 {
			t.Fatalf("%s B 열린 회차 %d(이전 2건만 남아야 한다)", vp, n)
		}
	}
}

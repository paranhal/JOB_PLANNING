package handler

import (
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"customer-support/internal/model"
)

// 95-209: 주간 업무 보고서 상단 링크 → 주 버튼, 이 화면의 버튼 크기 통일(다른 화면은 불변).

func TestHTTP95_209_TopLinksAreButtons(t *testing.T) {
	v := setup205(t)
	oa := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	body := v.do(t, "GET", w205, nil, oa).Body.String()
	for _, c := range []struct{ href, text string }{
		{`/stats/reports#outputs`, "다른 출력물"},
		{`/stats/reports`, "예전 화면으로"},
	} {
		re := regexp.MustCompile(`<a class="([^"]*)" href="` + regexp.QuoteMeta(c.href) + `">` + c.text + `</a>`)
		m := re.FindStringSubmatch(body)
		if m == nil {
			t.Fatalf("%s 링크 없음(href 불변)", c.text)
		}
		if !strings.Contains(m[1], "btn-primary") || !strings.Contains(m[1], "h-11 px-4") || strings.Contains(m[1], "underline") {
			t.Errorf("%s 클래스 %q", c.text, m[1])
		}
	}
	if !strings.Contains(body, `weekly-uniform`) {
		t.Error("화면 범위 클래스(weekly-uniform)가 없다")
	}
	// 정기·이동 줄의 ‹ › 는 정사각 44px
	if strings.Count(body, `class="w-11 h-11 grid place-items-center`) != 2 || strings.Contains(body, `w-9 h-9`) {
		t.Error("‹ › 크기")
	}
}

// 범위 제한: 같은 파셜(weekly_unassigned)을 쓰는 다른 화면(/stats/reports)에는 .weekly-uniform 이 없다.
func TestHTTP95_209_ScopeLimitedToWeeklyScreen(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("web", "css", "input.css"))
	if err != nil {
		t.Skip("input.css 없음")
	}
	css := string(src)
	i := strings.Index(css, ".weekly-uniform .btn,")
	if i < 0 {
		t.Fatal("범위 규칙 없음")
	}
	block := css[i : i+420]
	for _, line := range strings.Split(block, "\n") {
		l := strings.TrimSpace(line)
		if strings.HasPrefix(l, ".") && !strings.HasPrefix(l, ".weekly-uniform") {
			t.Errorf("범위 밖 선택자: %s", l)
		}
		if strings.Contains(l, "{") {
			break
		}
	}
}

// 실제 브라우저: 버튼 높이를 읽어 단언(1440/820/390), 다른 화면 버튼 높이는 그대로.
func Test95_209_BrowserButtonHeights(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	v := setup205(t)
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	v.e.Static("/static", staticDir)
	srv := httptest.NewServer(v.e)
	t.Cleanup(srv.Close)
	shot, _ := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-209"))
	cmd := exec.Command("node", "95_209_weekly_buttons_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(), "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1", "BASE_URL="+srv.URL,
		"AUTH_COOKIE="+jwtCookieOrg(t, model.RoleOrgAdmin, "O03").Value, "SCREENSHOT_DIR="+shot)
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

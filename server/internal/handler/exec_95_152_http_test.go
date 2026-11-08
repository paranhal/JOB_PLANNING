package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func Test95_152_T1_CardHTMLAndBoardHref(t *testing.T) {
	e, u, date := seed95_90(t)
	body := doGetWith(t, e, "/exec?date="+date, jwtCookieUser(t, u.ceo)).Body.String()
	if strings.Contains(body, ">\n     class=\"card") || strings.Contains(body, "> class=\"card flex") {
		t.Fatal("class가 본문 글자로 노출")
	}
	re := regexp.MustCompile(`<a[^>]*data-exec-org="O01"[^>]*>`)
	tag := re.FindString(body)
	if tag == "" {
		t.Fatal("O01 카드 a 없음")
	}
	if !strings.Contains(tag, `class="`) || !strings.Contains(tag, "card") {
		t.Fatalf("class 속성 없음 %s", tag)
	}
	if !strings.Contains(tag, "/meeting/board?") || !strings.Contains(body, "src=exec") || !strings.Contains(body, "view_org_id=O01") {
		t.Fatalf("보드 링크 없음 %s", tag)
	}
}

func Test95_152_T2_BoardBackToExec(t *testing.T) {
	e, u, date := seed95_90(t)
	ck := jwtCookieUser(t, u.ceo)
	q := "view_org_id=O02&view=people&from=" + date + "&to=" + date + "&src=exec&exec_view=progress&exec_org=O02&exec_date=" + date
	rec := doGetWith(t, e, "/meeting/board?"+q, ck)
	if rec.Code != http.StatusOK {
		t.Fatalf("board %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="page-back"`) {
		t.Fatal("이전 없음")
	}
	if !strings.Contains(body, `href="/exec?`) || !strings.Contains(body, "view=progress") || !strings.Contains(body, "focus=O02") {
		t.Fatalf("이전 href %s", extractPageBack(body))
	}
	if strings.Contains(extractPageBack(body), `data-page-back="history"`) {
		t.Fatal("Referer 없이 history")
	}
	plain := doGetWith(t, e, "/meeting/board?from="+date+"&to="+date+"&view_org_id=O02", ck)
	pb := extractPageBack(plain.Body.String())
	if strings.Contains(pb, "focus=O02") && strings.Contains(pb, "src=exec") {
		t.Fatal("src 없이 exec 복귀")
	}
}

func Test95_152_T3_ViewTabsKeepSrc(t *testing.T) {
	e, u, date := seed95_90(t)
	q := "view_org_id=O02&view=people&from=" + date + "&to=" + date + "&src=exec&exec_view=progress&exec_org=O02&exec_date=" + date
	body := doGetWith(t, e, "/meeting/board?"+q, jwtCookieUser(t, u.ceo)).Body.String()
	for _, tab := range []string{`data-view-tab="project"`, `data-view-tab="weekly"`, `data-view-tab="people"`} {
		href := hrefForAttr(body, tab)
		if !strings.Contains(href, "src=exec") || !strings.Contains(href, "exec_org=O02") || !strings.Contains(href, "exec_view=progress") {
			t.Fatalf("%s 유지 실패 %s", tab, href)
		}
	}
}

func Test95_152_T4_FocusToggleAndSalesStub(t *testing.T) {
	e, u, date := seed95_90(t)
	ck := jwtCookieUser(t, u.ceo)
	body := doGetWith(t, e, "/exec?date="+date+"&focus=O02", ck).Body.String()
	if !strings.Contains(body, `data-exec-focus="O02"`) {
		t.Fatal("포커스 표시 없음")
	}
	if !strings.Contains(body, `data-exec-view-btn="progress"`) || !strings.Contains(body, `data-exec-view-btn="sales"`) {
		t.Fatal("토글 없음")
	}
	if !strings.Contains(body, `aria-pressed="true"`) || !strings.Contains(body, `aria-pressed="false"`) {
		t.Fatal("aria-pressed")
	}
	sales := doGetWith(t, e, "/exec?view=sales&date="+date, ck).Body.String()
	if !strings.Contains(sales, "매출 현황은 준비 중입니다") {
		t.Fatal("준비 중 문구")
	}
	if strings.Contains(sales, `data-exec-card`) {
		t.Fatal("매출에 카드")
	}
	bad := doGetWith(t, e, "/exec?view=nope&date="+date, ck).Body.String()
	if !strings.Contains(bad, `data-exec-view-btn="progress"`) || !strings.Contains(bad, `aria-pressed="true"`) {
		t.Fatal("잘못된 view")
	}
	if !strings.Contains(bad, `data-exec-card`) {
		t.Fatal("기본 progress 카드 없음")
	}
}

func Test95_152_T6_BrowserFocus(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	e, u, date := seed95_90(t)
	ck := jwtCookieUser(t, u.ceo)
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	shot := filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰")
	shot, _ = filepath.Abs(shot)
	cmd := exec.Command("node", "95_152_exec_focus_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(),
		"PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1",
		"BASE_URL="+srv.URL,
		"AUTH_COOKIE="+ck.Value,
		"WORK_DATE="+date,
		"ORG_ID=O03",
		"SCREENSHOT_DIR="+shot,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		s := string(out)
		if strings.Contains(s, "Executable doesn't exist") || strings.Contains(s, "browserType.launch") {
			t.Skip(s)
		}
		t.Fatalf("브라우저: %v\n%s", err, out)
	}
}

func extractPageBack(body string) string {
	i := strings.Index(body, `id="page-back"`)
	if i < 0 {
		return ""
	}
	start := i - 80
	if start < 0 {
		start = 0
	}
	end := i + 220
	if end > len(body) {
		end = len(body)
	}
	return body[start:end]
}

func hrefForAttr(body, attr string) string {
	i := strings.Index(body, attr)
	if i < 0 {
		return ""
	}
	start := strings.LastIndex(body[:i], "<a")
	if start < 0 {
		return ""
	}
	end := strings.Index(body[start:], ">")
	if end < 0 {
		return ""
	}
	tag := body[start : start+end]
	m := regexp.MustCompile(`href="([^"]+)"`).FindStringSubmatch(tag)
	if len(m) < 2 {
		return tag
	}
	return m[1]
}

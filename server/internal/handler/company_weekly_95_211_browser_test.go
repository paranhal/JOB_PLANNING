package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

// 95-211: 「팀 전체」 박스를 실제 브라우저(1440/820/390)로 확인한다. SHOT_TAG=before 면(수정 전 트리) 재현 스크린샷만 만든다.
func Test95_211_BrowserTeamBox(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	type sc struct {
		name, path, expect string
		cards, confirm     bool
		readonly           bool
	}
	for _, s := range []sc{
		{name: "A-confirmed-view-empty", path: w211 + "&edit=0", expect: "read", cards: true, confirm: true},
		{name: "B-edit-zero-cards", path: w211 + "&edit=1", expect: "edit"},
		{name: "C-readonly-view-empty", path: w211 + "&edit=0", expect: "read", cards: true, readonly: true},
	} {
		t.Run(s.name, func(t *testing.T) {
			v := setup190(t, s.cards)
			oa := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
			// 보고서를 만들어 둔다(팀 전체는 비어 있음)
			if r := v.do(t, "POST", "/stats/company-weekly/save", url.Values{"week": {"2026-10-19"}, "org": {"O03"}, "touch_team": {"1"}}, oa); r.Code != http.StatusSeeOther {
				t.Fatalf("저장 %d", r.Code)
			}
			if s.confirm {
				if r := v.do(t, "POST", "/stats/company-weekly/ui-confirm", url.Values{"week": {"2026-10-19"}, "org": {"O03"}}, oa); r.Code != http.StatusSeeOther {
					t.Fatalf("확정 %d", r.Code)
				}
			}
			ck := oa
			if s.readonly {
				ck = jwtCookieRole(t, model.RoleCLevel)
			}
			staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
			if err != nil {
				t.Fatal(err)
			}
			v.e.Static("/static", staticDir)
			srv := httptest.NewServer(v.e)
			t.Cleanup(srv.Close)
			shot := os.Getenv("SCREENSHOT_DIR")
			if shot == "" {
				shot, _ = filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-211"))
			}
			tag := os.Getenv("SHOT_TAG")
			if tag == "" {
				tag = "after"
			}
			cmd := exec.Command("node", "95_211_team_box_browser.mjs")
			cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
			cmd.Env = append(os.Environ(), "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1", "BASE_URL="+srv.URL, "AUTH_COOKIE="+ck.Value,
				"SCREENSHOT_DIR="+shot, "SHOT_TAG="+tag, "NAME="+s.name, "PAGE_PATH="+s.path, "EXPECT="+s.expect)
			out, err := cmd.CombinedOutput()
			t.Log(string(out))
			if err != nil {
				so := string(out)
				if strings.Contains(so, "Executable doesn't exist") || strings.Contains(so, "browserType.launch") {
					t.Skip(so)
				}
				t.Fatalf("browser: %v", err)
			}
		})
	}
}

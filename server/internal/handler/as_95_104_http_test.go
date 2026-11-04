package handler

import (
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func Test95_104_T8_ActionHTMLOneKeywordBinder(t *testing.T) {
	root := findTemplateRoot(t)
	src := string(mustRead(t, filepath.Join(root, "as/action.html")))
	if strings.Contains(src, "(function bindKw(){") {
		t.Fatal("조치 화면에 옛 bindKw 가 남아 있다")
	}
	if strings.Contains(src, "bindKw95_85") {
		t.Fatal("조치 화면에 bindKw95_85 인라인이 남아 있다")
	}
	if strings.Count(src, "/static/js/as_keywords.js") != 1 {
		t.Fatal("as_keywords.js 가 한 번이 아니다")
	}
	e, _, _, _, asID := newASActionFixture(t)
	body := getASActionPage(t, e, asID)
	if strings.Contains(body, "(function bindKw(){") && strings.Contains(body, "bindKw95_85") {
		t.Fatal("렌더 HTML 에서 두 스크립트가 같은 목록을 갱신한다")
	}
	if strings.Contains(body, "bindKw95_85") {
		t.Fatal("렌더에 bindKw95_85 가 있다")
	}
	if !strings.Contains(body, "/static/js/as_keywords.js") {
		t.Fatal("as_keywords.js 미포함")
	}
	form := string(mustRead(t, filepath.Join(root, "as/form.html")))
	if strings.Contains(form, "bindKw95_85") {
		t.Fatal("접수 폼에 95-85 바인더가 겹친다")
	}
}

func Test95_104_T9_KeywordRoundtrip(t *testing.T) {
	e, h, _, _, asID := newASActionFixture(t)
	all, err := h.AS.kwRepo.List(true)
	if err != nil || len(all) == 0 {
		t.Fatalf("사전 시드 없음 %v", err)
	}
	k := all[0]
	if err := h.AS.kwRepo.ReplaceLinks(asID, model.KWFieldAction, []string{k.KeywordID}, []string{k.KeywordID}); err != nil {
		t.Fatal(err)
	}
	body := getASActionPage(t, e, asID)
	if !strings.Contains(body, k.KeywordID) || !strings.Contains(body, "checked") {
		t.Fatal("연결 키워드 체크 마크업 없음")
	}
	if !strings.Contains(body, `name="kw_`+model.KWFieldAction+`"`) || !strings.Contains(body, `name="kw_manual"`) {
		t.Fatal("저장 필드명 변경됨")
	}
	form := url.Values{
		"s1_work_place": {"office"}, "s1_process_type": {"remote"},
		"s1_action_taken": {"키워드 왕복 저장 내용입니다 충분히"},
		"s1_result_code": {model.ResultDone},
		"s1_cause_cat1": {"server"}, "s1_cause_cat2": {"server.sw"},
		"work_place": {"office"}, "process_type": {"remote"}, "cause_type": {"sw"},
		"action_taken": {"키워드 왕복 저장 내용입니다 충분히"},
		"result_code": {model.ResultDone},
		"kw_" + model.KWFieldAction: {k.KeywordID},
		"kw_suggested_" + model.KWFieldAction: {k.KeywordID},
		"kw_manual": {"DB(대상)"},
	}
	rec := postASAction(t, e, asID, form)
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("저장 %d %s", rec.Code, rec.Header().Get("Location")+rec.Body.String())
	}
	links, err := h.AS.kwRepo.LinksByAS(asID)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, l := range links {
		if l.Field == model.KWFieldAction {
			got[l.KeywordID] = true
		}
	}
	if !got[k.KeywordID] {
		t.Fatalf("기존 체크 키워드가 저장되지 않음 %+v", links)
	}
	foundDB := false
	all, _ = h.AS.kwRepo.List(true)
	for _, it := range all {
		if it.Keyword == "DB" && it.Group == model.KWGroupTarget && got[it.KeywordID] {
			foundDB = true
		}
	}
	if !foundDB {
		t.Fatalf("직접 입력 DB 가 연결되지 않음 %+v", links)
	}
	again := getASActionPage(t, e, asID)
	if !strings.Contains(again, k.KeywordID) || !strings.Contains(again, "checked") {
		t.Fatal("재조회 시 체크가 없다")
	}
}

func Test95_104_JSUnit(t *testing.T) {
	cmd := exec.Command("node", "as_keywords_test.js")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("js: %v\n%s", err, out)
	}
}

func Test95_104_T10toT12_Browser(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	cmd := exec.Command("node", "95_104_as_keywords_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(), "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := string(out)
		if strings.Contains(msg, "Executable doesn't exist") || (strings.Contains(msg, "playwright") && strings.Contains(msg, "install")) {
			t.Skip(msg)
		}
		t.Fatalf("browser: %v\n%s", err, out)
	}
}

func Test95_104_T13_RelatedKeywordTestsStillPass(t *testing.T) {
	e, _, _, _, asID := newASActionFixture(t)
	body := getASActionPage(t, e, asID)
	if !strings.Contains(body, `name="kw_manual"`) {
		t.Fatal("95-85 키워드 직접입력 칸 없음")
	}
	bad := postASAction(t, e, asID, url.Values{
		"work_place": {"office"}, "process_type": {"remote"}, "cause_type": {"hw"},
		"action_taken": {"키워드 오류 저장 내용입니다"}, "result_code": {model.ResultDone},
		"kw_manual": {"나쁜(없음)"},
	})
	if bad.Code != http.StatusSeeOther || !strings.Contains(bad.Header().Get("Location"), "keyword") {
		t.Fatalf("95-85 수동 키워드 거절 %d %s", bad.Code, bad.Header().Get("Location"))
	}
}

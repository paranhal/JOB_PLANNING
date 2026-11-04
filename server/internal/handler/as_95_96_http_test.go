package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"golang.org/x/net/html"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func as96LegacyForm() url.Values {
	return url.Values{
		"work_place":      {"office"},
		"process_type":    {"remote"},
		"cause_cat1":      {"server"},
		"cause_cat2":      {"server.hw"},
		"action_taken":    {"서버 전원 점검 후 재기동했습니다"},
		"result_code":     {model.ResultDone},
		"action_short_ok": {"1"},
	}
}

func as96SetForm() url.Values {
	return url.Values{
		"s1_work_place":   {"office"},
		"s1_process_type": {"remote"},
		"s1_cause_cat1":   {"server"},
		"s1_cause_cat2":   {"server.hw"},
		"s1_action_taken": {"서버 전원 점검 후 재기동했습니다"},
		"s1_result_code":  {model.ResultDone},
		"s1_leave_same":   {"1"},
		"cause_cat1":      {""},
		"cause_cat2":      {""},
		"cause_cat3":      {""},
		"work_place":      {"office"},
		"process_type":    {"remote"},
		"action_taken":    {""},
		"result_code":     {""},
		"action_short_ok": {"1"},
	}
}

func Test95_96_T0_LegacyAndSetEquivalent(t *testing.T) {
	snap := func(form url.Values) (cat1, cat2, cat3, taken, result, status string, hist int) {
		t.Helper()
		e, h, asRepo, _, asID := newASActionFixture(t)
		rec := postASAction(t, e, asID, form)
		if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
			t.Fatalf("저장 실패 %d %s", rec.Code, rec.Header().Get("Location"))
		}
		got, err := asRepo.GetByID(repository.OrgAll, asID)
		if err != nil || got == nil {
			t.Fatal(err)
		}
		n := 0
		if h.AS.processRepo != nil {
			ps, _ := h.AS.processRepo.ListByAS(asID)
			n = len(ps)
		}
		return got.CauseCat1, got.CauseCat2, got.CauseCat3, got.ActionTaken, got.ResultCode, got.Status, n
	}
	a1, a2, a3, at, ar, as, ah := snap(as96LegacyForm())
	b1, b2, b3, bt, br, bs, bh := snap(as96SetForm())
	if a1 != b1 || a2 != b2 || a3 != b3 || at != bt || ar != br || as != bs || ah != bh {
		t.Fatalf("동등성 실패 legacy=%s/%s/%s %s %s %s hist=%d set=%s/%s/%s %s %s %s hist=%d",
			a1, a2, a3, at, ar, as, ah, b1, b2, b3, bt, br, bs, bh)
	}
}

func Test95_96_T1_SetCatsSave(t *testing.T) {
	e, _, asRepo, _, asID := newASActionFixture(t)
	rec := postASAction(t, e, asID, as96SetForm())
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("T1 %d %s", rec.Code, rec.Header().Get("Location"))
	}
	got, _ := asRepo.GetByID(repository.OrgAll, asID)
	if got.CauseCat1 != "server" || got.CauseCat2 != "server.hw" {
		t.Fatalf("cats %s/%s", got.CauseCat1, got.CauseCat2)
	}
}

func Test95_96_T2_CustomCat(t *testing.T) {
	e, _, asRepo, _, asID := newASActionFixture(t)
	form := as96SetForm()
	form.Set("s1_cause_cat1", "custom")
	form.Set("s1_cause_cat1_custom", "CCTV특수")
	form.Set("s1_cause_cat2", "should-clear")
	form.Set("s1_cause_cat3", "x")
	rec := postASAction(t, e, asID, form)
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("T2 %s", rec.Header().Get("Location"))
	}
	got, _ := asRepo.GetByID(repository.OrgAll, asID)
	if got.CauseCat1 != model.CauseCatCustom || got.CauseCat1Custom != "CCTV특수" || got.CauseCat2 != "" || got.CauseCat3 != "" {
		t.Fatalf("%+v", got)
	}
}

func Test95_96_T3_CustomTextInfersCustom(t *testing.T) {
	e, _, asRepo, _, asID := newASActionFixture(t)
	form := as96SetForm()
	form.Set("s1_cause_cat1", "")
	form.Set("s1_cause_cat1_custom", "직접장비")
	rec := postASAction(t, e, asID, form)
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("T3 %s", rec.Header().Get("Location"))
	}
	got, _ := asRepo.GetByID(repository.OrgAll, asID)
	if got.CauseCat1 != model.CauseCatCustom || got.CauseCat1Custom != "직접장비" {
		t.Fatalf("%s %s", got.CauseCat1, got.CauseCat1Custom)
	}
}

func Test95_96_T4_MissingCatsKeepsDraft(t *testing.T) {
	e, h, _, _, asID := newASActionFixture(t)
	form := as96SetForm()
	form.Set("s1_cause_cat1", "")
	form.Set("s1_cause_cat2", "")
	form.Set("s1_action_taken", "입력유지용 조치내용입니다")
	rec := postASAction(t, e, asID, form)
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || !strings.Contains(loc, "err=cause_cat1") {
		t.Fatalf("T4 loc=%s", loc)
	}
	page := getASActionPage(t, e, asID)
	if !strings.Contains(page, "입력유지용 조치내용입니다") {
		t.Fatal("오류 후 입력이 사라졌다")
	}
	errPage := getASActionQuery(t, e, loc)
	if !strings.Contains(errPage, "조치 1:") || !strings.Contains(errPage, "1차 분류를 선택하세요") {
		t.Fatal("안내 문구 없음")
	}
	sets, _ := h.AS.setRepo.ListByAS(asID)
	if len(sets) == 0 || !strings.Contains(sets[0].ActionTaken, "입력유지용") {
		t.Fatalf("임시 저장 없음 %+v", sets)
	}
}

func Test95_96_T5_EmptyResultMessage(t *testing.T) {
	e, _, _, _, asID := newASActionFixture(t)
	form := as96SetForm()
	form.Set("s2_work_place", "office")
	form.Set("s2_action_taken", "결과 없는 둘째 조치 내용입니다")
	rec := postASAction(t, e, asID, form)
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || !strings.Contains(loc, "set_draft_2") || !strings.Contains(loc, "focus_set=2") {
		t.Fatalf("T5 loc=%s", loc)
	}
	page := getASActionQuery(t, e, loc)
	if !strings.Contains(page, "조치 2에 결과가 선택되지 않았습니다") {
		t.Fatal("문구 없음")
	}
	if !strings.Contains(getASActionPage(t, e, asID), "결과 없는 둘째 조치 내용입니다") {
		t.Fatal("둘째 조치 입력 유실")
	}
}

func getASActionQuery(t *testing.T, e *echo.Echo, loc string) string {
	t.Helper()
	if !strings.HasPrefix(loc, "http") {
		loc = "http://localhost" + loc
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, loc, nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("follow %s status=%d", loc, rec.Code)
	}
	return rec.Body.String()
}

func Test95_96_T6_SetWinsOverBareFields(t *testing.T) {
	e, _, asRepo, _, asID := newASActionFixture(t)
	form := as96SetForm()
	form.Set("cause_cat1", "rfid")
	form.Set("cause_cat2", "rfid.hw")
	form.Set("s1_cause_cat1", "server")
	form.Set("s1_cause_cat2", "server.hw")
	rec := postASAction(t, e, asID, form)
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("T6 %s", rec.Header().Get("Location"))
	}
	got, _ := asRepo.GetByID(repository.OrgAll, asID)
	if got.CauseCat1 != "server" || got.CauseCat2 != "server.hw" {
		t.Fatalf("세트 우선 실패 %s/%s", got.CauseCat1, got.CauseCat2)
	}
}

func Test95_96_T7_BareFieldsUnchanged(t *testing.T) {
	e, _, asRepo, _, asID := newASActionFixture(t)
	rec := postASAction(t, e, asID, as96LegacyForm())
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("T7 %s", rec.Header().Get("Location"))
	}
	got, _ := asRepo.GetByID(repository.OrgAll, asID)
	if got.CauseCat1 != "server" || got.Status != "completed" {
		t.Fatalf("%s %s", got.CauseCat1, got.Status)
	}
}

func Test95_96_T8_CascadeSelectOnSetPage(t *testing.T) {
	e, _, _, _, asID := newASActionFixture(t)
	body := getASActionPage(t, e, asID)
	if strings.Contains(body, `name="s1_cause_cat2"`) && strings.Contains(body, `<input type="text" name="s1_cause_cat2"`) {
		t.Fatal("2차가 자유 입력칸이다")
	}
	if !strings.Contains(body, `name="s1_cause_cat2"`) || !strings.Contains(body, `data-cause-wrap="2"`) {
		t.Fatal("2차 연쇄 select 없음")
	}
	if !strings.Contains(body, "/static/js/as_action_set_cause.js") {
		t.Fatal("연쇄 스크립트 없음")
	}
	if !strings.Contains(body, `data-cause-custom-wrap`) {
		t.Fatal("직접 입력 래퍼 없음")
	}
	cmd := exec.Command("node", "as_action_set_cause_test.js")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("js T8: %v\n%s", err, out)
	}
}

func htmlAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

func htmlHasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return true
		}
	}
	return false
}

func htmlInnerText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

func parseRenderedActionForm(page string) url.Values {
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		return url.Values{}
	}
	form := url.Values{}
	var walk func(*html.Node, bool)
	walk = func(n *html.Node, inForm bool) {
		if n.Type == html.ElementNode {
			if n.Data == "form" {
				inForm = strings.Contains(htmlAttr(n, "action"), "/update")
			}
			if inForm {
				switch n.Data {
				case "input":
					name := htmlAttr(n, "name")
					if name == "" {
						break
					}
					typ := strings.ToLower(htmlAttr(n, "type"))
					if typ == "" {
						typ = "text"
					}
					switch typ {
					case "submit", "button", "file", "image":
					case "checkbox", "radio":
						if htmlHasAttr(n, "checked") {
							v := htmlAttr(n, "value")
							if v == "" {
								v = "on"
							}
							form.Add(name, v)
						}
					default:
						form.Add(name, htmlAttr(n, "value"))
					}
				case "textarea":
					if name := htmlAttr(n, "name"); name != "" {
						form.Add(name, htmlInnerText(n))
					}
				case "select":
					name := htmlAttr(n, "name")
					if name == "" {
						break
					}
					val := ""
					var scan func(*html.Node)
					scan = func(o *html.Node) {
						if o.Type == html.ElementNode && o.Data == "option" && htmlHasAttr(o, "selected") {
							val = htmlAttr(o, "value")
						}
						for c := o.FirstChild; c != nil; c = c.NextSibling {
							scan(c)
						}
					}
					scan(n)
					form.Add(name, val)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			next := inForm
			if n.Type == html.ElementNode && n.Data == "form" && !strings.Contains(htmlAttr(n, "action"), "/update") {
				next = false
			}
			walk(c, next)
		}
	}
	walk(doc, false)
	return form
}

func postASActionRaw(t *testing.T, e *echo.Echo, asID string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+asID+"/update",
		strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	return rec
}

func fillRenderedSetCause(t *testing.T, form url.Values, cat1, cat2, cat3 string) {
	t.Helper()
	if _, ok := form["s1_cause_cat1"]; !ok {
		t.Fatal("렌더 폼에 s1_cause_cat1 이 없다")
	}
	if _, ok := form["cause_cat1"]; !ok {
		t.Fatal("렌더 폼에 접두어 없는 cause_cat1 숨은 칸이 없다")
	}
	form.Set("s1_cause_cat1", cat1)
	form.Set("s1_cause_cat2", cat2)
	form.Set("s1_cause_cat3", cat3)
	form.Set("s1_process_type", "remote")
	form.Set("s1_work_place", "office")
	form.Set("s1_action_taken", "RFID 게이트 오류를 확인하고 재기동했습니다")
	form.Set("s1_result_code", model.ResultDone)
	form.Set("s1_leave_same", "1")
	form.Set("action_short_ok", "1")
	if _, ok := form["assignee_external"]; ok {
		form.Set("assignee_external", "양기헌")
	}
	form.Set("cause_cat1", "")
	form.Set("cause_cat1_custom", "")
	form.Set("cause_cat2", "")
	form.Set("cause_cat3", "")
}

func Test95_96_T10_RenderFormFinalSave(t *testing.T) {
	cases := []struct {
		name       string
		c1, c2, c3 string
	}{
		{"rfid_with_l3", "rfid", "rfid.hw", "rfid.hw.part_check"},
		{"l2_without_l3", "server", "server.hw", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e, h, asRepo, _, asID := newASActionFixture(t)
			form := parseRenderedActionForm(getASActionPage(t, e, asID))
			fillRenderedSetCause(t, form, tc.c1, tc.c2, tc.c3)
			if strings.TrimSpace(form.Get("cause_cat1")) != "" || strings.TrimSpace(form.Get("cause_cat2")) != "" {
				t.Fatal("접두어 없는 분류 칸이 채워져 있으면 렌더 폼 테스트가 아니다")
			}
			rec := postASActionRaw(t, e, asID, form)
			if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
				t.Fatalf("T10 %s loc=%s", tc.name, rec.Header().Get("Location"))
			}
			got, err := asRepo.GetByID(repository.OrgAll, asID)
			if err != nil || got == nil {
				t.Fatal(err)
			}
			if got.CauseCat1 != tc.c1 || got.CauseCat2 != tc.c2 || got.CauseCat3 != tc.c3 {
				t.Fatalf("접수 분류 %s/%s/%s", got.CauseCat1, got.CauseCat2, got.CauseCat3)
			}
			sets, _ := h.AS.setRepo.ListByAS(asID)
			if len(sets) == 0 || sets[0].CauseCat1 != tc.c1 || sets[0].CauseCat2 != tc.c2 || sets[0].CauseCat3 != tc.c3 {
				t.Fatalf("세트 분류 %+v", sets)
			}
		})
	}
}

func Test95_96_T11_NoLegacyCausePrefill(t *testing.T) {
	e, _, asRepo, _, asID := newASActionFixture(t)
	form := parseRenderedActionForm(getASActionPage(t, e, asID))
	fillRenderedSetCause(t, form, "rfid", "rfid.hw", "rfid.hw.part_check")
	before := form.Encode()
	fillCauseCatsFromLegacyType(form)
	if form.Encode() != before {
		t.Fatal("fillCauseCatsFromLegacyType 가 접두어 없는 분류를 보충했다 — T11은 그 경로를 쓰지 않는다")
	}
	if form.Get("cause_cat1") != "" || form.Get("s1_cause_cat1") != "rfid" {
		t.Fatalf("T11 payload cat1=%q s1=%q", form.Get("cause_cat1"), form.Get("s1_cause_cat1"))
	}
	rec := postASActionRaw(t, e, asID, form)
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || strings.Contains(loc, "err=") {
		t.Fatalf("T11 저장 실패 loc=%s", loc)
	}
	got, _ := asRepo.GetByID(repository.OrgAll, asID)
	if got.CauseCat1 != "rfid" || got.CauseCat2 != "rfid.hw" || got.CauseCat3 != "rfid.hw.part_check" {
		t.Fatalf("T11 cats %s/%s/%s", got.CauseCat1, got.CauseCat2, got.CauseCat3)
	}
}

func Test95_96_T11_PreFixOverwriteWouldFail(t *testing.T) {
	e, h, _, _, asID := newASActionFixture(t)
	form := parseRenderedActionForm(getASActionPage(t, e, asID))
	fillRenderedSetCause(t, form, "rfid", "rfid.hw", "rfid.hw.part_check")
	as, err := h.AS.repo.GetByID(repository.OrgAll, asID)
	if err != nil || as == nil {
		t.Fatal(err)
	}
	posted := []model.ASActionSet{{
		Seq: 1, CauseCat1: form.Get("s1_cause_cat1"), CauseCat2: form.Get("s1_cause_cat2"),
		CauseCat3: form.Get("s1_cause_cat3"), ActionTaken: form.Get("s1_action_taken"),
		ResultCode: form.Get("s1_result_code"), WorkPlace: "office", ProcessType: "remote",
	}}
	last := model.LastCommittedActionSet(posted)
	if last == nil {
		t.Fatal("세트 없음")
	}
	model.CopyActionSetOntoReceipt(as, *last)
	if as.CauseCat1 != "rfid" {
		t.Fatalf("복사 후 %s", as.CauseCat1)
	}
	_ = model.ApplyCauseCat1(as, form.Get("cause_cat1"), form.Get("cause_cat1_custom"))
	as.CauseCat2 = strings.TrimSpace(form.Get("cause_cat2"))
	as.CauseCat3 = strings.TrimSpace(form.Get("cause_cat3"))
	cats, _ := h.AS.listCauseCategoriesForAS(as)
	code := actionCauseCatMissingErr(as, cats)
	if code != "cause_cat1" {
		t.Fatalf("수정 전 덮어쓰기면 cause_cat1 이어야 한다 code=%s cat1=%s", code, as.CauseCat1)
	}
}

func Test95_96_T4_ErrorOrder(t *testing.T) {
	steps := []struct {
		c1, c2, c3, want string
	}{
		{"", "", "", "cause_cat1"},
		{"rfid", "", "", "cause_cat2"},
		{"rfid", "rfid.hw", "", "cause_cat3"},
	}
	for _, st := range steps {
		e, _, _, _, asID := newASActionFixture(t)
		form := parseRenderedActionForm(getASActionPage(t, e, asID))
		fillRenderedSetCause(t, form, st.c1, st.c2, st.c3)
		rec := postASActionRaw(t, e, asID, form)
		loc := rec.Header().Get("Location")
		if rec.Code != http.StatusSeeOther || !strings.Contains(loc, "err="+st.want) {
			t.Fatalf("want err=%s loc=%s", st.want, loc)
		}
		page := getASActionQuery(t, e, loc)
		if !strings.Contains(page, "조치 1:") {
			t.Fatalf("접두 문구 없음 %s", st.want)
		}
		if !strings.Contains(getASActionPage(t, e, asID), "RFID 게이트 오류를 확인하고 재기동했습니다") {
			t.Fatal("오류 후 입력 유실")
		}
	}
}

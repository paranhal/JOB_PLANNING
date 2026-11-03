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

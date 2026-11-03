package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func postASActionSet(t *testing.T, e *echo.Echo, asID string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+asID+"/action-sets", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	return rec
}

func Test95_92_VisitUsesWorkDateWithoutVisitField(t *testing.T) {
	e, h, asRepo, _, asID := newASActionFixture(t)
	form := url.Values{
		"seq": {"1"}, "s1_work_place": {"field"}, "s1_process_type": {"visit"},
		"s1_work_date": {"2026-11-03"}, "s1_start_time": {"09:00"}, "s1_duration_min": {"30"},
		"s1_action_taken": {"현장 점검 작업일만 입력"}, "s1_result_code": {model.ResultDone},
		"s1_cause_cat1": {"server"}, "s1_cause_cat2": {"server.hw"}, "s1_leave_same": {"1"},
	}
	rec := postASActionSet(t, e, asID, form)
	if rec.Code != http.StatusOK {
		t.Fatalf("작업일만 저장 %d %s", rec.Code, rec.Body.String())
	}
	sets, _ := h.AS.setRepo.ListByAS(asID)
	if len(sets) != 1 || sets[0].WorkDate != "2026-11-03" || sets[0].VisitDate != "2026-11-03" {
		t.Fatalf("VisitDate!=WorkDate %+v", sets)
	}
	got, _ := asRepo.GetByID(repository.OrgAll, asID)
	if got.Status == "completed" {
		t.Fatal("임시 저장이 접수 상태를 바꿨다")
	}
}

func Test95_92_VisitMissingWorkDate(t *testing.T) {
	e, _, _, _, asID := newASActionFixture(t)
	form := url.Values{
		"seq": {"1"}, "s1_work_place": {"field"}, "s1_process_type": {"visit"},
		"s1_action_taken": {"방문일 없이 저장"}, "s1_result_code": {model.ResultDone},
		"s1_cause_cat1": {"server"}, "s1_cause_cat2": {"server.hw"},
	}
	rec := postASActionSet(t, e, asID, form)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "work_date_required") {
		t.Fatalf("작업일 필수 %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "작업일을 입력하세요") {
		t.Fatalf("문구 %s", rec.Body.String())
	}
}

func Test95_92_RemoteClearsVisitDate(t *testing.T) {
	e, h, _, _, asID := newASActionFixture(t)
	form := url.Values{
		"seq": {"1"}, "s1_work_place": {"office"}, "s1_process_type": {"remote"},
		"s1_work_date": {"2026-11-03"}, "s1_visit_date": {"2026-01-01"},
		"s1_action_taken": {"원격 조치 내용입니다"}, "s1_result_code": {model.ResultDone},
		"s1_cause_cat1": {"server"}, "s1_cause_cat2": {"server.hw"},
	}
	rec := postASActionSet(t, e, asID, form)
	if rec.Code != http.StatusOK {
		t.Fatalf("원격 저장 %d %s", rec.Code, rec.Body.String())
	}
	sets, _ := h.AS.setRepo.ListByAS(asID)
	if len(sets) != 1 || sets[0].VisitDate != "" {
		t.Fatalf("원격 VisitDate=%q", sets[0].VisitDate)
	}
}

func Test95_92_VisitDateFallbackFillsWorkDate(t *testing.T) {
	e, h, _, _, asID := newASActionFixture(t)
	form := url.Values{
		"seq": {"1"}, "s1_work_place": {"field"}, "s1_process_type": {"visit"},
		"s1_visit_date": {"2026-08-12"},
		"s1_action_taken": {"옛 방문일 폴백"}, "s1_result_code": {model.ResultDone},
		"s1_cause_cat1": {"server"}, "s1_cause_cat2": {"server.hw"},
	}
	rec := postASActionSet(t, e, asID, form)
	if rec.Code != http.StatusOK {
		t.Fatalf("폴백 %d %s", rec.Code, rec.Body.String())
	}
	sets, _ := h.AS.setRepo.ListByAS(asID)
	if sets[0].WorkDate != "2026-08-12" || sets[0].VisitDate != "2026-08-12" {
		t.Fatalf("%+v", sets[0])
	}
}

func Test95_92_DraftSaveWithoutResult(t *testing.T) {
	e, _, asRepo, _, asID := newASActionFixture(t)
	before, _ := asRepo.GetByID(repository.OrgAll, asID)
	form := url.Values{
		"seq": {"1"}, "draft": {"1"}, "s1_work_place": {"field"},
		"s1_action_taken": {"아직 결과 없는 긴 조치내용"}, "s1_work_date": {"2026-11-03"},
		"s1_start_time": {"10:00"}, "s1_duration_min": {"40"},
	}
	rec := postASActionSet(t, e, asID, form)
	if rec.Code != http.StatusOK {
		t.Fatalf("draft %d %s", rec.Code, rec.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["state"] != model.ActionSetStateDraft {
		t.Fatalf("state %+v", body)
	}
	miss, _ := body["missing"].([]interface{})
	joined := rec.Body.String()
	if !strings.Contains(joined, "result_code") {
		t.Fatalf("missing 없음 %s", joined)
	}
	_ = miss
	after, _ := asRepo.GetByID(repository.OrgAll, asID)
	if after.Status != before.Status || after.ResultCode != before.ResultCode {
		t.Fatalf("draft가 접수 반영 status=%s result=%s", after.Status, after.ResultCode)
	}
	page := getASActionPage(t, e, asID)
	if !strings.Contains(page, "아직 결과 없는 긴 조치내용") || !strings.Contains(page, "2026-11-03") {
		t.Fatal("재조회 값 복원 실패")
	}

	bad := url.Values{
		"seq": {"1"}, "draft": {"1"}, "s1_action_taken": {"시간 오류"},
		"s1_start_time": {"09:05"}, "s1_duration_min": {"30"},
	}
	rec2 := postASActionSet(t, e, asID, bad)
	if rec2.Code != http.StatusBadRequest || !strings.Contains(rec2.Body.String(), "start_time_step") && !strings.Contains(rec2.Body.String(), "10분") {
		t.Fatalf("형식 오류 %d %s", rec2.Code, rec2.Body.String())
	}
}

func Test95_92_SaveWithoutDraftRequiresResult(t *testing.T) {
	e, _, _, _, asID := newASActionFixture(t)
	form := url.Values{"seq": {"1"}, "s1_action_taken": {"결과 없이 저장"}, "s1_work_place": {"field"}}
	rec := postASActionSet(t, e, asID, form)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "result_code") {
		t.Fatalf("결과 필수 %d %s", rec.Code, rec.Body.String())
	}
}

func Test95_92_DraftThenFinalBlocksUntilResult(t *testing.T) {
	e, _, asRepo, _, asID := newASActionFixture(t)
	_ = postASActionSet(t, e, asID, url.Values{
		"seq": {"1"}, "draft": {"1"}, "s1_work_place": {"office"}, "s1_process_type": {"remote"},
		"s1_action_taken": {"임시만 있는 조치내용"},
	})
	saved := url.Values{
		"seq": {"1"}, "s1_work_place": {"office"}, "s1_process_type": {"remote"},
		"s1_action_taken": {"확정된 이전 조치내용입니다"}, "s1_result_code": {model.ResultPartial},
		"s1_cause_cat1": {"server"}, "s1_cause_cat2": {"server.hw"},
	}
	if rec := postASActionSet(t, e, asID, saved); rec.Code != http.StatusOK {
		t.Fatalf("saved %s", rec.Body.String())
	}
	draft2 := url.Values{
		"seq": {"2"}, "draft": {"1"}, "s2_work_place": {"office"},
		"s2_action_taken": {"다음 조치 임시"},
	}
	if rec := postASActionSet(t, e, asID, draft2); rec.Code != http.StatusOK {
		t.Fatalf("draft2 %s", rec.Body.String())
	}
	before, _ := asRepo.GetByID(repository.OrgAll, asID)
	final := url.Values{
		"s1_work_place": {"office"}, "s1_process_type": {"remote"},
		"s1_action_taken": {"확정된 이전 조치내용입니다"}, "s1_result_code": {model.ResultPartial},
		"s1_cause_cat1": {"server"}, "s1_cause_cat2": {"server.hw"},
		"s2_work_place": {"office"}, "s2_action_taken": {"다음 조치 임시"},
		"work_place": {"office"}, "process_type": {"remote"}, "cause_type": {"hw"},
		"action_taken": {"다음 조치 임시"}, "result_code": {""},
		"action_short_ok": {"1"},
	}
	rec := postASAction(t, e, asID, final)
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || !strings.Contains(loc, "set_draft") {
		t.Fatalf("draft 잔류 거절 %d %s", rec.Code, loc)
	}
	after, _ := asRepo.GetByID(repository.OrgAll, asID)
	if after.Status != before.Status {
		t.Fatalf("draft 마지막인데 상태가 바뀜 %s→%s", before.Status, after.Status)
	}
	ok := url.Values{
		"s1_work_place": {"office"}, "s1_process_type": {"remote"},
		"s1_action_taken": {"확정된 이전 조치내용입니다"}, "s1_result_code": {model.ResultPartial},
		"s1_cause_cat1": {"server"}, "s1_cause_cat2": {"server.hw"},
		"s2_work_place": {"office"}, "s2_process_type": {"remote"},
		"s2_action_taken": {"다음 조치 완료 내용입니다 충분히"}, "s2_result_code": {model.ResultDone},
		"s2_cause_cat1": {"server"}, "s2_cause_cat2": {"server.hw"},
		"work_place": {"office"}, "process_type": {"remote"}, "cause_type": {"hw"},
		"cause_cat1": {"server"}, "cause_cat2": {"server.hw"},
		"action_taken": {"다음 조치 완료 내용입니다 충분히"}, "result_code": {model.ResultDone},
		"action_short_ok": {"1"},
	}
	rec = postASAction(t, e, asID, ok)
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("최종 %d %s", rec.Code, rec.Header().Get("Location"))
	}
	done, _ := asRepo.GetByID(repository.OrgAll, asID)
	if done.Status != "completed" || done.ResultCode != model.ResultDone {
		t.Fatalf("최종 상태 %s %s", done.Status, done.ResultCode)
	}
}

func Test95_92_ActionSetPageHasNoVisitInput(t *testing.T) {
	e, _, _, _, asID := newASActionFixture(t)
	body := getASActionPage(t, e, asID)
	if strings.Contains(body, `class="as-set-visit-date"`) || strings.Contains(body, `name="s1_visit_date"`) {
		t.Fatal("세트 방문일 입력 칸이 남아 있다")
	}
	if !strings.Contains(body, "작업일이 방문일로") {
		t.Fatal("현장방문 안내 없음")
	}
	if !strings.Contains(body, "as-set-work-date") {
		t.Fatal("작업일 칸 없음")
	}
	if !strings.Contains(body, "임시 저장") {
		t.Fatal("임시 저장 버튼 없음")
	}
}

func Test95_92_JSUnit(t *testing.T) {
	cmd := exec.Command("node", "as_action_sets_test.js")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("js: %v\n%s", err, out)
	}
}

func Test95_92_Browser(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	cmd := exec.Command("node", "95_92_as_action_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(), "PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := string(out)
		if strings.Contains(msg, "Executable doesn't exist") || strings.Contains(msg, "playwright") && strings.Contains(msg, "install") {
			t.Skip(msg)
		}
		t.Fatalf("browser: %v\n%s", err, out)
	}
}

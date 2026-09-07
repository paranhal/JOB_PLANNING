package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

func TestASActionFormDropsStatusPartsFollowupAndWorkList(t *testing.T) {
	e, _, _, _, asID := newASActionFixture(t)
	body := getASActionPage(t, e, asID)
	formStart := strings.Index(body, `id="as-action-form"`)
	if formStart < 0 {
		t.Fatal("조치 폼 없음")
	}
	formEndRel := strings.Index(body[formStart:], "</form>")
	form := body[formStart : formStart+formEndRel]
	for _, s := range []string{`name="status"`, `name="parts_used"`, `name="followup_action"`, `id="as-action-status"`, "사용부품/교체품", "후속 조치", "하부업무 담당자"} {
		if strings.Contains(form, s) {
			t.Errorf("폼에 %q 가 남아 있다", s)
		}
	}
	if strings.Contains(body, ">하부 업무</h3>") {
		t.Error("조치 화면에 하부 업무 목록이 남아 있다")
	}
	if !strings.Contains(form, `id="as-process-type"`) || !strings.Contains(form, `data-places="office"`) {
		t.Error("처리유형 data-places 가 없다")
	}
	if !strings.Contains(form, `name="cause_cat1"`) || !strings.Contains(form, ">RFID<") {
		t.Error("원인분류 1차가 없다")
	}
	if !strings.Contains(form, `id="as-cause-cat2-wrap"`) || !strings.Contains(form, `id="as-cause-cat3-wrap"`) {
		t.Error("2·3차 래퍼가 없다")
	}
}

func TestASActionRejectsProcessTypeMismatchAndBannedResults(t *testing.T) {
	e, _, asRepo, _, asID := newASActionFixture(t)
	rec := postASAction(t, e, asID, url.Values{
		"work_place":   {"office"},
		"process_type": {"visit"},
		"cause_type":   {"hw"},
		"action_taken": {"원격 안내"},
		"result_code":  {model.ResultDone},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=process_type_mismatch") {
		t.Fatalf("내근+현장방문이 거절되지 않음: %s", rec.Header().Get("Location"))
	}

	rec = postASAction(t, e, asID, url.Values{
		"work_place":             {"field"},
		"process_type":           {"visit"},
		"cause_type":             {"hw"},
		"action_taken":           {"재방문 요청"},
		"result_code":            {model.ResultRevisit},
		"revisit_scheduled_date": {"2026-08-20"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=result_not_allowed") {
		t.Fatalf("재방문 결과가 거절되지 않음: %s", rec.Header().Get("Location"))
	}

	rec = postASAction(t, e, asID, url.Values{
		"work_place":   {"office"},
		"process_type": {"remote"},
		"cause_type":   {"hw"},
		"action_taken": {"대기 요청"},
		"result_code":  {model.ResultHold},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=result_not_allowed") {
		t.Fatalf("대기 결과가 거절되지 않음: %s", rec.Header().Get("Location"))
	}

	got, _ := asRepo.GetByID(asID)
	if got.Status == "completed" || got.Status == "hold" {
		t.Fatalf("거절된 저장이 반영됨: %+v", got)
	}
}

func TestASActionSavesCauseCat1(t *testing.T) {
	e, _, asRepo, _, asID := newASActionFixture(t)
	rec := postASAction(t, e, asID, url.Values{
		"work_place":   {"field"},
		"process_type": {"visit"},
		"cause_cat1":   {"rfid"},
		"cause_cat2":   {"rfid.hw"},
		"cause_cat3":   {"rfid.hw.part_check"},
		"cause_detail": {"안테나 불량"},
		"action_taken": {"안테나 점검"},
		"result_code":  {model.ResultDone},
	})
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || strings.Contains(loc, "err=") {
		t.Fatalf("저장 실패: %s", loc)
	}
	got, err := asRepo.GetByID(asID)
	if err != nil || got == nil {
		t.Fatal(err)
	}
	if got.CauseCat1 != "rfid" {
		t.Fatalf("cause_cat1=%q", got.CauseCat1)
	}
	if got.CauseType != "hw" {
		t.Fatalf("cause_type 자동채움=%q", got.CauseType)
	}
	if got.CauseDetail != "안테나 불량" {
		t.Fatalf("cause_detail=%q", got.CauseDetail)
	}
}

func TestASActionKeepsPartsUsedWhenFormOmitsIt(t *testing.T) {
	e, _, asRepo, _, asID := newASActionFixture(t)
	cur, err := asRepo.GetByID(asID)
	if err != nil || cur == nil {
		t.Fatal(err)
	}
	cur.PartsUsed = "기존부품"
	cur.FollowupAction = "기존후속"
	cur.WorkPlace = model.WorkPlaceField
	cur.ProcessType = "visit"
	cur.CauseType = "hw"
	cur.ActionTaken = "1차"
	if err := asRepo.Update(cur); err != nil {
		t.Fatal(err)
	}
	rec := postASAction(t, e, asID, url.Values{
		"work_place":   {"field"},
		"process_type": {"visit"},
		"cause_type":   {"hw"},
		"action_taken": {"2차 점검"},
		"result_code":  {model.ResultDone},
	})
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("저장 실패: %s", rec.Header().Get("Location"))
	}
	got, _ := asRepo.GetByID(asID)
	if got.PartsUsed != "기존부품" || got.FollowupAction != "기존후속" {
		t.Fatalf("컬럼이 지워짐: parts=%q followup=%q", got.PartsUsed, got.FollowupAction)
	}
}

func TestASWorkActionDropsStatusSelect(t *testing.T) {
	e, h, _, _, asID := newASActionFixture(t)

	w := &model.ASWorkItem{ASID: asID, WorkKind: "revisit", Status: "open", Notes: "확인"}
	if err := h.AS.workRepo.Create(w); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/work/"+w.WorkID+"/action", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), `name="status"`) {
		t.Fatal("하부업무 조치 폼에 상태가 남아 있다")
	}

	post := httptest.NewRecorder()
	preq := httptest.NewRequest(http.MethodPost, "http://localhost/as/work/"+w.WorkID+"/action",
		strings.NewReader(url.Values{
			"status": {"done"},
			"notes":  {"저장만"},
		}.Encode()))
	preq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	preq.AddCookie(jwtCookie(t))
	e.ServeHTTP(post, preq)
	if post.Code != http.StatusSeeOther {
		t.Fatalf("저장 status=%d", post.Code)
	}
	got, err := h.AS.workRepo.GetByID(w.WorkID)
	if err != nil || got == nil {
		t.Fatal(err)
	}
	if got.Status != "open" {
		t.Fatalf("상태만으로 완료되면 안 됨: %s", got.Status)
	}

	done := httptest.NewRecorder()
	dreq := httptest.NewRequest(http.MethodPost, "http://localhost/as/work/"+w.WorkID+"/action",
		strings.NewReader(url.Values{
			"mark_done": {"1"},
			"notes":     {"완료"},
		}.Encode()))
	dreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	dreq.AddCookie(jwtCookie(t))
	e.ServeHTTP(done, dreq)
	got, _ = h.AS.workRepo.GetByID(w.WorkID)
	if got.Status != "done" {
		t.Fatalf("완료 처리 실패: %s", got.Status)
	}
}

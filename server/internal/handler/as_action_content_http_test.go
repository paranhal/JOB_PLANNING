package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func TestASActionNAReasonSavesAndEmptyBlocked(t *testing.T) {
	e, h, asRepo, _, asID := newASActionFixture(t)

	empty := postASAction(t, e, asID, url.Values{
		"status":       {"in_progress"},
		"work_place":   {"field"},
		"process_type": {"visit"},
		"cause_type":   {"hw"},
		"action_taken": {""},
		"time_spent":   {"30"},
		"result_code":  {model.ResultDone},
	})
	if !strings.Contains(empty.Header().Get("Location"), "err=action_required") {
		t.Fatalf("빈 조치가 막히지 않음: %s", empty.Header().Get("Location"))
	}

	noReason := postASAction(t, e, asID, url.Values{
		"status":           {"in_progress"},
		"work_place":       {"field"},
		"process_type":     {"visit"},
		"cause_type":       {"hw"},
		"action_na":        {"1"},
		"action_na_reason": {""},
		"time_spent":       {"30"},
		"result_code":      {model.ResultDone},
	})
	if !strings.Contains(noReason.Header().Get("Location"), "err=action_na_reason") {
		t.Fatalf("해당 없음 사유 없음: %s", noReason.Header().Get("Location"))
	}

	ok := postASAction(t, e, asID, url.Values{
		"status":           {"in_progress"},
		"work_place":       {"field"},
		"process_type":     {"visit"},
		"cause_type":       {"hw"},
		"action_na":        {"1"},
		"action_na_reason": {"원격만 안내하고 종료"},
		"time_spent":       {"30"},
		"result_code":      {model.ResultDone},
	})
	if strings.Contains(ok.Header().Get("Location"), "err=") {
		t.Fatalf("해당 없음+사유가 거절: %s", ok.Header().Get("Location"))
	}
	got, err := asRepo.GetByID(asID)
	if err != nil || got == nil || !strings.HasPrefix(got.ActionTaken, model.ActionNAPrefix) {
		t.Fatalf("저장 본문: %+v err=%v", got, err)
	}
	procs, _ := h.AS.processRepo.ListByAS(asID)
	if len(procs) == 0 || !strings.HasPrefix(procs[0].WorkContent, model.ActionNAPrefix) {
		t.Fatalf("이력: %+v", procs)
	}
}

func TestASActionShortAsksOnceThenSaves(t *testing.T) {
	e, _, asRepo, _, asID := newASActionFixture(t)
	form := url.Values{
		"status":           {"in_progress"},
		"work_place":       {"field"},
		"process_type":     {"visit"},
		"cause_type":       {"hw"},
		"action_taken":     {"재시작"},
		"time_spent":       {"30"},
		"result_code":      {model.ResultDone},
		"action_short_ok":  {"0"},
	}
	ask := postASAction(t, e, asID, form)
	if !strings.Contains(ask.Header().Get("Location"), "err=action_short") {
		t.Fatalf("짧은 조치 되묻기 없음: %s", ask.Header().Get("Location"))
	}
	got, _ := asRepo.GetByID(asID)
	if got != nil && got.Status == "completed" {
		t.Fatal("되묻기 전에 완료 저장")
	}

	form.Set("action_short_ok", "1")
	save := postASAction(t, e, asID, form)
	if strings.Contains(save.Header().Get("Location"), "err=") {
		t.Fatalf("되묻기 후 저장 거절: %s", save.Header().Get("Location"))
	}
	got, err := asRepo.GetByID(asID)
	if err != nil || got == nil || got.ActionTaken != "재시작" {
		t.Fatalf("짧은 본문 저장: %+v err=%v", got, err)
	}
}

func TestASActionPageHintAndShortBanner(t *testing.T) {
	e, _, _, _, asID := newASActionFixture(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+asID+"/action", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "이 내용이 다음 사람에게 답이 됩니다") {
		t.Fatal("조치 화면 안내 문구가 없다")
	}
	if !strings.Contains(body, `name="action_na"`) {
		t.Fatal("해당 없음 회피 경로가 없다")
	}
	if !strings.Contains(body, "조금 더 적어 주시겠습니까?") {
		t.Fatal("10자 미만 되묻기 문구가 없다")
	}

	ban := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+asID+"/action?err=action_short", nil)
	req2.AddCookie(jwtCookie(t))
	e.ServeHTTP(ban, req2)
	if !strings.Contains(ban.Body.String(), "조금 더 적어 주시겠습니까?") {
		t.Fatal("action_short 배너가 없다")
	}
}

func TestASCaseVoteThenKnowledgeOrder(t *testing.T) {
	e, h, _, _, asID := newASActionFixture(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+asID+"/vote", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("vote status=%d", rec.Code)
	}
	if !h.AS.repo.CaseVotedBy(asID, "admin-id") {
		t.Fatal("👍 가 저장되지 않았다")
	}
	if h.AS.repo.CaseVoteCount(asID) != 1 {
		t.Fatal("투표 건수")
	}
}

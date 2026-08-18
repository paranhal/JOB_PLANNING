package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestASActionSavesTimeSpentOnProcess(t *testing.T) {
	e, h, _, _, asID := newASActionFixture(t)

	rec := postASAction(t, e, asID, url.Values{
		"status":       {"in_progress"},
		"work_place":   {"field"},
		"process_type": {"visit"},
		"cause_type":   {"hw"},
		"action_taken": {"현장 점검"},
		"time_spent":   {"45"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("저장 실패: status=%d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
	if strings.Contains(rec.Header().Get("Location"), "err=time_spent") {
		t.Fatal("소요시간 45분이 거절됐다")
	}

	procs, err := h.AS.processRepo.ListByAS(asID)
	if err != nil || len(procs) != 1 {
		t.Fatalf("조치 이력: n=%d err=%v", len(procs), err)
	}
	if procs[0].TimeSpent != 45 {
		t.Fatalf("time_spent=%d want 45", procs[0].TimeSpent)
	}
}

func TestASActionRejectsZeroOrMissingTimeSpent(t *testing.T) {
	e, h, _, _, asID := newASActionFixture(t)

	for _, spent := range []string{"", "0", "-10"} {
		rec := postASAction(t, e, asID, url.Values{
			"status":       {"in_progress"},
			"work_place":   {"field"},
			"process_type": {"visit"},
			"cause_type":   {"hw"},
			"action_taken": {"현장 점검"},
			"time_spent":   {spent},
		})
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("time_spent=%q status=%d", spent, rec.Code)
		}
		loc := rec.Header().Get("Location")
		if !strings.Contains(loc, "err=time_spent") {
			t.Fatalf("time_spent=%q 가 거절되지 않았다: loc=%s", spent, loc)
		}
	}

	procs, err := h.AS.processRepo.ListByAS(asID)
	if err != nil {
		t.Fatal(err)
	}
	if len(procs) != 0 {
		t.Fatalf("거절된 조치가 저장됐다: n=%d", len(procs))
	}
}

func TestASActionPageHasTimeSpentInput(t *testing.T) {
	e, _, _, _, asID := newASActionFixture(t)

	show := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+asID+"/action", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(show, req)
	if show.Code != http.StatusOK {
		t.Fatalf("조치 화면: status=%d", show.Code)
	}
	body := show.Body.String()
	if !strings.Contains(body, `name="time_spent"`) {
		t.Error("소요시간 입력란이 없다")
	}
	if !strings.Contains(body, `value="30"`) {
		t.Error("소요시간 기본값 30이 없다")
	}

	errPage := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+asID+"/action?err=time_spent", nil)
	req2.AddCookie(jwtCookie(t))
	e.ServeHTTP(errPage, req2)
	if errPage.Code != http.StatusOK {
		t.Fatalf("오류 화면: status=%d", errPage.Code)
	}
	if !strings.Contains(errPage.Body.String(), "소요시간을 입력하세요") {
		t.Error("오류 배너 문구가 없다")
	}
}

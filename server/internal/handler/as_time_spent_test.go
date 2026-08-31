package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func TestASActionCopiesDurationMinToTimeSpent(t *testing.T) {
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
		t.Fatal("소요시간 폼 값이 없어도 저장돼야 한다")
	}

	procs, err := h.AS.processRepo.ListByAS(asID)
	if err != nil || len(procs) != 1 {
		t.Fatalf("조치 이력: n=%d err=%v", len(procs), err)
	}
	if procs[0].TimeSpent != 30 {
		t.Fatalf("폼 45분을 무시하고 기본 30이어야 한다: time_spent=%d", procs[0].TimeSpent)
	}
}

func TestASActionSavesTimeSpentFromWorkTaskDuration(t *testing.T) {
	e, h, _, _, asID := newASActionFixture(t)
	if err := h.AS.wbRepo.CreateTask(&model.WorkTask{
		WorkType:    model.WBWorkAS,
		Title:       "AS",
		DueDate:     "2026-08-20",
		DurationMin: 90,
		Status:      model.WBTaskWaiting,
		SourceType:  model.WBSourceAS,
		SourceID:    asID,
	}); err != nil {
		t.Fatal(err)
	}

	rec := postASAction(t, e, asID, url.Values{
		"status":       {"in_progress"},
		"work_place":   {"field"},
		"process_type": {"visit"},
		"cause_type":   {"hw"},
		"action_taken": {"현장 점검"},
		"time_spent":   {"15"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("저장 실패: status=%d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
	procs, err := h.AS.processRepo.ListByAS(asID)
	if err != nil || len(procs) != 1 {
		t.Fatalf("조치 이력: n=%d err=%v", len(procs), err)
	}
	if procs[0].TimeSpent != 90 {
		t.Fatalf("duration_min=90 을 써야 한다: time_spent=%d", procs[0].TimeSpent)
	}
}

func TestASActionAcceptsMissingTimeSpentAndStoresDefault(t *testing.T) {
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
		if strings.Contains(loc, "err=time_spent") {
			t.Fatalf("time_spent=%q 가 거절됐다: loc=%s", spent, loc)
		}
	}

	procs, err := h.AS.processRepo.ListByAS(asID)
	if err != nil {
		t.Fatal(err)
	}
	if len(procs) != 3 {
		t.Fatalf("빈 소요시간도 저장돼야 한다: n=%d", len(procs))
	}
	for _, p := range procs {
		if p.TimeSpent != 30 {
			t.Fatalf("기본 30분이 아니다: time_spent=%d", p.TimeSpent)
		}
	}
}

func TestASActionPageHasNoTimeSpentInput(t *testing.T) {
	e, _, _, _, asID := newASActionFixture(t)

	show := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+asID+"/action", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(show, req)
	if show.Code != http.StatusOK {
		t.Fatalf("조치 화면: status=%d", show.Code)
	}
	body := show.Body.String()
	if strings.Contains(body, `name="time_spent"`) {
		t.Error("소요시간 입력란이 있으면 안 된다")
	}
}

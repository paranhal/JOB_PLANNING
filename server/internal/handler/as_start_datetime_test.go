package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestASActionFillsStartDatetimeFromFirstProcess(t *testing.T) {
	e, h, asRepo, _, asID := newASActionFixture(t)

	before, err := asRepo.GetByID(asID)
	if err != nil || before == nil {
		t.Fatalf("조회 실패: %v", err)
	}
	if before.StartDatetime != nil && !before.StartDatetime.IsZero() {
		t.Fatalf("착수시각이 비어 있어야 한다: %v", before.StartDatetime)
	}

	rec := postASAction(t, e, asID, url.Values{
		"status":       {"in_progress"},
		"work_place":   {"field"},
		"process_type": {"visit"},
		"cause_type":   {"hw"},
		"action_taken": {"현장 점검"},
		"time_spent":   {"30"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("저장 실패: status=%d body=%s", rec.Code, rec.Body.String())
	}

	got, err := asRepo.GetByID(asID)
	if err != nil || got == nil {
		t.Fatalf("저장 후 조회 실패: %v", err)
	}
	if got.StartDatetime == nil || got.StartDatetime.IsZero() {
		t.Fatal("착수시각 없이 조치를 저장하면 start_datetime이 채워져야 한다")
	}

	procs, err := h.AS.processRepo.ListByAS(asID)
	if err != nil || len(procs) != 1 {
		t.Fatalf("조치 이력: n=%d err=%v", len(procs), err)
	}
	start := got.StartDatetime.Format("2006-01-02 15:04:05")
	proc := procs[0].ProcessDatetime.Format("2006-01-02 15:04:05")
	if start != proc {
		t.Fatalf("start_datetime=%s process_datetime=%s — 첫 조치 시각과 같아야 한다", start, proc)
	}
}

func TestASActionDoesNotOverwriteStartDatetimeOrCopyToProcess(t *testing.T) {
	e, h, asRepo, _, asID := newASActionFixture(t)

	existing := time.Date(2026, 8, 1, 10, 0, 0, 0, time.Local)
	as, err := asRepo.GetByID(asID)
	if err != nil || as == nil {
		t.Fatal(err)
	}
	as.StartDatetime = &existing
	as.Status = "in_progress"
	if err := asRepo.Update(as); err != nil {
		t.Fatal(err)
	}

	rec := postASAction(t, e, asID, url.Values{
		"status":         {"in_progress"},
		"work_place":     {"field"},
		"process_type":   {"visit"},
		"cause_type":     {"hw"},
		"action_taken":   {"2차 조치"},
		"start_datetime": {"2020-01-01T00:00"},
		"time_spent":     {"30"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("저장 실패: status=%d", rec.Code)
	}

	got, err := asRepo.GetByID(asID)
	if err != nil || got == nil || got.StartDatetime == nil {
		t.Fatalf("조회 실패: %+v err=%v", got, err)
	}
	if got.StartDatetime.Format("2006-01-02 15:04:05") != "2026-08-01 10:00:00" {
		t.Fatalf("기존 착수시각을 덮어썼다: %v", got.StartDatetime)
	}

	procs, err := h.AS.processRepo.ListByAS(asID)
	if err != nil || len(procs) != 1 {
		t.Fatalf("조치 이력: n=%d err=%v", len(procs), err)
	}
	if procs[0].ProcessDatetime.Truncate(time.Minute).Equal(existing) {
		t.Fatal("start_datetime을 process_datetime으로 복사하면 안 된다")
	}
}

func TestASActionPageDoesNotExposeStartDatetimeInput(t *testing.T) {
	e, _, _, _, asID := newASActionFixture(t)

	show := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+asID+"/action", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(show, req)
	if show.Code != http.StatusOK {
		t.Fatalf("조치 화면: status=%d", show.Code)
	}
	body := show.Body.String()
	if strings.Contains(body, `name="start_datetime"`) {
		t.Error("착수시각 입력란이 있으면 안 된다")
	}
	if strings.Contains(body, "착수 시각") || strings.Contains(body, "첫 조치 저장 시 자동 기록") {
		t.Error("조치 화면에 착수 시각 안내가 남아 있다")
	}
}

func TestASShowDisplaysStartDatetimeReadOnly(t *testing.T) {
	e, h, asRepo, _, asID := newASActionFixture(t)
	e.GET("/as/:id", h.AS.Show, h.Auth.AuthMiddleware)

	rec := postASAction(t, e, asID, url.Values{
		"status":       {"in_progress"},
		"work_place":   {"field"},
		"process_type": {"visit"},
		"cause_type":   {"hw"},
		"action_taken": {"현장 점검"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("저장 실패: status=%d", rec.Code)
	}
	got, err := asRepo.GetByID(asID)
	if err != nil || got == nil || got.StartDatetime == nil {
		t.Fatalf("start_datetime 미기록: %+v err=%v", got, err)
	}
	want := "착수 " + got.StartDatetime.Format("2006-01-02 15:04")

	page := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+asID, nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(page, req)
	if page.Code != http.StatusOK {
		t.Fatalf("상세: status=%d", page.Code)
	}
	body := page.Body.String()
	if strings.Contains(body, `name="start_datetime"`) {
		t.Error("상세에 착수 입력란이 있다")
	}
	if !strings.Contains(body, want) {
		t.Fatalf("상세에 %q 가 없다", want)
	}
}

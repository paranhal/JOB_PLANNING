package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func TestASActionRejectsEmptyActionTakenOnDone(t *testing.T) {
	e, h, asRepo, _, asID := newASActionFixture(t)

	for _, taken := range []string{"", "   "} {
		rec := postASAction(t, e, asID, url.Values{
			"status":       {"in_progress"},
			"work_place":   {"field"},
			"process_type": {"visit"},
			"cause_type":   {"hw"},
			"action_taken": {taken},
			"time_spent":   {"30"},
			"result_code":  {model.ResultDone},
		})
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("action_taken=%q status=%d", taken, rec.Code)
		}
		loc := rec.Header().Get("Location")
		if !strings.Contains(loc, "err=action_required") {
			t.Fatalf("action_taken=%q 가 거절되지 않았다: loc=%s", taken, loc)
		}
	}

	got, err := asRepo.GetByID(asID)
	if err != nil || got == nil {
		t.Fatal(err)
	}
	if got.Status == "completed" || got.Status == "closed" {
		t.Fatalf("공란 완료가 저장됐다: status=%s", got.Status)
	}
	procs, err := h.AS.processRepo.ListByAS(asID)
	if err != nil {
		t.Fatal(err)
	}
	if len(procs) != 0 {
		t.Fatalf("거절된 조치가 이력에 남았다: n=%d content=%q", len(procs), procs[0].WorkContent)
	}
}

func TestASActionDoneRequiresWorkPlaceCauseProcessType(t *testing.T) {
	base := url.Values{
		"status":       {"in_progress"},
		"work_place":   {"field"},
		"process_type": {"visit"},
		"cause_type":   {"hw"},
		"action_taken": {"현장 점검 완료"},
		"time_spent":   {"30"},
		"result_code":  {model.ResultDone},
	}
	cases := []struct {
		drop, wantErr string
	}{
		{"work_place", "work_place"},
		{"process_type", "process_type"},
		{"cause_type", "cause_type"},
	}
	for _, tc := range cases {
		e, h, asRepo, _, asID := newASActionFixture(t)
		form := url.Values{}
		for k, v := range base {
			if k == tc.drop {
				continue
			}
			form[k] = append([]string{}, v...)
		}
		rec := postASAction(t, e, asID, form)
		loc := rec.Header().Get("Location")
		if rec.Code != http.StatusSeeOther || !strings.Contains(loc, "err="+tc.wantErr) {
			t.Fatalf("%s 누락이 거절되지 않았다: status=%d loc=%s", tc.drop, rec.Code, loc)
		}
		got, _ := asRepo.GetByID(asID)
		if got != nil && got.Status == "completed" {
			t.Fatalf("%s 누락인데 completed 저장", tc.drop)
		}
		procs, _ := h.AS.processRepo.ListByAS(asID)
		if len(procs) != 0 {
			t.Fatalf("%s 누락인데 이력이 남았다", tc.drop)
		}
	}
}

func TestASActionDoneWithRequiredFieldsCompletes(t *testing.T) {
	e, h, asRepo, _, asID := newASActionFixture(t)
	rec := postASAction(t, e, asID, url.Values{
		"status":       {"in_progress"},
		"work_place":   {"field"},
		"process_type": {"visit"},
		"cause_type":   {"hw"},
		"action_taken": {"프로그램 재시작 후 정상"},
		"time_spent":   {"30"},
		"result_code":  {model.ResultDone},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("저장 실패: status=%d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
	if strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("필수값을 채웠는데 거절: loc=%s", rec.Header().Get("Location"))
	}
	got, err := asRepo.GetByID(asID)
	if err != nil || got == nil || got.Status != "completed" {
		t.Fatalf("완료 상태: %+v err=%v", got, err)
	}
	if got.ActionTaken != "프로그램 재시작 후 정상" || got.WorkPlace != model.WorkPlaceField ||
		got.ProcessType != "visit" || got.CauseType != "hw" {
		t.Fatalf("필수 필드: %+v", got)
	}
	procs, err := h.AS.processRepo.ListByAS(asID)
	if err != nil || len(procs) != 1 {
		t.Fatalf("이력: n=%d err=%v", len(procs), err)
	}
	if procs[0].WorkContent == "조치 저장" {
		t.Fatal("조치내용 대신 자동 문구가 저장됐다")
	}
}

func TestASActionPageMapsActionRequiredBanner(t *testing.T) {
	e, _, _, _, asID := newASActionFixture(t)
	cases := []struct{ code, msg string }{
		{"action_required", "조치내용을 입력하세요"},
		{"work_place", "근무구분(내근/외근)을 선택하세요"},
		{"cause_type", "원인분류를 선택하세요"},
		{"process_type", "처리유형을 선택하세요"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+asID+"/action?err="+tc.code, nil)
		req.AddCookie(jwtCookie(t))
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s 화면 status=%d", tc.code, rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, tc.msg) {
			t.Errorf("%s 배너 %q 없음", tc.code, tc.msg)
		}
	}
}

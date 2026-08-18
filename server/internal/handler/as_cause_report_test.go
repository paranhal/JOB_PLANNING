package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func TestASActionCauseReportFieldsOnPage(t *testing.T) {
	e, _, _, _, asID := newASActionFixture(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+asID+"/action", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="as-cause-report-wrap"`,
		`name="cause_detail"`,
		`name="conclusion"`,
		`name="cause_type"`,
		"장애원인",
		"결론",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q", want)
		}
	}
	if !strings.Contains(body, `id="as-cause-report-wrap"`) || !strings.Contains(body, "hidden") {
		t.Fatal("완료가 아닐 때 숨기는 표시가 없다")
	}
}

func TestASActionDoneEmptyCauseReportWarnsButSaves(t *testing.T) {
	e, _, asRepo, _, asID := newASActionFixture(t)
	rec := postASAction(t, e, asID, url.Values{
		"status":       {"in_progress"},
		"work_place":   {"field"},
		"process_type": {"visit"},
		"cause_type":   {"hw"},
		"action_taken": {"프로그램 재시작 후 정상"},
		"time_spent":   {"30"},
		"result_code":  {model.ResultDone},
	})
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d", rec.Code)
	}
	if strings.Contains(loc, "err=") {
		t.Fatalf("빈 서술 때문에 저장을 막으면 안 됨: %s", loc)
	}
	if !strings.Contains(loc, "warn=cause_report") {
		t.Fatalf("경고 없음: %s", loc)
	}
	got, err := asRepo.GetByID(asID)
	if err != nil || got == nil || got.Status != "completed" {
		t.Fatalf("완료 저장 실패: %+v err=%v", got, err)
	}
	if got.CauseType != "hw" {
		t.Fatalf("cause_type 이 바뀌면 안 됨: %q", got.CauseType)
	}
	if got.CauseDetail != "" || got.Conclusion != "" {
		t.Fatalf("비운 서술이 채워짐: detail=%q conclusion=%q", got.CauseDetail, got.Conclusion)
	}

	page := httptest.NewRecorder()
	preq := httptest.NewRequest(http.MethodGet, "http://localhost"+loc, nil)
	preq.AddCookie(jwtCookie(t))
	e.ServeHTTP(page, preq)
	if !strings.Contains(page.Body.String(), "보고서 발급 때 다시 필요합니다") {
		t.Fatal("경고 배너 없음")
	}
}

func TestASActionDoneSavesCauseDetailAndConclusion(t *testing.T) {
	e, _, asRepo, _, asID := newASActionFixture(t)
	rec := postASAction(t, e, asID, url.Values{
		"status":       {"in_progress"},
		"work_place":   {"field"},
		"process_type": {"visit"},
		"cause_type":   {"sw"},
		"cause_detail": {"설정 오류"},
		"conclusion":   {"설정 오류로 로그인 실패 문제가 발생했고, 설정 복구하여 정상작동하는 것으로 확인됨."},
		"action_taken": {"설정 복구"},
		"time_spent":   {"30"},
		"result_code":  {model.ResultDone},
	})
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || strings.Contains(loc, "err=") || strings.Contains(loc, "warn=") {
		t.Fatalf("저장 실패: status=%d loc=%s", rec.Code, loc)
	}
	got, err := asRepo.GetByID(asID)
	if err != nil || got == nil {
		t.Fatal(err)
	}
	if got.CauseType != "sw" {
		t.Fatalf("cause_type=%q", got.CauseType)
	}
	if got.CauseDetail != "설정 오류" {
		t.Fatalf("cause_detail=%q", got.CauseDetail)
	}
	if !strings.Contains(got.Conclusion, "설정 복구하여 정상작동") {
		t.Fatalf("conclusion=%q", got.Conclusion)
	}
}

func TestASActionRevisitDoesNotClearCauseReport(t *testing.T) {
	e, _, asRepo, _, asID := newASActionFixture(t)
	cur, err := asRepo.GetByID(asID)
	if err != nil || cur == nil {
		t.Fatal(err)
	}
	cur.CauseDetail = "전원 불량"
	cur.Conclusion = "이미 적어 둔 결론"
	cur.CauseType = "hw"
	cur.WorkPlace = model.WorkPlaceField
	cur.ProcessType = "visit"
	cur.ActionTaken = "1차 점검"
	if err := asRepo.Update(cur); err != nil {
		t.Fatal(err)
	}

	rec := postASAction(t, e, asID, url.Values{
		"status":                {"in_progress"},
		"work_place":            {"field"},
		"process_type":          {"visit"},
		"cause_type":            {"hw"},
		"action_taken":          {"부품 대기"},
		"time_spent":            {"30"},
		"result_code":           {model.ResultRevisit},
		"revisit_reason":        {"부품 수급"},
		"revisit_scheduled_date": {"2026-08-20"},
	})
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("재방문 저장 실패: %s", rec.Header().Get("Location"))
	}
	got, _ := asRepo.GetByID(asID)
	if got.CauseDetail != "전원 불량" || got.Conclusion != "이미 적어 둔 결론" {
		t.Fatalf("재방문이 서술을 지움: detail=%q conclusion=%q", got.CauseDetail, got.Conclusion)
	}
	if got.CauseType != "hw" {
		t.Fatalf("cause_type=%q", got.CauseType)
	}
}

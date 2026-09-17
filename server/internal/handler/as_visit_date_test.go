package handler

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func TestASActionPageShowsVisitDateWrap(t *testing.T) {
	e, _, _, _, asID := newASActionFixture(t)
	body := getASActionPage(t, e, asID)
	if !strings.Contains(body, `id="as-visit-date-wrap"`) || !strings.Contains(body, `name="visit_date"`) {
		t.Fatal("방문일 칸이 없다")
	}
	if !strings.Contains(body, `id="as-process-type-reason-wrap"`) {
		t.Fatal("미정 사유 칸이 없다")
	}
	idx := strings.Index(body, `id="as-visit-date-wrap"`)
	chunk := body[idx : idx+80]
	if !strings.Contains(chunk, "hidden") {
		t.Fatalf("처리유형 비어 있는데 방문일이 보인다: %s", chunk)
	}
}

func TestASActionRemoteHidesVisitDateAndSavesWithoutIt(t *testing.T) {
	e, _, asRepo, _, asID := newASActionFixture(t)
	rec := postASAction(t, e, asID, url.Values{
		"status":       {"in_progress"},
		"work_place":   {"office"},
		"process_type": {"remote"},
		"cause_type":   {"hw"},
		"action_taken": {"원격 조치"},
		"time_spent":   {"30"},
	})
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("원격 저장 거절: loc=%s", rec.Header().Get("Location"))
	}
	got, err := asRepo.GetByID(asID)
	if err != nil || got == nil {
		t.Fatal(err)
	}
	if got.VisitDate != "" {
		t.Fatalf("원격인데 방문일이 저장됐다: %q", got.VisitDate)
	}
	if got.StartDatetime == nil || got.StartDatetime.IsZero() {
		t.Fatal("착수일시는 그대로 채워져야 한다")
	}
	page := getASActionPage(t, e, asID)
	idx := strings.Index(page, `id="as-visit-date-wrap"`)
	if idx < 0 || !strings.Contains(page[idx:idx+90], "hidden") {
		t.Fatal("원격 저장 후 방문일 칸이 숨겨지지 않았다")
	}
}

func TestASActionVisitRequiresVisitDate(t *testing.T) {
	e, _, asRepo, _, asID := newASActionFixture(t)
	rec := postASAction(t, e, asID, url.Values{
		"status":       {"in_progress"},
		"work_place":   {"field"},
		"process_type": {"visit"},
		"visit_date":   {""},
		"cause_type":   {"hw"},
		"action_taken": {"현장 점검"},
		"time_spent":   {"30"},
		"result_code":  {model.ResultDone},
	})
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || !strings.Contains(loc, "err=visit_date") {
		t.Fatalf("방문일 누락이 거절되지 않았다: loc=%s", loc)
	}
	got, _ := asRepo.GetByID(asID)
	if got != nil && got.Status == "completed" {
		t.Fatal("방문일 없이 완료 저장")
	}
}

func TestASActionUndeterminedNeedsReason(t *testing.T) {
	e, _, asRepo, _, asID := newASActionFixture(t)
	rec := postASAction(t, e, asID, url.Values{
		"status":       {"in_progress"},
		"work_place":   {"office"},
		"process_type": {"undetermined"},
		"cause_type":   {"hw"},
		"action_taken": {"유형 미정"},
		"time_spent":   {"30"},
		"result_code":  {model.ResultDone},
	})
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || !strings.Contains(loc, "err=process_type_reason") {
		t.Fatalf("미정 사유 누락이 거절되지 않았다: loc=%s", loc)
	}

	rec = postASAction(t, e, asID, url.Values{
		"status":              {"in_progress"},
		"work_place":          {"office"},
		"process_type":        {"undetermined"},
		"process_type_reason": {"현장에서 확인 후 정함"},
		"cause_type":          {"hw"},
		"action_taken":        {"유형 미정"},
		"time_spent":          {"30"},
		"result_code":         {model.ResultDone},
	})
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("미정+사유 저장 거절: loc=%s", rec.Header().Get("Location"))
	}
	got, err := asRepo.GetByID(asID)
	if err != nil || got == nil {
		t.Fatal(err)
	}
	if got.ProcessType != model.ProcessTypeUndetermined || got.ProcessTypeReason != "현장에서 확인 후 정함" {
		t.Fatalf("미정 저장: type=%s reason=%s", got.ProcessType, got.ProcessTypeReason)
	}
	if got.VisitDate != "" {
		t.Fatalf("미정인데 방문일=%q", got.VisitDate)
	}
	if got.StartDatetime == nil {
		t.Fatal("착수일시가 지워졌다")
	}
}

func TestASActionKeepsStartDatetimeWhenVisitDateSaved(t *testing.T) {
	e, _, asRepo, _, asID := newASActionFixture(t)
	rec := postASAction(t, e, asID, url.Values{
		"status":       {"in_progress"},
		"work_place":   {"field"},
		"process_type": {"visit"},
		"visit_date":   {"2026-08-12"},
		"cause_type":   {"hw"},
		"action_taken": {"현장 방문"},
		"time_spent":   {"30"},
	})
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("저장 실패: loc=%s", rec.Header().Get("Location"))
	}
	got, err := asRepo.GetByID(asID)
	if err != nil || got == nil {
		t.Fatal(err)
	}
	if got.VisitDate != "2026-08-12" {
		t.Fatalf("visit_date=%q", got.VisitDate)
	}
	if got.StartDatetime == nil || got.StartDatetime.IsZero() {
		t.Fatal("start_datetime 이 비었다")
	}
	page := getASActionPage(t, e, asID)
	idx := strings.Index(page, `id="as-visit-date-wrap"`)
	if idx < 0 || strings.Contains(page[idx:idx+90], "hidden") {
		t.Fatal("현장방문인데 방문일 칸이 숨겨져 있다")
	}
}

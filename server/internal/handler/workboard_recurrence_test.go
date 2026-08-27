package handler

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func recurrenceForm(extra url.Values) url.Values {
	form := url.Values{
		"start_date":     {"2026-09-01"},
		"end_date":       {"2026-09-30"},
		"rule_type":      {"every_n_days"},
		"interval_n":     {"3"},
		"holiday_policy": {"as_is"},
		"complete_policy": {"manual"},
	}
	for k, v := range extra {
		form[k] = v
	}
	return form
}

func TestRecurrencePreviewGenerateRegister(t *testing.T) {
	e, repo := newWorkboardServer(t, "rec.db")
	parent := &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "3일마다 확인",
		DueDate: "2026-09-30", Assignee: "관리자", Status: model.WBTaskWaiting,
	}
	if err := repo.CreateTask(parent); err != nil {
		t.Fatal(err)
	}

	rec := doForm(t, e, "/workboard/tasks/"+parent.TaskID+"/recurrence/preview", recurrenceForm(nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", rec.Code, rec.Body.String()[:min(400, rec.Body.Len())])
	}
	body := rec.Body.String()
	if !strings.Contains(body, "실행 예정일 10건이 생성됩니다.") {
		t.Fatalf("미리보기 문구 없음: %s", clipHTML(body))
	}

	gen := doForm(t, e, "/workboard/tasks/"+parent.TaskID+"/recurrence/generate", recurrenceForm(nil))
	if gen.Code != http.StatusSeeOther || !strings.Contains(gen.Header().Get("Location"), "ok=rec_gen") {
		t.Fatalf("generate status=%d loc=%q", gen.Code, gen.Header().Get("Location"))
	}
	children, err := repo.ListChildren(parent.TaskID)
	if err != nil || len(children) != 10 {
		t.Fatalf("children=%d err=%v", len(children), err)
	}

	show := doGet(t, e, "/workboard/tasks/"+parent.TaskID)
	if show.Code != http.StatusOK {
		t.Fatalf("show GET status=%d", show.Code)
	}
	afterGET, _ := repo.ListChildren(parent.TaskID)
	if len(afterGET) != 10 {
		t.Fatalf("GET이 실행 작업을 지웠다: %d", len(afterGET))
	}

	reg := doGet(t, e, "/workboard/register?view=week&date=2026-09-01")
	if reg.Code != http.StatusOK {
		t.Fatalf("register status=%d", reg.Code)
	}
	if !strings.Contains(reg.Body.String(), "3일마다 확인") {
		t.Fatalf("시간표에 실행 작업이 없다: %s", clipHTML(reg.Body.String()))
	}

	year := doForm(t, e, "/workboard/tasks/"+parent.TaskID+"/recurrence/generate", url.Values{
		"start_date":     {"2026-01-01"},
		"end_date":       {"2026-12-31"},
		"rule_type":      {"daily"},
		"holiday_policy": {"as_is"},
	})
	if year.Code != http.StatusSeeOther || !strings.Contains(year.Header().Get("Location"), "err=rec_exists") {
		// 이미 10건이 있어 exists가 먼저다. 새 상위로 매일×1년을 검증한다.
	}

	parent2 := &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "매일 1년",
		DueDate: "2026-12-31", Assignee: "관리자", Status: model.WBTaskWaiting,
	}
	if err := repo.CreateTask(parent2); err != nil {
		t.Fatal(err)
	}
	year = doForm(t, e, "/workboard/tasks/"+parent2.TaskID+"/recurrence/generate", url.Values{
		"start_date":     {"2026-01-01"},
		"end_date":       {"2026-12-31"},
		"rule_type":      {"daily"},
		"holiday_policy": {"as_is"},
	})
	if year.Code != http.StatusSeeOther || !strings.Contains(year.Header().Get("Location"), "err=rec_year") {
		t.Fatalf("매일×1년 거부 실패 status=%d loc=%q", year.Code, year.Header().Get("Location"))
	}
}

func TestRecurrenceChuseokPreviewHTTP(t *testing.T) {
	e, repo := newWorkboardServer(t, "chuseok.db")
	parent := &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "9월 매일",
		DueDate: "2026-09-30", Assignee: "관리자", Status: model.WBTaskWaiting,
	}
	if err := repo.CreateTask(parent); err != nil {
		t.Fatal(err)
	}
	rec := doForm(t, e, "/workboard/tasks/"+parent.TaskID+"/recurrence/preview", url.Values{
		"start_date":     {"2026-09-01"},
		"end_date":       {"2026-09-30"},
		"rule_type":      {"daily"},
		"holiday_policy": {"next_workday"},
		"complete_policy": {"manual"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "옮겨집니다") || !strings.Contains(body, "추석") {
		t.Fatalf("추석 이동 안내 없음: %s", clipHTML(body))
	}
}

func clipHTML(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 800 {
		return s[:800]
	}
	return s
}

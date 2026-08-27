package handler

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func recurrenceForm(extra url.Values) url.Values {
	form := url.Values{
		"start_date":      {"2026-09-01"},
		"end_date":        {"2026-09-30"},
		"rule_type":       {"every_n_days"},
		"interval_n":      {"3"},
		"holiday_policy":  {"as_is"},
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
		"start_date":      {"2026-09-01"},
		"end_date":        {"2026-09-30"},
		"rule_type":       {"daily"},
		"holiday_policy":  {"next_workday"},
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

func TestOccurrenceOverdueOnShowAndNextKept(t *testing.T) {
	e, repo := newWorkboardServer(t, "occ_overdue.db")
	today := time.Now().Format("2006-01-02")
	past := time.Now().AddDate(0, 0, -3).Format("2006-01-02")
	future := time.Now().AddDate(0, 0, 10).Format("2006-01-02")
	parent := &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "기한 반복", DueDate: future,
		Assignee: "관리자", Status: model.WBTaskWaiting,
	}
	if err := repo.CreateTask(parent); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GenerateOccurrences(parent, model.WorkRecurrence{
		StartDate: past, EndDate: future, RuleType: model.RecurrenceManual,
		CompletePolicy: model.CompletePolicyManual,
	}, []string{past, future}); err != nil {
		t.Fatal(err)
	}
	show := doGet(t, e, "/workboard/tasks/"+parent.TaskID)
	if show.Code != http.StatusOK {
		t.Fatalf("show status=%d", show.Code)
	}
	body := show.Body.String()
	if !strings.Contains(body, "미완료") {
		t.Fatalf("상세에 미완료가 없다: %s", clipHTML(body))
	}
	children, _ := repo.ListChildren(parent.TaskID)
	if len(children) != 2 {
		t.Fatalf("GET이 다음 회차를 지웠다 n=%d", len(children))
	}
	byDate := map[string]model.WorkTask{}
	for _, ch := range children {
		byDate[ch.WorkDate] = ch
	}
	if byDate[past].OccurrenceStatus != model.OccurrenceOverdue {
		t.Fatalf("과거=%s", byDate[past].OccurrenceStatus)
	}
	if byDate[future].OccurrenceStatus != model.OccurrenceScheduled || byDate[future].WorkDate != future {
		t.Fatalf("미래 회차가 바뀌었다 %+v", byDate[future])
	}
	_ = today

	open, _ := repo.CountOpenOccurrences(parent.TaskID)
	noConfirm := doForm(t, e, "/workboard/tasks/"+parent.TaskID+"/update", url.Values{
		"title": {parent.Title}, "due_date": {future}, "work_date": {past},
		"status": {"complete"}, "complete_note": {"상위 종결"}, "work_type": {"admin"},
		"priority": {"normal"}, "assignee": {"관리자"},
	})
	if noConfirm.Code != http.StatusSeeOther || !strings.Contains(noConfirm.Header().Get("Location"), "err=rec_open") {
		t.Fatalf("미완료 확인 없이 완료되면 안 됨 loc=%q open=%d", noConfirm.Header().Get("Location"), open)
	}
	confirm := doForm(t, e, "/workboard/tasks/"+parent.TaskID+"/update", url.Values{
		"title": {parent.Title}, "due_date": {future}, "work_date": {past},
		"status": {"complete"}, "complete_note": {"상위 종결"}, "work_type": {"admin"},
		"priority": {"normal"}, "assignee": {"관리자"}, "confirm_incomplete_occ": {"1"},
	})
	if confirm.Code != http.StatusSeeOther || strings.Contains(confirm.Header().Get("Location"), "err=") {
		t.Fatalf("확인 후 완료 loc=%q", confirm.Header().Get("Location"))
	}
	got, _ := repo.GetTask(parent.TaskID)
	if got.Status != model.WBTaskComplete {
		t.Fatalf("상위 상태=%s", got.Status)
	}
	left, _ := repo.GetTask(byDate[past].TaskID)
	if left == nil || left.OccurrenceStatus != model.OccurrenceOverdue {
		t.Fatalf("미완료 기록이 지워졌다 %+v", left)
	}
	if n, _ := repo.CountOccurrences(parent.TaskID); n != 2 {
		t.Fatalf("실행 작업 수=%d", n)
	}
}

func TestOccurrenceSkipAutoCompletesParent(t *testing.T) {
	e, repo := newWorkboardServer(t, "occ_auto.db")
	parent := &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "자동정책", DueDate: "2026-09-30",
		Assignee: "관리자", Status: model.WBTaskWaiting,
	}
	if err := repo.CreateTask(parent); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GenerateOccurrences(parent, model.WorkRecurrence{
		StartDate: "2026-09-01", EndDate: "2026-09-30", RuleType: model.RecurrenceManual,
		CompletePolicy: model.CompletePolicyAuto,
	}, []string{"2026-09-01", "2026-09-04"}); err != nil {
		t.Fatal(err)
	}
	chs, _ := repo.ListChildren(parent.TaskID)
	done := doForm(t, e, "/workboard/tasks/"+parent.TaskID+"/occurrences/"+chs[0].TaskID, url.Values{
		"occurrence_status": {"complete"},
	})
	if done.Code != http.StatusSeeOther {
		t.Fatalf("완료 status=%d", done.Code)
	}
	skip := doForm(t, e, "/workboard/tasks/"+parent.TaskID+"/occurrences/"+chs[1].TaskID, url.Values{
		"occurrence_status": {"skipped"},
		"not_done_reason":   {"해당 회차 없음"},
	})
	if skip.Code != http.StatusSeeOther {
		t.Fatalf("제외 status=%d loc=%q", skip.Code, skip.Header().Get("Location"))
	}
	got, _ := repo.GetTask(parent.TaskID)
	if got.Status != model.WBTaskComplete {
		t.Fatalf("auto 상위 상태=%s", got.Status)
	}
}

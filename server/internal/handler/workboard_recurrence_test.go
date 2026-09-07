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
		t.Fatalf("일정표에 실행 작업이 없다: %s", clipHTML(reg.Body.String()))
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

func TestRecurrenceChangeKeepsCompleteAndBlocksDelete(t *testing.T) {
	e, repo := newWorkboardServer(t, "occ_change.db")
	parent := &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "주기변경", DueDate: "2026-09-30",
		Assignee: "관리자", Status: model.WBTaskWaiting,
	}
	if err := repo.CreateTask(parent); err != nil {
		t.Fatal(err)
	}
	gen := doForm(t, e, "/workboard/tasks/"+parent.TaskID+"/recurrence/generate", recurrenceForm(nil))
	if gen.Code != http.StatusSeeOther || !strings.Contains(gen.Header().Get("Location"), "ok=rec_gen") {
		t.Fatalf("generate loc=%q", gen.Header().Get("Location"))
	}
	chs, _ := repo.ListChildren(parent.TaskID)
	keepID := chs[0].TaskID
	done := doForm(t, e, "/workboard/tasks/"+parent.TaskID+"/occurrences/"+keepID, url.Values{
		"occurrence_status": {"complete"},
	})
	if done.Code != http.StatusSeeOther {
		t.Fatalf("완료 status=%d", done.Code)
	}
	if err := repo.CreateActivity(&model.WorkActivity{TaskID: keepID, Content: "1회차 처리", Actor: "관리자"}); err != nil {
		t.Fatal(err)
	}

	form := recurrenceForm(url.Values{
		"interval_n":     {"7"},
		"change_mode":    {"future"},
		"start_date":     {"2026-09-01"},
		"end_date":       {"2026-09-30"},
		"rule_type":      {"every_n_days"},
		"holiday_policy": {"as_is"},
	})
	reg := doForm(t, e, "/workboard/tasks/"+parent.TaskID+"/recurrence/regenerate", form)
	if reg.Code != http.StatusSeeOther || strings.Contains(reg.Header().Get("Location"), "err=") {
		t.Fatalf("재생성 loc=%q", reg.Header().Get("Location"))
	}
	still, _ := repo.GetTask(keepID)
	if still == nil || still.OccurrenceStatus != model.OccurrenceComplete {
		t.Fatalf("완료 회차가 지워졌다 %+v", still)
	}
	acts, _ := repo.ListActivities(keepID)
	if len(acts) == 0 || acts[0].Content != "1회차 처리" {
		t.Fatalf("처리 기록이 없다 %+v", acts)
	}

	del := doForm(t, e, "/workboard/tasks/"+parent.TaskID+"/delete", url.Values{})
	if del.Code != http.StatusSeeOther || !strings.Contains(del.Header().Get("Location"), "err=rec_has_complete") {
		t.Fatalf("삭제 거부 loc=%q", del.Header().Get("Location"))
	}
	if got, _ := repo.GetTask(parent.TaskID); got == nil {
		t.Fatal("상위가 삭제됐다")
	}
	if got, _ := repo.GetTask(keepID); got == nil {
		t.Fatal("완료 회차가 삭제됐다")
	}

	getDel := doGet(t, e, "/workboard/tasks/"+parent.TaskID+"/delete")
	if getDel.Code == http.StatusSeeOther {
		t.Fatalf("GET 삭제가 동작하면 안 된다 status=%d loc=%q", getDel.Code, getDel.Header().Get("Location"))
	}
	if got, _ := repo.GetTask(parent.TaskID); got == nil {
		t.Fatal("GET이 상위를 지웠다")
	}

	noConfirm := doForm(t, e, "/workboard/tasks/"+parent.TaskID+"/recurrence/regenerate", recurrenceForm(url.Values{
		"change_mode": {"replace"},
		"interval_n":  {"7"},
	}))
	if noConfirm.Code != http.StatusSeeOther || !strings.Contains(noConfirm.Header().Get("Location"), "err=rec_replace") {
		t.Fatalf("replace 확인 없이 loc=%q", noConfirm.Header().Get("Location"))
	}
}

func TestRecurrenceUnitPreviewAndRegisterLabels(t *testing.T) {
	e, repo := newWorkboardServer(t, "rec_units.db")
	parent := &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "매월 확인",
		DueDate: "2026-11-30", Assignee: "관리자", Status: model.WBTaskWaiting,
	}
	if err := repo.CreateTask(parent); err != nil {
		t.Fatal(err)
	}
	rec := doForm(t, e, "/workboard/tasks/"+parent.TaskID+"/recurrence/preview", url.Values{
		"start_date":      {"2026-09-01"},
		"end_date":        {"2026-11-30"},
		"rule_type":       {"monthly"},
		"month_day":       {"15"},
		"holiday_policy":  {"as_is"},
		"complete_policy": {"manual"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("monthly preview status=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "실행 예정일 3건이 생성됩니다.") || !strings.Contains(body, "9/15") {
		t.Fatalf("매월 15일 미리보기 없음: %s", clipHTML(body))
	}

	last := doForm(t, e, "/workboard/tasks/"+parent.TaskID+"/recurrence/preview", url.Values{
		"start_date":      {"2026-10-01"},
		"end_date":        {"2026-10-31"},
		"rule_type":       {"monthly"},
		"monthly_mode":    {"last"},
		"holiday_policy":  {"as_is"},
		"complete_policy": {"manual"},
	})
	if last.Code != http.StatusOK || !strings.Contains(last.Body.String(), "10/30") {
		t.Fatalf("마지막 영업일 미리보기 없음: %s", clipHTML(last.Body.String()))
	}

	manual := doForm(t, e, "/workboard/tasks/"+parent.TaskID+"/recurrence/preview", url.Values{
		"rule_type":       {"manual"},
		"manual_dates":    {"2026-09-24\n2026-09-27"},
		"holiday_policy":  {"next_workday"},
		"complete_policy": {"manual"},
	})
	if manual.Code != http.StatusOK {
		t.Fatalf("manual preview status=%d", manual.Code)
	}
	mb := manual.Body.String()
	if strings.Contains(mb, "옮겨집니다") {
		t.Fatalf("지정일자 HTTP 미리보기가 날짜를 옮겼다: %s", clipHTML(mb))
	}
	if !strings.Contains(mb, "옮기지 않습니다") || !strings.Contains(mb, "9/24") {
		t.Fatalf("지정일자 경고·날짜 없음: %s", clipHTML(mb))
	}

	reg := doGet(t, e, "/workboard/register?view=week&date=2026-09-01")
	if reg.Code != http.StatusOK {
		t.Fatalf("register status=%d", reg.Code)
	}
	rb := reg.Body.String()
	if !strings.Contains(rb, "매월") || !strings.Contains(rb, "지정일자") {
		t.Fatalf("등록 화면에 매월·지정일자 없음: %s", clipHTML(rb))
	}
}

func TestCreateTaskMonthlyRecurrenceFromForm(t *testing.T) {
	e, repo := newWorkboardServer(t, "rec_create.db")
	rec := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type":          {"admin"},
		"title":              {"매월 15일 보고"},
		"due_date":           {"2026-11-30"},
		"work_date":          {"2026-09-01"},
		"start_time":         {"09:00"},
		"end_time":           {"10:00"},
		"assignee":           {"관리자"},
		"recurrence_enabled": {"1"},
		"start_date":         {"2026-09-01"},
		"end_date":           {"2026-11-30"},
		"rule_type":          {"monthly"},
		"month_day":          {"15"},
		"holiday_policy":     {"as_is"},
		"complete_policy":    {"manual"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "ok=rec_gen") {
		t.Fatalf("등록+반복 loc=%q", rec.Header().Get("Location"))
	}
	tasks, err := repo.ListTasks()
	if err != nil || len(tasks) == 0 {
		t.Fatalf("tasks=%d err=%v", len(tasks), err)
	}
	var parent *model.WorkTask
	for i := range tasks {
		if tasks[i].Title == "매월 15일 보고" && tasks[i].ParentTaskID == "" {
			parent = &tasks[i]
			break
		}
	}
	if parent == nil {
		t.Fatal("상위 업무가 없다")
	}
	children, err := repo.ListChildren(parent.TaskID)
	if err != nil || len(children) != 3 {
		t.Fatalf("children=%d err=%v", len(children), err)
	}
	rule, err := repo.GetRecurrence(parent.TaskID)
	if err != nil || rule == nil || rule.RuleType != model.RecurrenceMonthly || rule.MonthDay != 15 {
		t.Fatalf("rule=%+v err=%v", rule, err)
	}
}

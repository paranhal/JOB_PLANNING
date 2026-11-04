package handler

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func Test95_116_ProjectWorkActionErrorKeepsInputThenGETUsesDB(t *testing.T) {
	e, repo, _ := newProjectWorkServer(t)
	id := pwCreateTask(t, e, repo, "116오류유지")
	const draft = "116에서만 쓰는 조치 초안입니다"
	form := pwActionForm()
	form.Del("work_place")
	form.Set("action_taken", draft)
	form.Set("work_subtype", "build")
	rec := doForm(t, e, "/workboard/tasks/"+id+"/action", form)
	if rec.Code != http.StatusOK {
		t.Fatalf("오류 재표시 code=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "근무 구분을 선택하세요") {
		t.Fatal("근무구분 오류 없음")
	}
	if !strings.Contains(body, draft) {
		t.Fatal("오류 화면에 조치 초안이 없다")
	}
	if !strings.Contains(body, `value="build"`) || !strings.Contains(body, `name="work_subtype"`) {
		t.Fatal("작업 구분 미유지")
	}
	if !strings.Contains(body, `value="2026-10-21T08:00"`) {
		t.Fatal("시작 일시 미유지")
	}
	if !strings.Contains(body, `value="2026-10-21T15:00"`) {
		t.Fatal("완료 일시 미유지")
	}

	page := doGet(t, e, "/workboard/tasks/"+id+"/action")
	if page.Code != http.StatusOK {
		t.Fatalf("GET action %d", page.Code)
	}
	if strings.Contains(page.Body.String(), draft) {
		t.Fatal("오류 없이 다시 열었는데 미저장 초안이 남아 있다")
	}
	tk, _ := repo.GetTask(id)
	_ = repo.LoadTaskActionFields(tk)
	if strings.TrimSpace(tk.ActionTaken) != "" {
		t.Fatalf("거절된 조치가 DB에 저장됨 %q", tk.ActionTaken)
	}
}

func Test95_116_ASActionErrorKeepsDraft(t *testing.T) {
	e, _, _, _, asID := newASActionFixture(t)
	form := as96SetForm()
	form.Set("s1_cause_cat1", "")
	form.Set("s1_cause_cat2", "")
	form.Set("s1_action_taken", "AS116 오류 재표시 초안입니다")
	rec := postASAction(t, e, asID, form)
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || !strings.Contains(loc, "err=") {
		t.Fatalf("AS 오류 리다이렉트 loc=%s code=%d", loc, rec.Code)
	}
	page := getASActionQuery(t, e, loc)
	if !strings.Contains(page, "AS116 오류 재표시 초안입니다") {
		t.Fatal("AS 조치 오류 후 입력이 사라졌다")
	}
}

func Test95_116_MntVisitActionErrorKeepsNotes(t *testing.T) {
	e, db, _ := newMnt47Server(t)
	seedMnt47Visit(t, db, "v116k")
	bad := doMnt47Form(t, e, "/maintenance/visits/v116k/action", url.Values{
		"assignee":   {"최혜영"},
		"visit_date": {"2026-10-12"},
		"status":     {"open"},
		"notes":      {"116 보존 비고"},
		"start_time": {"11:30"},
		"end_time":   {"10:00"},
	}, nil)
	if bad.Code != http.StatusOK {
		t.Fatalf("정기점검 오류 재표시 %d", bad.Code)
	}
	if !strings.Contains(bad.Body.String(), "116 보존 비고") {
		t.Fatal("정기점검 오류 후 비고가 사라졌다")
	}
}

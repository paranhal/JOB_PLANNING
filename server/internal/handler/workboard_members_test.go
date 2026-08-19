package handler

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func TestCreateTaskWithTwoSupportsMakesThreeMembers(t *testing.T) {
	e, repo := newWorkboardServer(t, "members.db")
	today := "2026-08-18"
	rec := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type":   {"admin"},
		"title":       {"CRM 자산정비"},
		"due_date":    {today},
		"work_date":   {today},
		"start_time":  {"09:00"},
		"end_time":    {"11:00"},
		"status":      {"waiting"},
		"priority":    {"normal"},
		"assignee":    {"최혜영"},
		"member_name": {"양기헌", "태자운"},
		"member_min":  {"120", "60"},
		"redirect":    {"/workboard/register"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "ok=task") {
		t.Fatalf("등록: status=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}
	tasks, err := repo.ListTasks()
	if err != nil || len(tasks) != 1 {
		t.Fatalf("tasks=%d err=%v", len(tasks), err)
	}
	ms, err := repo.ListMembers(tasks[0].TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 3 {
		t.Fatalf("members=%d want 3 %+v", len(ms), ms)
	}
	var owner, support int
	for _, m := range ms {
		if m.Role == model.WBMemberOwner {
			owner++
			if m.Assignee != "최혜영" {
				t.Fatalf("owner=%q", m.Assignee)
			}
		}
		if m.Role == model.WBMemberSupport {
			support++
		}
	}
	if owner != 1 || support != 2 {
		t.Fatalf("owner=%d support=%d", owner, support)
	}
}

func TestCreateTaskSupportOverlapRedirectsWithNameAndTime(t *testing.T) {
	e, repo := newWorkboardServer(t, "overlap.db")
	today := "2026-08-18"
	rec := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type":  {"admin"},
		"title":      {"기존"},
		"due_date":   {today},
		"work_date":  {today},
		"start_time": {"09:00"},
		"end_time":   {"11:00"},
		"assignee":   {"최혜영"},
		"redirect":   {"/workboard/register"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("첫번째: %d", rec.Code)
	}
	tasks, _ := repo.ListTasks()
	if len(tasks) != 1 {
		t.Fatalf("tasks=%d", len(tasks))
	}
	if err := repo.ReplaceSupportMembers(tasks[0].TaskID, []model.WorkTaskMember{
		{Assignee: "양기헌", Role: model.WBMemberSupport},
	}); err != nil {
		t.Fatal(err)
	}

	rec = doForm(t, e, "/workboard/tasks", url.Values{
		"work_type":   {"admin"},
		"title":       {"겹침"},
		"due_date":    {today},
		"work_date":   {today},
		"start_time":  {"10:00"},
		"end_time":    {"10:30"},
		"assignee":    {"태자운"},
		"member_name": {"양기헌"},
		"member_min":  {""},
		"redirect":    {"/workboard/register"},
	})
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || !strings.Contains(loc, "err=") {
		t.Fatalf("겹침 거부 아님: %d %s", rec.Code, loc)
	}
	got := urlUnescape(loc)
	if !strings.Contains(got, "양기헌의 10:00~10:30에 다른 업무가 있습니다") {
		t.Fatalf("오류 문구 없음: %s decoded=%s", loc, got)
	}
}

func urlUnescape(s string) string {
	if i := strings.Index(s, "err="); i >= 0 {
		s = s[i+4:]
	}
	out, err := url.QueryUnescape(s)
	if err != nil {
		return s
	}
	return out
}

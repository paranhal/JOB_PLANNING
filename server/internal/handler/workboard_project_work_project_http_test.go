package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"customer-support/internal/repository"
)

func TestProjectWorkProjectIDPersistsAndShows(t *testing.T) {
	e, repo, users := newProjectWorkServer(t)

	reg := doGet(t, e, "/workboard/register?view=day")
	body := reg.Body.String()
	for _, name := range []string{"도서관공사", "SI구축", "보류사업"} {
		if !strings.Contains(body, name) {
			t.Fatalf("등록 팝업 사업 목록에 %q 없음", name)
		}
	}
	if strings.Contains(body, "종료사업") || strings.Contains(body, "보관사업") {
		t.Fatal("완료·보관 사업이 선택 목록에 있다")
	}

	ok := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type":    {"project_work"},
		"title":        {"사업연결작업"},
		"work_subtype": {"construction"},
		"work_date":    {"2026-10-21"},
		"assignee":     {"관리자"},
		"project_id":   {"WP-O01"},
		"site_id":      {"C-high"},
		"start_time":   {"14:00"},
		"end_time":     {"14:30"},
	})
	if ok.Code != http.StatusSeeOther {
		t.Fatalf("저장 %d %s", ok.Code, ok.Body.String())
	}
	id := pwFindTitle(t, repo, "사업연결작업")
	got, err := repo.GetTask(id)
	if err != nil || got == nil {
		t.Fatal(err)
	}
	if got.ProjectID != "WP-O01" {
		t.Fatalf("project_id=%q (저장 후 비어 있음)", got.ProjectID)
	}
	if got.ProjectName != "도서관공사" {
		t.Fatalf("ProjectName=%q", got.ProjectName)
	}

	cal := doGet(t, e, "/workboard/register?view=day&date=2026-10-21")
	if !strings.Contains(cal.Body.String(), "도서관공사") {
		t.Fatal("일정표 카드에 사업명 없음")
	}
	if !strings.Contains(cal.Body.String(), "/workboard/tasks/"+id+"/action") {
		t.Fatal("일정표 조치가 /action 이 아님")
	}
	show := doGet(t, e, "/workboard/tasks/"+id)
	if !strings.Contains(show.Body.String(), "도서관공사") {
		t.Fatal("상세에 사업명 없음")
	}
	act := doGet(t, e, "/workboard/tasks/"+id+"/action")
	if !strings.Contains(act.Body.String(), "도서관공사") {
		t.Fatal("조치 화면에 사업명 없음")
	}

	none := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type": {"project_work"}, "title": {"사업없음"}, "work_subtype": {"other"},
		"work_date": {"2026-10-21"}, "assignee": {"관리자"},
		"start_time": {"15:00"}, "end_time": {"15:30"},
	})
	if none.Code != http.StatusSeeOther {
		t.Fatalf("사업 없이 %d", none.Code)
	}
	idNone := pwFindTitle(t, repo, "사업없음")
	tn, _ := repo.GetTask(idNone)
	if tn.ProjectID != "" {
		t.Fatalf("빈 사업이 %q", tn.ProjectID)
	}
	actNone := doGet(t, e, "/workboard/tasks/"+idNone+"/action")
	if !strings.Contains(actNone.Body.String(), "사업 없음") {
		t.Fatal("조치 화면에 「사업 없음」 없음")
	}

	cross := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type": {"project_work"}, "title": {"타조직사업"}, "work_subtype": {"build"},
		"work_date": {"2026-10-21"}, "assignee": {"관리자"}, "project_id": {"WP-O03"},
		"start_time": {"16:00"}, "end_time": {"16:30"},
	})
	if cross.Code != http.StatusSeeOther {
		t.Fatalf("타 조직 사업 저장 status=%d (95-9A Q3 허용)", cross.Code)
	}
	idX := pwFindTitle(t, repo, "타조직사업")
	tx, _ := repo.GetTask(idX)
	if tx.ProjectID != "WP-O03" || tx.ProjectName != "SI구축" {
		t.Fatalf("타 조직 사업 project=%s name=%s", tx.ProjectID, tx.ProjectName)
	}

	hold := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type": {"project_work"}, "title": {"보류사업작업"}, "work_subtype": {"build"},
		"work_date": {"2026-10-21"}, "assignee": {"관리자"}, "project_id": {"WP-hold"},
		"start_time": {"17:00"}, "end_time": {"17:30"},
	})
	if hold.Code != http.StatusSeeOther {
		t.Fatalf("hold 사업 %d", hold.Code)
	}
	if g, _ := repo.GetTask(pwFindTitle(t, repo, "보류사업작업")); g.ProjectID != "WP-hold" {
		t.Fatalf("hold project=%s", g.ProjectID)
	}

	oa04, err := users.GetByUsername("oa04")
	if err != nil || oa04 == nil {
		t.Fatal(err)
	}
	empty := httptest.NewRequest(http.MethodGet, "/workboard/register?view=day", nil)
	empty.AddCookie(jwtCookieUser(t, oa04))
	emptyRec := httptest.NewRecorder()
	e.ServeHTTP(emptyRec, empty)
	if emptyRec.Code != http.StatusOK {
		t.Fatalf("O04 register %d", emptyRec.Code)
	}
	if strings.Contains(emptyRec.Body.String(), "등록된 사업이 없습니다") {
		t.Fatal("조직 무관 목록인데 빈 안내가 나옴")
	}
	if !strings.Contains(emptyRec.Body.String(), "도서관공사") {
		t.Fatal("O04도 도서관공사 후보를 봐야 한다")
	}
}

func pwFindTitle(t *testing.T, repo *repository.WBRepo, title string) string {
	t.Helper()
	tasks, err := repo.ListTasks()
	if err != nil {
		t.Fatal(err)
	}
	for _, tk := range tasks {
		if tk.Title == title {
			return tk.TaskID
		}
	}
	t.Fatalf("task %s 없음", title)
	return ""
}

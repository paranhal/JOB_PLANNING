package handler

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"customer-support/internal/model"
)

var slot198 int

func time198(n int) string { return string(rune('0'+(13+n)/10)) + string(rune('0'+(13+n)%10)) + ":00" }

// 95-198: 업무 등록·수정 화면의 활동 분류 입력칸.
func createForm198(v env191, title, kind string) url.Values {
	slot198++
	st := time198(slot198)
	f := url.Values{"work_type": {"admin"}, "title": {title}, "due_date": {"2026-11-12"}, "work_date": {"2026-11-12"},
		"start_time": {st}, "end_time": {st[:3] + "30"}, "duration_min": {"30"}, "assignee": {v.owner.FullName}}
	if kind != "" {
		f.Set("activity_kind", kind)
	}
	return f
}

func taskIDByTitle198(t *testing.T, v env191, title string) string {
	t.Helper()
	tasks, _ := v.wb.ListTasks()
	for _, tk := range tasks {
		if tk.Title == title {
			return tk.TaskID
		}
	}
	return ""
}

func TestHTTP95_198_CreateFormHasField(t *testing.T) {
	v := setup191(t)
	rr := v.do("GET", "/workboard/register?kind=admin", nil, v.admin)
	body := rr.Body.String()
	t.Logf("form %d len=%d", rr.Code, len(body))
	if !strings.Contains(body, "data-activity-kind-select") {
		t.Fatal("등록 화면에 활동 분류 칸이 없다")
	}
	for _, want := range []string{`value="mgmt"`, `value="tech"`, `value="strategy"`, `value="asset"`, `value="biz_support"`, `value="service"`, `value="growth"`, "경영지원", "역량강화"} {
		if !strings.Contains(body, want) {
			t.Fatalf("%s 없음", want)
		}
	}
}

func TestHTTP95_198_CreateSavesKind(t *testing.T) {
	v := setup191(t)
	r := v.do("POST", "/workboard/tasks", createForm198(v, "활동 분류 등록 업무", "strategy"), v.admin)
	if r.Code != http.StatusSeeOther || strings.Contains(r.Header().Get("Location"), "err=") {
		t.Fatalf("등록 %d %s", r.Code, r.Header().Get("Location"))
	}
	id := taskIDByTitle198(t, v, "활동 분류 등록 업무")
	if id == "" || v.wb.TaskActivityKind(id) != "strategy" {
		t.Fatalf("분류 저장 id=%q kind=%q", id, v.wb.TaskActivityKind(id))
	}
	// 입력하지 않으면 비어 있다
	rr := v.do("POST", "/workboard/tasks", createForm198(v, "분류 없는 업무", ""), v.admin)
	t.Logf("create %d %s", rr.Code, rr.Header().Get("Location"))
	if id2 := taskIDByTitle198(t, v, "분류 없는 업무"); id2 == "" || v.wb.TaskActivityKind(id2) != "" {
		t.Fatalf("분류 없는 업무에 값 %q", v.wb.TaskActivityKind(id2))
	}
	// 알 수 없는 값은 거절
	if r := v.do("POST", "/workboard/tasks", createForm198(v, "이상한 분류", "nope"), v.admin); r.Code != http.StatusBadRequest {
		t.Fatalf("잘못된 분류 %d", r.Code)
	}
	if taskIDByTitle198(t, v, "이상한 분류") != "" {
		t.Fatal("잘못된 분류인데 업무가 만들어졌다")
	}
}

func TestHTTP95_198_EditShowsAndChanges(t *testing.T) {
	v := setup191(t)
	if err := func() error { _, _, e := v.wb.SetTaskActivity(model.OrgIDLibrary, v.progress, "service"); return e }(); err != nil {
		t.Fatal(err)
	}
	edit := v.do("GET", "/workboard/tasks/"+v.progress+"/edit", nil, v.admin).Body.String()
	if !strings.Contains(edit, "data-activity-kind-select") || !strings.Contains(edit, `value="service" selected`) {
		t.Fatal("수정 화면에 현재 활동 분류가 선택돼 있지 않다")
	}
	if !strings.Contains(v.do("GET", "/workboard/tasks/"+v.progress, nil, v.admin).Body.String(), "data-activity-label") {
		t.Fatal("상세에 활동 분류 표시가 없다")
	}
	form := url.Values{"scope": {"register"}, "title": {"진행 업무"}, "status": {model.WBTaskInProgress},
		"assignee": {v.owner.FullName}, "work_date": {"2026-11-10"}, "due_date": {"2026-11-10"}, "start_time": {"11:00"}, "end_time": {"11:30"},
		"duration_min": {"30"}, "work_type": {model.WBWorkAdmin}, "activity_kind": {"growth"}}
	if r := v.do("POST", "/workboard/tasks/"+v.progress+"/update", form, v.admin); r.Code != http.StatusSeeOther || strings.Contains(r.Header().Get("Location"), "err=") {
		t.Fatalf("저장 %d %s", r.Code, r.Header().Get("Location"))
	}
	if k := v.wb.TaskActivityKind(v.progress); k != "growth" {
		t.Fatalf("변경 뒤 %q", k)
	}
	// 비우면 해제
	form.Set("activity_kind", "")
	v.do("POST", "/workboard/tasks/"+v.progress+"/update", form, v.admin)
	if k := v.wb.TaskActivityKind(v.progress); k != "" {
		t.Fatalf("해제 뒤 %q", k)
	}
}

func TestHTTP95_198_ProjectWinsOverActivity(t *testing.T) {
	v := setup191(t)
	db := v.wb.DB()
	if _, err := db.Exec(`INSERT INTO work_projects(project_id,name,status,org_id) VALUES ('WPT198','우선 사업','active','O01')`); err != nil {
		t.Fatal(err)
	}
	f := createForm198(v, "사업과 활동 함께", "mgmt")
	f.Set("project_id", "WPT198")
	if r := v.do("POST", "/workboard/tasks", f, v.admin); r.Code != http.StatusSeeOther {
		t.Fatalf("등록 %d %s", r.Code, r.Header().Get("Location"))
	}
	id := taskIDByTitle198(t, v, "사업과 활동 함께")
	if id == "" || v.wb.TaskActivityKind(id) != "" {
		t.Fatalf("사업이 있는데 활동 분류가 남았다 id=%q kind=%q", id, v.wb.TaskActivityKind(id))
	}
	if got, _ := v.wb.GetTask(id); got == nil || got.ProjectID != "WPT198" {
		t.Fatalf("사업이 저장되지 않았다 %+v", got)
	}
}

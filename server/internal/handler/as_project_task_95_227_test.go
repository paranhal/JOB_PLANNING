package handler

import (
	"net/http"
	"net/url"
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-227 HTTP: 접수 화면에서 사업을 연결·해제하면 그 접수의 업무에 바로 반영되고, 새로 만든 업무는 접수의 사업을 가진다.
func Test95_227_ASProjectFlowsToTasks(t *testing.T) {
	e, db, _ := newAS151Server(t)
	seed151Projects(t, db)
	insert151AS(t, db, "R227-a", "C151", "in_progress", "", "", false)
	ck := as151OA(t, db)
	wb := repository.NewWBRepo(db)
	mk := func(src, title string) string {
		tk := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: title, Status: model.WBTaskWaiting, Priority: model.WBPriorityNormal,
			DueDate: "2026-10-21", WorkDate: "2026-10-21", OrgID: model.OrgIDLibrary, SourceType: model.WBSourceAS, SourceID: src}
		if err := wb.CreateTask(tk); err != nil {
			t.Fatal(err)
		}
		return tk.TaskID
	}
	taskProject := func(id string) string {
		var p string
		_ = db.QueryRow(`SELECT COALESCE(project_id,'') FROM work_tasks WHERE task_id=?`, id).Scan(&p)
		return p
	}
	first := mk("R227-a", "[AS]성환도서관_R227-a")
	if taskProject(first) != "" {
		t.Fatalf("원본에 사업이 없는데 업무에 %q", taskProject(first))
	}
	if r := doAS151Form(t, e, "/workboard/projects/link-site", url.Values{"confirm": {"1"}, "project_id": {"WP-A"}, "customer_id": {"C151"}}, ck); r.Code != http.StatusOK {
		t.Fatalf("link %d %s", r.Code, r.Body.String())
	}
	if r := doAS151Form(t, e, "/as/R227-a/project", url.Values{"project_id": {"WP-A"}}, ck); r.Code != http.StatusSeeOther {
		t.Fatalf("사업 연결 %d %s", r.Code, r.Body.String())
	}
	if taskProject(first) != "WP-A" {
		t.Fatalf("접수에서 사업을 연결했는데 업무에 반영 안 됨 %q", taskProject(first))
	}
	// 사업이 이미 있는 접수에서 새로 만든 업무는 만들 때부터 접수의 사업을 가진다
	insert151AS(t, db, "R227-b", "C151", "in_progress", "", "WP-A", false)
	if got := taskProject(mk("R227-b", "[AS]성환도서관_R227-b")); got != "WP-A" {
		t.Fatalf("새 업무가 접수의 사업을 못 받았다 %q", got)
	}
	// 해제하면 업무도 비워진다
	if r := doAS151Form(t, e, "/as/R227-a/project", url.Values{"project_id": {""}}, ck); r.Code != http.StatusSeeOther {
		t.Fatalf("사업 해제 %d %s", r.Code, r.Body.String())
	}
	if taskProject(first) != "" {
		t.Fatalf("접수에서 사업을 해제했는데 업무에 남음 %q", taskProject(first))
	}
}

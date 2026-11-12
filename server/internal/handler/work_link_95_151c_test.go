package handler

import (
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 사업 연결은 소속만 바꾸므로 완료·취소 업무도 잠금 해제 없이 연결된다(AS 와 같은 규칙, 95-151B/C).

// 주간 보고서의 「사업 연결」: 완료·취소 일반 업무.
func Test95_151C_WeeklyLinkCompletedTaskWithoutUnlock(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "w151c.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	seedWeekly32(t, db)
	if _, err := db.Exec(`UPDATE work_tasks SET status='complete', complete_date='2026-08-12' WHERE task_id='T-NP32'`); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	mountWeekly28(e, h)
	ck := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")

	rec := postWeeklyLink(t, e, url.Values{
		"kind": {"work_task"}, "id": {"T-NP32"}, "project_id": {"WP-P1"},
		"org": {"O03"}, "week": {"2026-08-10"}, "assignee_keep": {"1"},
	}, ck)
	loc, _ := url.QueryUnescape(rec.Header().Get("Location"))
	if rec.Code != http.StatusSeeOther || !strings.Contains(loc, "연결했습니다") || strings.Contains(loc, "잠금") {
		t.Fatalf("완료 업무 연결 %d %s", rec.Code, loc)
	}
	var pid string
	if err := db.QueryRow(`SELECT COALESCE(project_id,'') FROM work_tasks WHERE task_id='T-NP32'`).Scan(&pid); err != nil || pid != "WP-P1" {
		t.Fatalf("project_id=%q err=%v", pid, err)
	}
	// 취소 건도 같다(다른 사업으로 바꿔 본다)
	if _, err := db.Exec(`UPDATE work_tasks SET status='cancelled' WHERE task_id='T-NP32'`); err != nil {
		t.Fatal(err)
	}
	rec = postWeeklyLink(t, e, url.Values{
		"kind": {"work_task"}, "id": {"T-NP32"}, "project_id": {"WP-P2"},
		"org": {"O03"}, "week": {"2026-08-10"}, "assignee_keep": {"1"},
	}, ck)
	loc, _ = url.QueryUnescape(rec.Header().Get("Location"))
	if rec.Code != http.StatusSeeOther || strings.Contains(loc, "잠금") {
		t.Fatalf("취소 업무 연결 %d %s", rec.Code, loc)
	}
	// 그래도 권한 확인은 남는다: 타 조직 업무는 404
	if _, err := db.Exec(`UPDATE work_tasks SET org_id=? WHERE task_id='T-NP32'`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	rec = postWeeklyLink(t, e, url.Values{
		"kind": {"work_task"}, "id": {"T-NP32"}, "project_id": {"WP-P1"},
		"org": {"O03"}, "week": {"2026-08-10"}, "assignee_keep": {"1"},
	}, ck)
	loc, _ = url.QueryUnescape(rec.Header().Get("Location"))
	if strings.Contains(loc, "연결했습니다") {
		t.Fatalf("타 조직 업무가 연결됐다: %s", loc)
	}
}

// 사업 업무 조치 화면의 「사업 연결」: 완료·종결 건.
func Test95_151C_ProjectWorkActionLinkClosedTask(t *testing.T) {
	e, repo, _ := newProjectWorkServer(t)
	id := pwCreateTask(t, e, repo, "완료연결대상")
	tk, _ := repo.GetTask(id)
	if tk.ProjectID != "" {
		t.Fatal("처음엔 사업 없음이어야 함")
	}
	if _, err := repo.DB().Exec(`UPDATE work_tasks SET status='complete', complete_date='2026-08-12' WHERE task_id=?`, id); err != nil {
		t.Fatal(err)
	}
	ok := doForm(t, e, "/workboard/tasks/"+id+"/action/project", url.Values{"project_id": {"WP-O01"}})
	if ok.Code != http.StatusSeeOther {
		t.Fatalf("완료 건 연결 %d", ok.Code)
	}
	if loc := ok.Header().Get("Location"); strings.Contains(loc, "locked") || strings.Contains(loc, "err=") {
		t.Fatalf("잠금으로 거절됐다: %s", loc)
	}
	tk, _ = repo.GetTask(id)
	if tk.ProjectID != "WP-O01" {
		t.Fatalf("완료 건이 연결되지 않았다: %q", tk.ProjectID)
	}
	// 조치 내용 수정은 여전히 잠겨 있고, 쓰기 권한 없는 역할은 완료 건도 연결할 수 없다.
	cl := doFormUser(t, e, "/workboard/tasks/"+id+"/action/project", url.Values{"project_id": {"WP-O01"}}, jwtCookieRole(t, model.RoleCLevel))
	if cl.Code != http.StatusForbidden {
		t.Fatalf("C-Level 완료 건 연결 %d", cl.Code)
	}
}

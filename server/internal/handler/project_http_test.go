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

func newProjectServer(t *testing.T) (*echo.Echo, *repository.ProjectRepo, *repository.WBRepo) {
	t.Helper()
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "project.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/projects", h.Project.List)
	g.GET("/projects/new", h.Project.New)
	g.POST("/projects", h.Project.Create)
	g.GET("/projects/:id", h.Project.Show)
	g.GET("/projects/:id/edit", h.Project.Edit)
	g.POST("/projects/:id", h.Project.Update)
	g.POST("/projects/:id/archive", h.Project.Archive)
	g.POST("/projects/:id/activate", h.Project.Activate)
	g.POST("/projects/:id/delete", h.Project.Delete)
	return e, repository.NewProjectRepo(db), repository.NewWBRepo(db)
}

func TestProjectCRUDArchiveSmoke(t *testing.T) {
	e, repo, wb := newProjectServer(t)

	// 등록
	rec := doForm(t, e, "/projects", url.Values{
		"name":          {"스모크 테스트 사업"},
		"short_name":    {"스모크사업"},
		"plan_year":     {"2026"},
		"is_paid":       {"1"},
		"contract_type": {"유지보수"},
		"billing_type":  {"월정액"},
		"start_date":    {"2026-01-01"},
		"end_date":      {"2026-12-31"},
		"status":        {"active"},
		"color":         {"#3B82F6"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("등록 status=%d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "/projects/") || !strings.Contains(loc, "ok=created") {
		t.Fatalf("등록 redirect=%q", loc)
	}
	id := strings.TrimPrefix(strings.Split(loc, "?")[0], "/projects/")
	if id == "" {
		t.Fatal("project id 없음")
	}

	// 목록 노출
	list := doGet(t, e, "/projects?search="+url.QueryEscape("스모크"))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "스모크사업") {
		t.Fatalf("목록 미노출 status=%d", list.Code)
	}

	// 수정
	rec = doForm(t, e, "/projects/"+id, url.Values{
		"name":       {"스모크 테스트 사업 수정"},
		"short_name": {"스모크수정"},
		"plan_year":  {"2026"},
		"is_paid":    {"1"},
		"status":     {"active"},
		"color":      {"#10B981"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "ok=updated") {
		t.Fatalf("수정: status=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}
	got, err := repo.Get(id)
	if err != nil || got.ShortName != "스모크수정" {
		t.Fatalf("수정 반영 실패: %+v err=%v", got, err)
	}

	// 보관 → 활성 콤보 제외
	rec = doForm(t, e, "/projects/"+id+"/archive", url.Values{})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "ok=archived") {
		t.Fatalf("보관: status=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}
	active, err := wb.ListProjects(true)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range active {
		if p.ProjectID == id {
			t.Fatal("보관 사업이 활성 콤보에 남음")
		}
	}
	p, err := wb.GetProject(id)
	if err != nil || p.Status != model.WBProjectArchived {
		t.Fatalf("보관 상태: %+v err=%v", p, err)
	}

	// 재개
	rec = doForm(t, e, "/projects/"+id+"/activate", url.Values{})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "ok=activated") {
		t.Fatalf("재개: status=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}
	filtered, err := wb.ListProjectsFiltered("스모크수정", model.WBProjectActive)
	if err != nil || len(filtered) != 1 {
		t.Fatalf("재개 후 필터: len=%d err=%v", len(filtered), err)
	}

	// 연결 업무 없이 삭제
	rec = doForm(t, e, "/projects/"+id+"/delete", url.Values{})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "ok=deleted") {
		t.Fatalf("삭제: status=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestWBRepoProjectHelpers(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "wb_proj.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	wb := repository.NewWBRepo(db)
	p := &model.WorkProject{Name: "헬퍼사업", Status: model.WBProjectActive, Color: "#3B82F6", IsPaid: true}
	if err := wb.CreateProject(p); err != nil {
		t.Fatal(err)
	}
	got, err := wb.GetProject(p.ProjectID)
	if err != nil || got.Name != "헬퍼사업" {
		t.Fatalf("GetProject: %+v err=%v", got, err)
	}
	got.Notes = "메모"
	if err := wb.UpdateProject(got); err != nil {
		t.Fatal(err)
	}
	n, err := wb.CountTasksByProject(p.ProjectID)
	if err != nil || n != 0 {
		t.Fatalf("CountTasks: %d err=%v", n, err)
	}
	if err := wb.SetProjectStatus(p.ProjectID, model.WBProjectArchived); err != nil {
		t.Fatal(err)
	}
	list, err := wb.ListProjectsFiltered("헬퍼", model.WBProjectArchived)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListProjectsFiltered: %d err=%v", len(list), err)
	}
	if err := wb.DeleteProject(p.ProjectID); err != nil {
		t.Fatal(err)
	}
}

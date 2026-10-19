package handler

import (
	"net/http"
	"net/url"
	"os"
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
	h.Project.backup.DataDir = filepath.Join(dir, "data")
	_ = os.MkdirAll(h.Project.backup.DataDir, 0755)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/projects", h.Project.List)
	g.GET("/projects/new", h.Project.New)
	g.GET("/projects/registry", h.Project.Registry)
	g.GET("/projects/kind-review", h.Project.KindReview)
	g.POST("/projects/kind-review", h.Project.KindReviewSave)
	g.GET("/projects/dashboard", h.Project.Dashboard)
	g.GET("/contracts/:id", h.Project.ContractShow)
	g.GET("/projects/docs", h.Project.Docs)
	g.GET("/projects/diary", h.Project.Diary)
	g.POST("/projects/diary/draft", h.Project.DiaryDraft)
	g.POST("/projects/diary/review", h.Project.DiaryReview)
	g.POST("/projects", h.Project.Create)
	g.GET("/projects/:id", h.Project.Show)
	g.GET("/projects/:id/edit", h.Project.Edit)
	g.POST("/projects/:id/kind", h.Project.SetKind)
	g.POST("/projects/:id", h.Project.Update)
	g.POST("/projects/:id/archive", h.Project.Archive)
	g.POST("/projects/:id/complete", h.Project.Complete)
	g.POST("/projects/:id/complete-cancel", h.Project.CompleteCancel)
	g.POST("/projects/:id/activate", h.Project.Activate)
	g.GET("/projects/:id/merge", h.Project.MergeForm, h.Auth.RequireVisionOnly)
	g.POST("/projects/:id/merge", h.Project.MergeSave, h.Auth.RequireVisionOnly)
	g.POST("/projects/:id/merge/undo", h.Project.MergeUndo, h.Auth.RequireVisionOnly)
	g.POST("/projects/:id/delete", h.Project.Delete)
	return e, repository.NewProjectRepo(db), repository.NewWBRepo(db)
}

func TestProjectFormHasOtherKind(t *testing.T) {
	e, _, _ := newProjectServer(t)
	rec := doForm(t, e, "/projects", url.Values{
		"name": {"기타유형"}, "status": {"active"}, "is_paid": {"1"}, "project_kind": {"other"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("등록 status=%d", rec.Code)
	}
	id := strings.TrimPrefix(strings.Split(rec.Header().Get("Location"), "?")[0], "/projects/")
	page := doGet(t, e, "/projects/"+id+"/edit")
	if page.Code != http.StatusOK {
		t.Fatalf("수정 status=%d", page.Code)
	}
	body := page.Body.String()
	if !strings.Contains(body, `value="other"`) || !strings.Contains(body, "기타") {
		t.Fatal("수정 화면에 기타 유형이 없다")
	}
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
		"project_kind":  {"maintenance"},
		"contract_amount": {"1000"},
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

	show := doGet(t, e, "/projects/"+id)
	if show.Code != http.StatusOK || !strings.Contains(show.Body.String(), "스모크사업") {
		t.Fatalf("상세 미노출 status=%d", show.Code)
	}
	list := doGet(t, e, "/projects/registry")
	if list.Code != http.StatusOK {
		t.Fatalf("목록 status=%d", list.Code)
	}
	if !strings.Contains(list.Body.String(), "스모크사업") {
		t.Fatal("등록 관리 목록에 사업이 없다")
	}
	home := doGet(t, e, "/projects")
	if home.Code != http.StatusOK || !strings.Contains(home.Body.String(), "사업 등록 관리") {
		t.Fatal("사업·업무관리에 사업 등록 관리 링크가 없다")
	}
	if strings.Contains(home.Body.String(), ">사업 등록<") {
		t.Fatal("사업관리에서 직접 등록 버튼이 남아 있다")
	}

	// 수정
	rec = doForm(t, e, "/projects/"+id, url.Values{
		"name":       {"스모크 테스트 사업 수정"},
		"short_name": {"스모크수정"},
		"plan_year":  {"2026"},
		"is_paid":    {"1"},
		"status":     {"active"},
		"project_kind": {"maintenance"},
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

func TestProjectListNumberAndDuplicateBanner(t *testing.T) {
	e, _, _ := newProjectServer(t)
	for i := 0; i < 2; i++ {
		rec := doForm(t, e, "/projects", url.Values{
			"name": {"중복사업"}, "status": {"active"}, "is_paid": {"1"}, "project_kind": {"maintenance"}, "contract_amount": {"1000"},
		})
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("등록 %d", rec.Code)
		}
	}
	list := doGet(t, e, "/projects/registry")
	body := list.Body.String()
	if !strings.Contains(body, "이름이 같은 사업이 2건") {
		t.Fatalf("중복 띠 없음: %s", clipBody(body))
	}
}

func TestProjectWorkMgmtFigmaLayout(t *testing.T) {
	e, _, _ := newProjectServer(t)
	page := doGet(t, e, "/projects")
	if page.Code != http.StatusOK {
		t.Fatalf("status=%d", page.Code)
	}
	body := page.Body.String()
	for _, want := range []string{
		"사업·업무관리",
		"업무 리포트",
		"업무 등록",
		"사업 등록 관리",
		"사업 선택",
		"표시할 사업이 없습니다",
		"공정→기성→준공",
		"구축→개통→검수",
		"접수→조치→완료",
		`value="other"`,
		"수행 기간",
		"단계별 일정표",
		"계획 시작",
		"실제 처리",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("화면에 %q 없음", want)
		}
	}
	for _, leak := range []string{"A동 골조공사", "김현우", "data-node-id", "화면 설계를 위한 예시"} {
		if strings.Contains(body, leak) {
			t.Fatalf("샘플/원본 흔적 %q", leak)
		}
	}
}

func TestProjectMergeHTTPArchivesDrop(t *testing.T) {
	e, repo, _ := newProjectServer(t)
	vision := jwtCookieRole(t, model.RoleVisionAdmin)
	keepRec := doForm(t, e, "/projects", url.Values{"name": {"남길사업"}, "status": {"active"}, "is_paid": {"1"}, "project_kind": {"maintenance"}})
	dropRec := doForm(t, e, "/projects", url.Values{"name": {"없앨사업"}, "status": {"active"}, "is_paid": {"1"}, "project_kind": {"maintenance"}})
	keepID := strings.TrimPrefix(strings.Split(keepRec.Header().Get("Location"), "?")[0], "/projects/")
	dropID := strings.TrimPrefix(strings.Split(dropRec.Header().Get("Location"), "?")[0], "/projects/")
	form := workFormCookie(t, e, "/projects/"+keepID+"/merge", url.Values{"drop_id": {dropID}}, vision)
	if form.Code != http.StatusSeeOther || !strings.Contains(form.Header().Get("Location"), "ok=merged") {
		t.Fatalf("merge status=%d loc=%s body=%s", form.Code, form.Header().Get("Location"), form.Body.String())
	}
	drop, err := repo.Get(dropID)
	if err != nil || drop.Status != model.WBProjectArchived {
		t.Fatalf("보관되지 않음 %+v err=%v", drop, err)
	}
}

func TestProjectCreateKindRequired(t *testing.T) {
	e, repo, _ := newProjectServer(t)
	rec := doForm(t, e, "/projects", url.Values{
		"name": {"유형없음"}, "status": {"active"}, "is_paid": {"1"},
	})
	if rec.Code == http.StatusSeeOther {
		t.Fatal("유형 없이 등록되면 안 된다")
	}
	rec = doForm(t, e, "/projects", url.Values{
		"name": {"유형있음"}, "status": {"active"}, "is_paid": {"1"}, "project_kind": {"build"}, "contract_amount": {"1"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("유형 지정 등록 status=%d", rec.Code)
	}
	id := strings.TrimPrefix(strings.Split(rec.Header().Get("Location"), "?")[0], "/projects/")
	got, err := repo.Get(id)
	if err != nil || got.ProjectKind != model.ProjectKindBuild || got.ProjectKindSet != 1 {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestProjectKindInlineAndBulk(t *testing.T) {
	e, repo, _ := newProjectServer(t)
	p := &model.WorkProject{Name: "도서관 구축", PlanYear: 2026, Status: model.WBProjectActive, ContractAmount: 1}
	if err := repo.Create(p); err != nil {
		t.Fatal(err)
	}
	rec := doForm(t, e, "/projects/"+p.ProjectID+"/kind", url.Values{"project_kind": {"build"}, "back": {"/projects/registry"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("인라인 %d", rec.Code)
	}
	got, _ := repo.Get(p.ProjectID)
	if got.ProjectKind != model.ProjectKindBuild || got.ProjectKindSet != 1 {
		t.Fatalf("확정 %+v", got)
	}
	ro := workFormCookie(t, e, "/projects/"+p.ProjectID+"/kind", url.Values{"project_kind": {"dev"}}, jwtCookieReadOnly(t, model.RoleAdmin))
	if ro.Code != http.StatusForbidden {
		t.Fatalf("읽기전용 %d", ro.Code)
	}

	p2 := &model.WorkProject{Name: "유지보수 힌트", PlanYear: 2026, Status: model.WBProjectActive, ContractAmount: 1}
	if err := repo.Create(p2); err != nil {
		t.Fatal(err)
	}
	rev := doGet(t, e, "/projects/kind-review")
	if !strings.Contains(rev.Body.String(), "유지보수 힌트") {
		t.Fatal("정리 화면 없음")
	}
	skip := doForm(t, e, "/projects/kind-review", url.Values{
		"kind_" + p2.ProjectID: {model.ProjectKindMaintenance},
	})
	if skip.Code != http.StatusSeeOther {
		t.Fatalf("미체크 저장 %d", skip.Code)
	}
	still, _ := repo.Get(p2.ProjectID)
	if still.ProjectKindSet == 1 {
		t.Fatal("미체크 행이 저장됐다")
	}
	save := doForm(t, e, "/projects/kind-review", url.Values{
		"pick":                 {p2.ProjectID},
		"kind_" + p2.ProjectID: {model.ProjectKindMaintenance},
	})
	if save.Code != http.StatusSeeOther {
		t.Fatalf("체크 저장 %d", save.Code)
	}
	done, _ := repo.Get(p2.ProjectID)
	if done.ProjectKind != model.ProjectKindMaintenance || done.ProjectKindSet != 1 {
		t.Fatalf("일괄 %+v", done)
	}
}

func TestProjectCompleteDoesNotAutoOnEndDate(t *testing.T) {
	e, repo, _ := newProjectServer(t)
	p := &model.WorkProject{Name: "종료경과", PlanYear: 2026, Status: model.WBProjectActive, ContractAmount: 1, EndDate: "2020-01-01"}
	if err := repo.Create(p); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.Get(p.ProjectID)
	if got.ContractRunStatus("2026-10-19") != model.RunNeedComplete {
		t.Fatalf("run=%s", got.ContractRunStatus("2026-10-19"))
	}
	if got.Status == model.WBProjectComplete {
		t.Fatal("자동 완료")
	}
	show := doGet(t, e, "/projects/"+p.ProjectID)
	if !strings.Contains(show.Body.String(), "완료 확인 필요") || !strings.Contains(show.Body.String(), "사업 완료 처리") {
		t.Fatal("완료 확인 필요 표시 없음")
	}
	ro := workFormCookie(t, e, "/projects/"+p.ProjectID+"/complete", url.Values{}, jwtCookieReadOnly(t, model.RoleAdmin))
	if ro.Code != http.StatusForbidden {
		t.Fatalf("읽기전용 %d", ro.Code)
	}
	rec := doForm(t, e, "/projects/"+p.ProjectID+"/complete", url.Values{})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("완료 %d", rec.Code)
	}
	got, _ = repo.Get(p.ProjectID)
	if got.Status != model.WBProjectComplete || got.ContractRunStatus("2026-10-19") != model.RunDone {
		t.Fatalf("완료 후 %+v", got)
	}
	rec = doForm(t, e, "/projects/"+p.ProjectID+"/complete-cancel", url.Values{})
	got, _ = repo.Get(p.ProjectID)
	if got.Status != model.WBProjectActive {
		t.Fatalf("취소 %+v", got)
	}
}

func TestProjectWriteRejectedWhenReadOnly(t *testing.T) {
	e, _, _ := newProjectServer(t)
	rec := workFormCookie(t, e, "/projects", url.Values{
		"name": {"RO"}, "status": {"active"}, "is_paid": {"1"}, "project_kind": {"maintenance"},
	}, jwtCookieReadOnly(t, model.RoleAdmin))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d", rec.Code)
	}
}

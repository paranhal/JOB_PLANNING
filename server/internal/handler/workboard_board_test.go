package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// newWorkboardServer 워크보드 라우트만 붙인 테스트 서버
func newWorkboardServer(t *testing.T, name string) (*echo.Echo, *repository.WBRepo) {
	t.Helper()
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, name))
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
	g.GET("/workboard/kanban", h.Workboard.Kanban)
	g.GET("/workboard/tasks", h.Workboard.List)
	g.GET("/workboard/register", h.Workboard.Register)
	g.GET("/workboard/tasks/:id", h.Workboard.ShowTask)
	g.POST("/workboard/tasks", h.Workboard.CreateTask)
	g.POST("/workboard/projects", h.Workboard.CreateProject)
	g.POST("/workboard/tasks/:id/subtasks", h.Workboard.CreateSubtasks)
	g.POST("/workboard/tasks/:id/recurrence/preview", h.Workboard.PreviewRecurrence)
	g.POST("/workboard/tasks/:id/recurrence/generate", h.Workboard.GenerateRecurrence)
	g.POST("/workboard/tasks/:id/recurrence/regenerate", h.Workboard.RegenerateRecurrence)
	g.POST("/workboard/tasks/:id/recurrence/settings", h.Workboard.SaveRecurrenceSettings)
	g.POST("/workboard/tasks/:id/occurrences/:oid", h.Workboard.UpdateOccurrence)
	g.POST("/workboard/tasks/:id/update", h.Workboard.UpdateTask)
	return e, repository.NewWBRepo(db)
}

func doForm(t *testing.T, e *echo.Echo, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost"+path, strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	return rec
}

func doGet(t *testing.T, e *echo.Echo, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost"+path, nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	return rec
}

// 프로젝트 등록 → 업무(지원업무) 등록 → 칸반·목록에 반영되는 흐름
func TestWorkboardCreateAndRender(t *testing.T) {
	e, repo := newWorkboardServer(t, "wb.db")

	rec := doForm(t, e, "/workboard/projects", url.Values{
		"name":          {"2026 도서관 자동화 유지보수"},
		"contract_type": {"private"},
		"billing_type":  {"monthly"},
		"start_date":    {"2026-01-01"},
		"end_date":      {"2026-12-31"},
		"notes":         {"연간 계약"},
		"redirect":      {"/workboard/kanban"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "ok=project") {
		t.Fatalf("프로젝트 등록: status=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}
	projects, err := repo.ListProjects(false)
	if err != nil {
		t.Fatalf("ListProjects: err=%v", err)
	}
	// 기본 사업이 함께 들어 있으므로 방금 등록한 건만 찾는다.
	var p model.WorkProject
	for _, item := range projects {
		if item.Name == "2026 도서관 자동화 유지보수" {
			p = item
			break
		}
	}
	if p.ProjectID == "" {
		t.Fatalf("등록한 사업이 목록에 없음: len=%d", len(projects))
	}
	if p.Status != "active" || p.Color == "" {
		t.Fatalf("프로젝트 기본값: status=%q color=%q", p.Status, p.Color)
	}
	if p.ContractType != "private" || p.BillingType != "monthly" || p.EndDate != "2026-12-31" {
		t.Fatalf("프로젝트 저장값: %+v", p)
	}

	today := time.Now().Format("2006-01-02")
	rec = doForm(t, e, "/workboard/tasks", url.Values{
		"work_type":  {"support"},
		"project_id": {p.ProjectID},
		"title":      {"월 정기점검 보고서 작성"},
		"due_date":   {today},
		"work_date":  {today},
		"start_time": {"09:00"},
		"end_time":   {"10:00"},
		"status":     {"in_progress"},
		"priority":   {"urgent"},
		"assignee":   {"관리자"},
		"tags":       {"보고서,점검"},
		"progress":   {"40"},
		"redirect":   {"/workboard/tasks"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "ok=task") {
		t.Fatalf("업무 등록: status=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}

	tasks, err := repo.ListTasks()
	if err != nil || len(tasks) != 1 {
		t.Fatalf("ListTasks: len=%d err=%v", len(tasks), err)
	}
	task := tasks[0]
	if task.WorkType != "support" || task.ProjectID != p.ProjectID || task.Progress != 40 {
		t.Fatalf("업무 저장값: %+v", task)
	}

	kanban := doGet(t, e, "/workboard/kanban")
	if kanban.Code != http.StatusOK {
		t.Fatalf("칸반: status=%d", kanban.Code)
	}
	body := kanban.Body.String()
	for _, want := range []string{"칸반 보드", "정기점검", "AS", "행정/사업지원", "완료", "월 정기점검 보고서 작성", "긴급", "지원업무", "40%", "할 일", "진행중", "검토"} {
		if !strings.Contains(body, want) {
			t.Errorf("칸반 화면에 %q 없음", want)
		}
	}

	list := doGet(t, e, "/workboard/tasks")
	if list.Code != http.StatusOK {
		t.Fatalf("목록: status=%d", list.Code)
	}
	body = list.Body.String()
	for _, want := range []string{"업무 목록", "월 정기점검 보고서 작성", "2026 도서관 자동화 유지보수", today, "관리자"} {
		if !strings.Contains(body, want) {
			t.Errorf("목록 화면에 %q 없음", want)
		}
	}
}

// 지원업무는 사업명 없이 등록되면 안 되고, 행정업무는 사업명 없이도 등록된다.
func TestWorkboardTaskWorkTypeRules(t *testing.T) {
	e, repo := newWorkboardServer(t, "wb_rules.db")

	rec := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type":  {"support"},
		"title":      {"사업명 없는 지원업무"},
		"due_date":   {"2026-08-31"},
		"work_date":  {"2026-08-31"},
		"start_time": {"09:00"},
		"end_time":   {"09:30"},
	})
	if !strings.Contains(rec.Header().Get("Location"), "err=project_required") {
		t.Fatalf("지원업무 사업명 검증 실패: loc=%q", rec.Header().Get("Location"))
	}

	rec = doForm(t, e, "/workboard/tasks", url.Values{
		"work_type":  {"admin"},
		"title":      {"행정업무"},
		"due_date":   {"2026-08-31"},
		"work_date":  {"2026-08-31"},
		"start_time": {"09:00"},
		"end_time":   {"09:30"},
	})
	if !strings.Contains(rec.Header().Get("Location"), "ok=task") {
		t.Fatalf("행정업무 등록 실패: loc=%q", rec.Header().Get("Location"))
	}

	rec = doForm(t, e, "/workboard/tasks", url.Values{
		"work_type": {"admin"}, "title": {"시간 없음"}, "due_date": {"2026-08-31"},
	})
	if !strings.Contains(rec.Header().Get("Location"), "err=time") {
		t.Fatalf("시간 필수 검증 실패: loc=%q", rec.Header().Get("Location"))
	}

	rec = doForm(t, e, "/workboard/projects", url.Values{"name": {""}})
	if !strings.Contains(rec.Header().Get("Location"), "err=name") {
		t.Fatalf("사업명 검증 실패: loc=%q", rec.Header().Get("Location"))
	}

	tasks, _ := repo.ListTasks()
	if len(tasks) != 1 {
		t.Fatalf("등록된 업무 수 = %d, want 1", len(tasks))
	}
	if tasks[0].WorkType != "admin" || tasks[0].ProjectID != "" {
		t.Fatalf("행정업무 저장값: %+v", tasks[0])
	}
}

// 완료 상태로 등록하면 진행률이 100%로 채워진다.
func TestWorkboardCompleteTaskProgress(t *testing.T) {
	e, repo := newWorkboardServer(t, "wb_progress.db")

	doForm(t, e, "/workboard/tasks", url.Values{
		"work_type":  {"admin"},
		"title":      {"완료 업무"},
		"due_date":   {"2026-08-10"},
		"work_date":  {"2026-08-10"},
		"start_time": {"09:00"},
		"end_time":   {"09:30"},
		"status":     {"complete"},
	})
	doForm(t, e, "/workboard/tasks", url.Values{
		"work_type":  {"admin"},
		"title":      {"진행률 초과 입력"},
		"due_date":   {"2026-08-10"},
		"work_date":  {"2026-08-10"},
		"start_time": {"10:00"},
		"end_time":   {"10:30"},
		"progress":   {"250"},
	})

	tasks, _ := repo.ListTasks()
	byTitle := map[string]int{}
	for _, t := range tasks {
		byTitle[t.Title] = t.Progress
	}
	if byTitle["완료 업무"] != 100 {
		t.Errorf("완료 업무 진행률 = %d, want 100", byTitle["완료 업무"])
	}
	if byTitle["진행률 초과 입력"] != 100 {
		t.Errorf("진행률 상한 = %d, want 100", byTitle["진행률 초과 입력"])
	}
}

func TestInboxAndCancelledExcludedFromKanban(t *testing.T) {
	e, repo := newWorkboardServer(t, "wb_inbox_kanban.db")
	today := time.Now().Format("2006-01-02")
	inbox := &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "수집함만있는메모",
		Status: model.WBTaskInbox, DueDate: today, WorkDate: today,
	}
	if err := repo.CreateTask(inbox); err != nil {
		t.Fatal(err)
	}
	cancelled := &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "취소된행정업무",
		Status: model.WBTaskCancelled, CancelReason: "중복", DueDate: today, WorkDate: today,
	}
	if err := repo.CreateTask(cancelled); err != nil {
		t.Fatal(err)
	}
	hold := &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "보류된행정업무",
		Status: model.WBTaskHold, HoldReason: "일정조정", ReviewDate: today, DueDate: today, WorkDate: today,
	}
	if err := repo.CreateTask(hold); err != nil {
		t.Fatal(err)
	}
	waitingFor := &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "회신대기행정업무",
		Status: model.WBTaskWaitingFor, WaitParty: "업체", WaitRequest: "견적",
		ReplyDueDate: today, DueDate: today, WorkDate: today,
	}
	if err := repo.CreateTask(waitingFor); err != nil {
		t.Fatal(err)
	}

	body := doGet(t, e, "/workboard/kanban").Body.String()
	if strings.Contains(body, "수집함만있는메모") {
		t.Fatal("수집함이 칸반에 올라감")
	}
	if strings.Contains(body, "취소된행정업무") {
		t.Fatal("취소 건이 칸반에 올라감")
	}
	if !strings.Contains(body, "보류된행정업무") || !strings.Contains(body, "회신대기행정업무") {
		t.Fatal("검토 열에 보류·회신 대기가 없음")
	}
}

func TestWaitingActionNextCheckShowsAsTodayTodo(t *testing.T) {
	e, repo := newWorkboardServer(t, "wb_next_check.db")
	today := time.Now().Format("2006-01-02")
	task := &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "IRM 본건", Status: model.WBTaskWaitingFor,
		DueDate: today, WorkDate: today,
	}
	if err := repo.CreateTask(task); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateAction(&model.WorkAction{
		TaskID: task.TaskID, Title: "전자도서관 부문 재확인", Status: model.WBActionWaiting, Required: true,
		WaitParty: "□□정보", WaitRequest: "수정본", ReplyDueDate: today, NextCheckDate: today,
	}); err != nil {
		t.Fatal(err)
	}
	body := doGet(t, e, "/workboard/kanban").Body.String()
	if !strings.Contains(body, "확인: 전자도서관 부문 재확인") {
		t.Fatal("다음 확인일이 오늘인 행동이 할 일로 안 보임")
	}
}

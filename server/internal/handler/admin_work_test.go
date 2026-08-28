package handler

import (
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/repository"
)

func newAdminWorkServer(t *testing.T) (*echo.Echo, *repository.WBRepo) {
	t.Helper()
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "admin_work.db"))
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
	aw := g.Group("/admin-work")
	aw.GET("", h.AdminWork.List)
	aw.GET("/stats", h.AdminWork.Stats)
	aw.GET("/new", h.AdminWork.New)
	aw.POST("", h.AdminWork.Create)
	aw.POST("/inbox", h.AdminWork.CreateInbox)
	aw.POST("/:id/classify", h.AdminWork.Classify)
	aw.POST("/:id/move", h.AdminWork.MoveKanban)
	aw.GET("/:id", h.AdminWork.Show)
	return e, repository.NewWBRepo(db)
}

func TestAdminWorkMenuPagesRender(t *testing.T) {
	e, _ := newAdminWorkServer(t)

	list := doGet(t, e, "/admin-work")
	if list.Code != http.StatusOK {
		t.Fatalf("목록 status=%d body=%s", list.Code, list.Body.String())
	}
	body := list.Body.String()
	if !strings.Contains(body, "행정·지원업무") || !strings.Contains(body, `href="/admin-work"`) {
		t.Fatal("사이드바 행정관련업무 메뉴 없음")
	}
	if strings.Contains(body, "수집함") {
		t.Fatal("목록에 수집함 문구가 남아 있음")
	}
	if !strings.Contains(body, "빠른 등록") || !strings.Contains(body, "sort=due_date") {
		t.Fatal("빠른 등록 또는 정렬 링크 없음")
	}
	kanban := doGet(t, e, "/admin-work?view=kanban")
	kb := kanban.Body.String()
	if kanban.Code != http.StatusOK || !strings.Contains(kb, "할 일") || !strings.Contains(kb, "진행중") || !strings.Contains(kb, "완료") {
		t.Fatal("칸반 3열이 렌더되지 않음")
	}

	stats := doGet(t, e, "/admin-work/stats")
	if stats.Code != http.StatusOK || !strings.Contains(stats.Body.String(), "행정관련업무현황") {
		t.Fatalf("현황 status=%d", stats.Code)
	}

	form := doGet(t, e, "/admin-work/new")
	if form.Code != http.StatusOK {
		t.Fatalf("등록 status=%d", form.Code)
	}
	fb := form.Body.String()
	if !strings.Contains(fb, `name="customer_id"`) || !strings.Contains(fb, `name="customer_name"`) {
		t.Fatal("등록 화면에 거래처 필드 없음")
	}
	if !strings.Contains(fb, `name="project_id"`) || strings.Contains(fb, `:disabled="workType !== 'support'"`) {
		t.Fatal("등록 화면에 행정업무용 사업명 필드가 없거나 비활성화됨")
	}
	if !strings.Contains(fb, "충남교육청") {
		t.Fatal("거래처 직접입력 안내(충남교육청) 없음")
	}
	if !strings.Contains(fb, "매월") || !strings.Contains(fb, "지정일자") {
		t.Fatal("등록 화면에 매월·지정일자 반복 규칙이 없다")
	}
}

func TestAdminWorkCreateSeparatesCustomerAndTitle(t *testing.T) {
	e, repo := newAdminWorkServer(t)
	title := "충남교육청통합도서관 27년 유지보수 계약을 위한 과업심의 안건 자료 제출"

	rec := doForm(t, e, "/admin-work", url.Values{
		"work_type":     {"admin"},
		"customer_name": {"충남교육청"},
		"title":         {title},
		"description":   {"과업심의 자료"},
		"due_date":      {"2026-08-14"},
		"status":        {"waiting"},
		"priority":      {"normal"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "/admin-work") {
		t.Fatalf("등록: status=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}

	items, err := repo.ListAdminWork("", "")
	if err != nil || len(items) != 1 {
		t.Fatalf("ListAdminWork len=%d err=%v", len(items), err)
	}
	got := items[0]
	if got.CustomerName != "충남교육청" {
		t.Fatalf("거래처=%q want 충남교육청", got.CustomerName)
	}
	if got.Title != title {
		t.Fatalf("업무명=%q", got.Title)
	}
	if got.CustomerLabel() != "충남교육청" {
		t.Fatalf("CustomerLabel=%q", got.CustomerLabel())
	}

	list := doGet(t, e, "/admin-work")
	body := list.Body.String()
	if list.Code != http.StatusOK {
		t.Fatalf("목록 status=%d", list.Code)
	}
	if !strings.Contains(body, "충남교육청") || !strings.Contains(body, title) {
		t.Fatalf("목록에 거래처·업무명 미노출")
	}

	search := doGet(t, e, "/admin-work?search="+url.QueryEscape("충남교육청"))
	if search.Code != http.StatusOK || !strings.Contains(search.Body.String(), title) {
		t.Fatal("거래처 검색 실패")
	}

	show := doGet(t, e, "/admin-work/"+got.TaskID)
	if show.Code != http.StatusSeeOther {
		t.Fatalf("상세 redirect status=%d", show.Code)
	}
	loc := show.Header().Get("Location")
	if !strings.Contains(loc, "/workboard/tasks/"+got.TaskID) || !strings.Contains(loc, "back=") {
		t.Fatalf("상세 loc=%q", loc)
	}
}

func TestAdminWorkCreateRequiresTitleAndDue(t *testing.T) {
	e, _ := newAdminWorkServer(t)
	rec := doForm(t, e, "/admin-work", url.Values{
		"work_type":     {"admin"},
		"customer_name": {"충남교육청"},
		"title":         {""},
		"due_date":      {"2026-08-14"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=task") {
		t.Fatalf("빈 업무명: status=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestAdminWorkCreateKeepsProjectForAdmin(t *testing.T) {
	e, repo := newAdminWorkServer(t)
	rec := doForm(t, e, "/admin-work", url.Values{
		"work_type":     {"admin"},
		"customer_name": {"충남교육청"},
		"title":         {"과업심의 자료 제출"},
		"due_date":      {"2026-08-20"},
		"status":        {"waiting"},
		"priority":      {"normal"},
		"project_id":    {"WPSEED01"},
	})
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("행정+사업명 등록 loc=%q", rec.Header().Get("Location"))
	}
	items, err := repo.ListAdminWork("", "")
	if err != nil || len(items) != 1 {
		t.Fatalf("len=%d err=%v", len(items), err)
	}
	if items[0].ProjectID != "WPSEED01" {
		t.Fatalf("project_id=%q want WPSEED01", items[0].ProjectID)
	}
	if strings.TrimSpace(items[0].ProjectName) == "" {
		t.Fatal("ProjectName 비어 있음")
	}
	list := doGet(t, e, "/admin-work")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), items[0].ProjectName) {
		t.Fatalf("목록에 사업명 없음 project=%q", items[0].ProjectName)
	}
}

func TestAdminWorkCreateDefaultsStatusPriority(t *testing.T) {
	e, repo := newAdminWorkServer(t)
	rec := doForm(t, e, "/admin-work", url.Values{
		"work_type":     {"admin"},
		"customer_name": {"충남교육청"},
		"title":         {"상태 없이 등록"},
		"due_date":      {"2026-08-20"},
	})
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("등록 loc=%q", rec.Header().Get("Location"))
	}
	items, err := repo.ListAdminWork("", "")
	if err != nil || len(items) != 1 {
		t.Fatalf("len=%d err=%v", len(items), err)
	}
	if items[0].Status != "waiting" || items[0].Priority != "normal" {
		t.Fatalf("기본값 status=%q priority=%q", items[0].Status, items[0].Priority)
	}
	if items[0].DueDate != "2026-08-20" {
		t.Fatalf("due_date=%q — 실행률 분모 컬럼이 바뀌면 안 된다", items[0].DueDate)
	}
}

func TestAdminWorkCreateUndeterminedClearsDueDate(t *testing.T) {
	e, repo := newAdminWorkServer(t)
	rec := doForm(t, e, "/admin-work", url.Values{
		"work_type":        {"admin"},
		"title":            {"종료일 미정"},
		"due_date":         {"2026-08-20"},
		"due_undetermined": {"1"},
		"no_date_reason":   {"customer"},
	})
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("미정 등록 loc=%q", rec.Header().Get("Location"))
	}
	items, err := repo.ListAdminWork("", "")
	if err != nil || len(items) != 1 {
		t.Fatalf("len=%d err=%v", len(items), err)
	}
	if items[0].DueDate != "" {
		t.Fatalf("미정이면 due_date가 비어야 한다: %q", items[0].DueDate)
	}
	if items[0].Status != "waiting" || items[0].Priority != "normal" {
		t.Fatalf("기본값 status=%q priority=%q", items[0].Status, items[0].Priority)
	}
}

func TestAdminWorkNewFormSection33Labels(t *testing.T) {
	e, _ := newAdminWorkServer(t)
	form := doGet(t, e, "/admin-work/new")
	if form.Code != http.StatusOK {
		t.Fatalf("status=%d", form.Code)
	}
	body := form.Body.String()
	for _, want := range []string{"업무 내용", "업무 등록일", "업무 시작일", "업무 종료일", "＋하위 업무 등록", "기간 내 반복 실행"} {
		if !strings.Contains(body, want) {
			t.Errorf("등록 화면에 %q 없음", want)
		}
	}
	if strings.Contains(body, "접수·처리") {
		t.Fatal("접수·처리 내용 라벨이 남아 있다")
	}
	if strings.Contains(body, `name="status"`) || strings.Contains(body, `name="priority"`) {
		t.Fatal("등록 폼에 상태·우선순위 입력란이 있다")
	}
}

func TestAdminWorkCreateSubtaskPrefillAndDepth(t *testing.T) {
	e, repo := newAdminWorkServer(t)
	rec := doForm(t, e, "/admin-work", url.Values{
		"work_type":     {"admin"},
		"customer_name": {"충남교육청"},
		"title":         {"상위 업무"},
		"due_date":      {"2026-09-30"},
		"assignee":      {"관리자"},
	})
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("parent loc=%q", rec.Header().Get("Location"))
	}
	items, err := repo.ListAdminWork("", "")
	if err != nil || len(items) != 1 {
		t.Fatalf("len=%d err=%v", len(items), err)
	}
	parent := items[0]
	form := doGet(t, e, "/admin-work/new?parent="+parent.TaskID)
	if form.Code != http.StatusOK {
		t.Fatalf("form status=%d", form.Code)
	}
	body := form.Body.String()
	if !strings.Contains(body, `name="parent_task_id"`) || !strings.Contains(body, parent.TaskID) {
		t.Fatal("상위 번호가 폼에 없다")
	}
	if !strings.Contains(body, "충남교육청") {
		t.Fatal("거래처 기본값이 없다")
	}

	rec = doForm(t, e, "/admin-work", url.Values{
		"work_type":      {"admin"},
		"title":          {"1단계 하위"},
		"due_date":       {"2026-09-10"},
		"parent_task_id": {parent.TaskID},
	})
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("child loc=%q", rec.Header().Get("Location"))
	}
	child, err := repo.GetTask(parent.TaskID + "-1")
	if err != nil || child == nil {
		t.Fatalf("child missing err=%v", err)
	}
	if child.ParentTaskID != parent.TaskID {
		t.Fatalf("parent=%q", child.ParentTaskID)
	}

	rec = doForm(t, e, "/admin-work", url.Values{
		"work_type":      {"admin"},
		"title":          {"2단계 하위"},
		"due_date":       {"2026-09-11"},
		"parent_task_id": {child.TaskID},
	})
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("grandchild loc=%q", rec.Header().Get("Location"))
	}
	g := parent.TaskID + "-1-1"
	rec = doForm(t, e, "/admin-work", url.Values{
		"work_type":      {"admin"},
		"title":          {"4단계"},
		"due_date":       {"2026-09-12"},
		"parent_task_id": {g},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=sub_depth") {
		t.Fatalf("depth loc=%q", rec.Header().Get("Location"))
	}

	list := doGet(t, e, "/admin-work")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "＋하위 업무 등록") {
		t.Fatal("목록에 하위 등록 버튼이 없다")
	}
}

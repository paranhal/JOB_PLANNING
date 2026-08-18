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

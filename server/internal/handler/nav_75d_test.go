package handler

import (
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestAdminWorkNewHrefAbsentInTemplates(t *testing.T) {
	root := "web/templates"
	if _, err := os.Stat(root); err != nil {
		root = filepath.Join("..", "..", "web/templates")
	}
	var hits []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(b), `href="/admin-work/new"`) || strings.Contains(string(b), `href='/admin-work/new'`) {
			hits = append(hits, path)
		}
		return nil
	})
	if len(hits) > 0 {
		t.Fatalf("admin-work/new 링크: %v", hits)
	}
}

func TestAdminWorkSidebarHrefZero(t *testing.T) {
	res := FilterNav(visionMenuCtx(), NavDashboard, NavBadges{})
	if containsHref(collectNavHrefs(res.Groups), "/admin-work") {
		t.Fatal("사이드바에 /admin-work 가 있다")
	}
}

func TestWorkAllHasAdminKindFilter(t *testing.T) {
	p := "web/templates/work/_filters.html"
	b, err := os.ReadFile(p)
	if err != nil {
		b, err = os.ReadFile(filepath.Join("..", "..", p))
	}
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `value="admin"`) || !strings.Contains(s, "행정·지원") {
		t.Fatal("전체 업무에 행정·지원 필터가 없다")
	}
}

func TestNavTreeParentButtonsHaveAria(t *testing.T) {
	p := "web/templates/layout/_nav_tree.html"
	b, err := os.ReadFile(p)
	if err != nil {
		b, err = os.ReadFile(filepath.Join("..", "..", p))
	}
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `:aria-expanded="openP('{{.ID}}')`) {
		t.Fatal("부모 메뉴 aria-expanded 없음")
	}
}

func TestAdminWorkCreateFromRegister(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "aw_reg.db"))
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
	g.GET("/workboard/register", h.Workboard.Register)
	g.POST("/workboard/tasks", h.Workboard.CreateTask)
	day := time.Now().Format("2006-01-02")
	rec := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type":  {model.WBWorkAdmin},
		"title":      {"75D 행정 등록"},
		"due_date":   {day},
		"work_date":  {day},
		"start_time": {"09:00"},
		"end_time":   {"10:00"},
		"status":     {"waiting"},
		"priority":   {"normal"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}
	items, err := repository.NewWBRepo(db).ListAdminWork(repository.OrgAll, "", "")
	if err != nil || len(items) != 1 || items[0].Title != "75D 행정 등록" {
		t.Fatalf("등록 결과 n=%d err=%v", len(items), err)
	}
}

func TestAdminWorkListAndShowStillOpen(t *testing.T) {
	e, _ := newAdminWorkServer(t)
	for _, path := range []string{"/admin-work", "/admin-work/stats"} {
		rec := doGet(t, e, path)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status=%d", path, rec.Code)
		}
	}
}

func TestMeetingWeeklyRouteOnMainPattern(t *testing.T) {
	e, _, _, _ := newMeetingWeeklyServer(t)
	rec := weeklyGet(t, e, "/meeting/weekly", jwtCookie(t))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
}

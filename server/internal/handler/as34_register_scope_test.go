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

func TestSaveSiteConfigStoresFixedDays(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "site-fixed.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c1','세종도서관','세종도서관',1)`); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.POST("/maintenance/sites", h.Maintenance.SaveSiteConfig)
	g.GET("/maintenance/sites/:customer_id/edit", h.Maintenance.EditSiteConfigPage)

	form := url.Values{
		"customer_id":      {"c1"},
		"short_name":       {"세종"},
		"region":           {"세종"},
		"inspection_cycle": {"monthly"},
		"has_klas":         {"1"},
		"entry_category":   {"normal"},
		"fixed_day":        {"5", "15"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/maintenance/sites", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	cfg, err := h.Maintenance.repo.GetSiteConfig("c1")
	if err != nil || cfg == nil {
		t.Fatalf("설정: %v %v", cfg, err)
	}
	if cfg.FixedRule != "5,15" {
		t.Fatalf("fixed_rule=%q want 5,15", cfg.FixedRule)
	}

	edit := httptest.NewRecorder()
	ereq := httptest.NewRequest(http.MethodGet, "http://localhost/maintenance/sites/c1/edit", nil)
	ereq.AddCookie(jwtCookie(t))
	e.ServeHTTP(edit, ereq)
	if edit.Code != http.StatusOK {
		t.Fatalf("edit status=%d", edit.Code)
	}
	body := edit.Body.String()
	if !strings.Contains(body, `name="fixed_day" value="5"`) || !strings.Contains(body, "checked") {
		t.Fatalf("고정일 체크가 복원되지 않음")
	}
}

func TestRegisterTechSeesOwnTasksByDefault(t *testing.T) {
	e, repo := newWorkboardServer(t, "reg-scope.db")
	today := time.Now().Format("2006-01-02")
	for _, tsk := range []*model.WorkTask{
		{WorkType: model.WBWorkAdmin, Title: "관리자할일", Status: model.WBTaskWaiting, Assignee: "관리자", WorkDate: today, DueDate: today, StartTime: "09:00"},
		{WorkType: model.WBWorkAdmin, Title: "기술담당일", Status: model.WBTaskWaiting, Assignee: "tech", WorkDate: today, DueDate: today, StartTime: "10:00"},
	} {
		if err := repo.CreateTask(tsk); err != nil {
			t.Fatal(err)
		}
	}

	get := func(path string, ck *http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://localhost"+path, nil)
		req.AddCookie(ck)
		e.ServeHTTP(rec, req)
		return rec
	}

	mine := get("/workboard/register?view=day&date="+today, jwtCookieRole(t, "tech"))
	if mine.Code != http.StatusOK {
		t.Fatalf("status=%d", mine.Code)
	}
	body := mine.Body.String()
	if !strings.Contains(body, "기술담당일") {
		t.Fatal("기술담당 본인 건이 없다")
	}
	if strings.Contains(body, "관리자할일") {
		t.Fatal("기술담당 기본에 다른 사람 건이 보인다")
	}
	if !strings.Contains(body, "팀 전체 보기") {
		t.Fatal("팀 전체 토글이 없다")
	}

	team := get("/workboard/register?view=day&date="+today+"&mine=0", jwtCookieRole(t, "tech"))
	if team.Code != http.StatusOK {
		t.Fatalf("team status=%d", team.Code)
	}
	tbody := team.Body.String()
	if !strings.Contains(tbody, "관리자할일") || !strings.Contains(tbody, "기술담당일") {
		t.Fatal("팀 전체에서 다른 사람 건이 안 보인다")
	}
}

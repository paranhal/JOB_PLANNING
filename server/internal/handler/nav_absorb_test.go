package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/repository"
)

func TestAbsorbTabsPresentInTemplates(t *testing.T) {
	checks := []struct {
		file string
		need []string
	}{
		{"web/templates/workboard/register.html", []string{`id="absorb-tabs"`, `/plan/unplanned`}},
		{"web/templates/plan/unplanned.html", []string{`id="absorb-tabs"`, `/workboard/register`}},
		{"web/templates/stats/overview.html", []string{`id="absorb-tabs"`, `/work-status`, `/as/stats`}},
		{"web/templates/work_status/calendar.html", []string{`id="absorb-tabs"`, `/stats`}},
		{"web/templates/quotes/list.html", []string{`id="absorb-tabs"`, `/orders`, `/contracts`}},
		{"web/templates/orders/list.html", []string{`id="absorb-tabs"`, `/quotes`, `/contracts`}},
		{"web/templates/contracts/list.html", []string{`id="absorb-tabs"`, `/quotes`, `/orders`}},
		{"web/templates/leave/list.html", []string{`id="absorb-tabs"`, `/approvals/inbox`}},
		{"web/templates/leave/inbox.html", []string{`id="absorb-tabs"`, `/leaves`}},
	}
	for _, c := range checks {
		b, err := os.ReadFile(c.file)
		if err != nil {
			b, err = os.ReadFile(filepath.Join("..", "..", c.file))
		}
		if err != nil {
			t.Fatalf("%s: %v", c.file, err)
		}
		s := string(b)
		for _, n := range c.need {
			if !strings.Contains(s, n) {
				t.Errorf("%s 에 %s 없음", c.file, n)
			}
		}
	}
}

func TestAbsorbedMenuAddressesStillOpen(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "absorb.db"))
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
	g.GET("/plan/unplanned", h.Work.UnplannedList)
	g.GET("/work-status", h.WorkStatus.Calendar)
	g.GET("/stats", h.Stats.Overview)
	g.GET("/as/stats", h.AS.StatsDashboard)
	g.GET("/quotes", h.Quotes.List)
	g.GET("/orders", h.Orders.List)
	g.GET("/contracts", h.Project.Contracts)
	g.GET("/leaves", h.Leave.List)
	g.GET("/approvals/inbox", h.Leave.Inbox)
	g.GET("/admin-work", h.AdminWork.List)
	g.GET("/admin-work/stats", h.AdminWork.Stats)
	ck := jwtCookie(t)
	for _, path := range []string{
		"/plan/unplanned", "/work-status", "/stats", "/as/stats",
		"/quotes", "/orders", "/contracts", "/leaves", "/approvals/inbox",
		"/admin-work", "/admin-work/stats",
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://localhost"+path, nil)
		req.AddCookie(ck)
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("%s status=%d body=%s", path, rec.Code, rec.Body.String())
		}
	}
}

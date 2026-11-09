package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
	"customer-support/internal/service/approval"
)

func new95_159LeaveApp(t *testing.T) (*echo.Echo, *Handler, *model.User, *model.User, *model.User) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "leave159.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	head := &model.User{Username: "h159", PasswordHash: HashPassword("pw"), FullName: "팀장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01", ProfileDone: true}
	admin := &model.User{Username: "a159", PasswordHash: HashPassword("pw"), FullName: "가관리자", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01", ProfileDone: true}
	staff := &model.User{Username: "s159", PasswordHash: HashPassword("pw"), FullName: "직원", Role: model.RoleTech, IsActive: true, OrgID: "O01", ProfileDone: true}
	for _, u := range []*model.User{head, admin, staff} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	h := New(db)
	if err := h.Leave.eng.SetOrgHead(approval.Actor{UserID: head.UserID, OrgID: "O01", Role: model.RoleOrgAdmin}, head.UserID, "O01"); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/leaves/new", h.Leave.New)
	g.POST("/leaves", h.Leave.Create)
	g.GET("/leaves/:id", h.Leave.Show)
	g.GET("/admin/approval-settings", h.Approval.Page, h.Auth.RequireAdminOnly)
	g.POST("/admin/approval-settings", h.Approval.Save, h.Auth.RequireAdminOnly)
	g.POST("/admin/approval-settings/head", h.Approval.HeadSave, h.Auth.RequireAdminOnly)
	return e, h, head, admin, staff
}

func Test95_159_FormApproverFollowsHeadThenOrgAdmin(t *testing.T) {
	e, h, head, admin, staff := new95_159LeaveApp(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/leaves/new", nil)
	req.AddCookie(jwtCookieUserClaims(t, staff))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "data-leave-approver") || !strings.Contains(rec.Body.String(), "팀장") {
		t.Fatalf("head name %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "data-leave-remain") {
		t.Fatal("연차 잔여 칸이 빠졌다")
	}
	if err := h.Leave.eng.ClearOrgHead(approval.Actor{UserID: head.UserID, OrgID: "O01", Role: model.RoleOrgAdmin}, "O01"); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/leaves/new", nil)
	req.AddCookie(jwtCookieUserClaims(t, staff))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "가관리자") {
		t.Fatalf("fallback name %d %s", rec.Code, rec.Body.String())
	}
	_ = admin
}

func Test95_159_SubmitWithoutHeadGoesToOrgAdmin(t *testing.T) {
	e, h, head, admin, staff := new95_159LeaveApp(t)
	if err := h.Leave.eng.ClearOrgHead(approval.Actor{UserID: head.UserID, OrgID: "O01", Role: model.RoleOrgAdmin}, "O01"); err != nil {
		t.Fatal(err)
	}
	form := url.Values{"type_key": {"annual"}, "start_date": {"2026-08-10"}, "reason": {"폴백상신"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/leaves", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieUserClaims(t, staff))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "/leaves/LR-") {
		t.Fatalf("%d %s", rec.Code, rec.Header().Get("Location"))
	}
	inbox, err := h.Leave.eng.ListInbox("O01", admin.UserID, "2026-08-10")
	if err != nil || len(inbox) != 1 {
		t.Fatalf("inbox=%d err=%v", len(inbox), err)
	}
}

func Test95_159_BrowserHeadAndApprover(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	e, _, head, _, staff := new95_159LeaveApp(t)
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	shot := filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-159")
	shot, _ = filepath.Abs(shot)
	cmd := exec.Command("node", "95_159_leave_approver_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(),
		"PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1",
		"BASE_URL="+srv.URL,
		"STAFF_COOKIE="+jwtCookieVerified(t, staff).Value,
		"HEAD_COOKIE="+jwtCookieVerified(t, head).Value,
		"SCREENSHOT_DIR="+shot,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		s := string(out)
		if strings.Contains(s, "Executable doesn't exist") || strings.Contains(s, "browserType.launch") || (strings.Contains(s, "playwright") && strings.Contains(s, "install")) {
			t.Skip(s)
		}
		t.Fatalf("browser: %v\n%s", err, s)
	}
}

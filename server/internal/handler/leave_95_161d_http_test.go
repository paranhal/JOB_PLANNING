package handler

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
	"customer-support/internal/service/leaveimport"
	"customer-support/internal/service/leaveledger"
)

func mountLeave161D(e *echo.Echo, h *Handler) {
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/admin/leaves/import", h.Leave.ImportForm)
	g.GET("/admin/leaves/import.xlsx", h.Leave.ImportTemplate)
	g.POST("/admin/leaves/import/preview", h.Leave.ImportPreview)
	g.POST("/admin/leaves/import/confirm", h.Leave.ImportConfirm)
	g.GET("/leaves", h.Leave.List)
}

func Test95_161D_ImportForbiddenStaff(t *testing.T) {
	e, h, _, staff := new95_161LeaveApp(t)
	mountLeave161D(e, h)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/leaves/import", nil)
	req.AddCookie(jwtCookieUserClaims(t, staff))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("%d", rec.Code)
	}
}

func Test95_161D_ImportPreviewConfirm(t *testing.T) {
	e, h, head, _ := new95_161LeaveApp(t)
	mountLeave161D(e, h)
	h.Leave.svc.Now = func() time.Time { return time.Date(2026, 6, 1, 9, 0, 0, 0, time.Local) }
	users := repository.NewUserRepo(h.Leave.svc.DB)
	a := &model.User{Username: "ga161d", PasswordHash: HashPassword("pw"), FullName: "가상갑", Role: model.RoleTech, IsActive: true, OrgID: "O01", ProfileDone: true}
	if err := users.Create(a); err != nil {
		t.Fatal(err)
	}
	raw, err := leaveimport.BuildFixture(2026)
	if err != nil {
		t.Fatal(err)
	}
	rec := postLeaveXLSX(t, e, head, "/admin/leaves/import/preview", raw)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "가상갑") {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "unmatched") || !strings.Contains(rec.Body.String(), "unregistered") {
		t.Fatal("status missing")
	}
	if !strings.Contains(rec.Body.String(), "확인 필요") {
		t.Fatal("parse error missing")
	}
	tok := extractImportToken(rec.Body.String())
	if tok == "" {
		t.Fatal("token")
	}
	form := url.Values{"token": {tok}}
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/leaves/import/confirm", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieUserClaims(t, head))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("confirm %s", rec.Header().Get("Location"))
	}
	led := leaveledger.New(h.Leave.svc.DB)
	led.Now = h.Leave.svc.Now
	bal, err := led.Balance(a.UserID, "2026-06-01")
	if err != nil || !bal.Measurable || bal.AvailableMinutes != leaveimport.DaysToMinutes(14.5) {
		t.Fatalf("remain %+v err=%v", bal, err)
	}
}

func Test95_161D_BrowserImport(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	e, h, head, _ := new95_161LeaveApp(t)
	mountLeave161D(e, h)
	h.Leave.svc.Now = func() time.Time { return time.Date(2026, 6, 1, 9, 0, 0, 0, time.Local) }
	users := repository.NewUserRepo(h.Leave.svc.DB)
	a := &model.User{Username: "ga161db", PasswordHash: HashPassword("pw"), FullName: "가상갑", Role: model.RoleTech, IsActive: true, OrgID: "O01", ProfileDone: true}
	if err := users.Create(a); err != nil {
		t.Fatal(err)
	}
	raw, err := leaveimport.BuildFixture(2026)
	if err != nil {
		t.Fatal(err)
	}
	fx := filepath.Join(t.TempDir(), "fixture.xlsx")
	if err := os.WriteFile(fx, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	shot := filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-161D")
	shot, _ = filepath.Abs(shot)
	cmd := exec.Command("node", "95_161d_leave_import_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(),
		"PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1",
		"BASE_URL="+srv.URL,
		"HEAD_COOKIE="+jwtCookieVerified(t, head).Value,
		"XLSX_PATH="+fx,
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

func postLeaveXLSX(t *testing.T, e *echo.Echo, u *model.User, path string, raw []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", "fixture.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write(raw)
	_ = w.Close()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set(echo.HeaderContentType, w.FormDataContentType())
	req.AddCookie(jwtCookieUserClaims(t, u))
	e.ServeHTTP(rec, req)
	return rec
}

func extractImportToken(body string) string {
	m := regexp.MustCompile(`name="token" value="([^"]+)"`).FindStringSubmatch(body)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

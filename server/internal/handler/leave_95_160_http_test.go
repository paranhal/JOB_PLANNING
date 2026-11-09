package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
	"customer-support/internal/service/approval"
)

func new95_160LeaveApp(t *testing.T) (*echo.Echo, *Handler, *model.User, *model.User) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "leave160.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	head := &model.User{Username: "h160", PasswordHash: HashPassword("pw"), FullName: "팀장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01", ProfileDone: true}
	staff := &model.User{Username: "s160", PasswordHash: HashPassword("pw"), FullName: "직원", Role: model.RoleTech, IsActive: true, OrgID: "O01", ProfileDone: true}
	for _, u := range []*model.User{head, staff} {
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
	g.GET("/leaves/:id/hwpx", h.Leave.Download)
	g.GET("/approvals/inbox", h.Leave.Inbox)
	g.GET("/approvals/:id", h.Leave.ApprovalShow)
	g.POST("/approvals/:id/approve", h.Leave.Approve)
	return e, h, head, staff
}

func Test95_160_HTTPApproveShowsActedAt(t *testing.T) {
	e, h, head, staff := new95_160LeaveApp(t)
	form := url.Values{
		"type_key": {"annual"}, "period_mode": {"days"},
		"start_date": {"2026-11-20"}, "end_date": {"2026-11-20"}, "reason": {"160HTTP일시"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/leaves", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieUserClaims(t, staff))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create %d %s", rec.Code, rec.Body.String())
	}
	leavePath := strings.Split(rec.Header().Get("Location"), "?")[0]
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, leavePath, nil)
	req.AddCookie(jwtCookieUserClaims(t, staff))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `data-stamp-at`) {
		t.Fatalf("상신 작성일시 %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "승인 공란") {
		t.Fatal("승인 공란")
	}
	var arID string
	_ = h.Leave.svc.DB.QueryRow(`SELECT request_id FROM approval_requests WHERE doc_type='leave' ORDER BY submitted_at DESC LIMIT 1`).Scan(&arID)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/approvals/"+arID+"/approve", strings.NewReader(""))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieUserClaims(t, head))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("approve %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, leavePath, nil)
	req.AddCookie(jwtCookieUserClaims(t, staff))
	e.ServeHTTP(rec, req)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || strings.Contains(body, "승인 공란") {
		t.Fatalf("승인 후 %d", rec.Code)
	}
	if !regexp.MustCompile(`data-stamp-at>[0-9]{4}-[0-9]{2}-[0-9]{2} [0-9]{2}:[0-9]{2}<`).MatchString(body) {
		t.Fatalf("승인 일시 없음")
	}
	if !strings.Contains(body, "팀장") {
		t.Fatal("이름")
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, leavePath+"/hwpx", nil)
	req.AddCookie(jwtCookieUserClaims(t, staff))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.Len() < 100 {
		t.Fatalf("hwpx %d", rec.Code)
	}
}

func Test95_160_HTTPAdminEmergencyActedAt(t *testing.T) {
	e, h, head, staff := new95_160LeaveApp(t)
	users := repository.NewUserRepo(h.Leave.svc.DB)
	mgr := &model.User{Username: "m160", PasswordHash: HashPassword("pw"), FullName: "관리자", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01", ProfileDone: true}
	if err := users.Create(mgr); err != nil {
		t.Fatal(err)
	}
	p, err := h.Leave.eng.GetPolicy("O01")
	if err != nil {
		t.Fatal(err)
	}
	p.AdminEmergencyApprove = true
	if err := h.Leave.eng.SavePolicy(approval.Actor{UserID: head.UserID, Role: model.RoleOrgAdmin, OrgID: "O01"}, "O01", p); err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"type_key": {"annual"}, "period_mode": {"days"},
		"start_date": {"2026-11-23"}, "end_date": {"2026-11-23"}, "reason": {"160긴급일시"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/leaves", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieUserClaims(t, staff))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create %d %s", rec.Code, rec.Body.String())
	}
	leavePath := strings.Split(rec.Header().Get("Location"), "?")[0]
	if !strings.HasPrefix(leavePath, "/leaves/LR-") {
		t.Fatalf("loc %s", rec.Header().Get("Location"))
	}
	leaveID := strings.TrimPrefix(leavePath, "/leaves/")
	var arID string
	_ = h.Leave.svc.DB.QueryRow(`SELECT approval_request_id FROM leave_requests WHERE leave_req_id=?`, leaveID).Scan(&arID)
	if arID == "" {
		t.Fatal("approval_request_id")
	}
	show := httptest.NewRecorder()
	sreq := httptest.NewRequest(http.MethodGet, "/approvals/"+arID, nil)
	sreq.AddCookie(jwtCookieUserClaims(t, mgr))
	e.ServeHTTP(show, sreq)
	if show.Code != http.StatusOK || !strings.Contains(show.Body.String(), `name="emergency_reason"`) {
		t.Fatalf("긴급 사유 칸 %d", show.Code)
	}
	af := url.Values{"emergency_reason": {"팀장 출장"}}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/approvals/"+arID+"/approve", strings.NewReader(af.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieUserClaims(t, mgr))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("emergency %d %s", rec.Code, rec.Header().Get("Location"))
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, leavePath, nil)
	req.AddCookie(jwtCookieUserClaims(t, staff))
	e.ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, "관리자") || !strings.Contains(body, "(대결)") {
		t.Fatalf("긴급 표시 %s", body)
	}
	if !regexp.MustCompile(`data-stamp-at>[0-9]{4}-[0-9]{2}-[0-9]{2} [0-9]{2}:[0-9]{2}<`).MatchString(body) {
		t.Fatal("긴급 일시")
	}
}

func Test95_160_BrowserSubmitApproveStamp(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	e, _, head, staff := new95_160LeaveApp(t)
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	shot := filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-160")
	shot, _ = filepath.Abs(shot)
	cmd := exec.Command("node", "95_160_leave_stamp_browser.mjs")
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

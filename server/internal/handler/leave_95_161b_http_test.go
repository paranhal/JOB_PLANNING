package handler

import (
	"encoding/json"
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
	"customer-support/internal/service/leaveapp"
	"customer-support/internal/service/leaveledger"
)

func new95_161LeaveApp(t *testing.T) (*echo.Echo, *Handler, *model.User, *model.User) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "leave161b.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	head := &model.User{Username: "h161b", PasswordHash: HashPassword("pw"), FullName: "팀장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01", ProfileDone: true}
	staff := &model.User{Username: "s161b", PasswordHash: HashPassword("pw"), FullName: "직원", Role: model.RoleTech, IsActive: true, OrgID: "O01", ProfileDone: true}
	for _, u := range []*model.User{head, staff} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	h := New(db)
	if err := h.Leave.eng.SetOrgHead(approval.Actor{UserID: head.UserID, Role: model.RoleOrgAdmin}, head.UserID, "O01"); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/leaves", h.Leave.List)
	g.GET("/leaves/new", h.Leave.New)
	g.GET("/leaves/preview", h.Leave.Preview)
	g.POST("/leaves", h.Leave.Create)
	g.GET("/leaves/:id", h.Leave.Show)
	g.POST("/leaves/:id/cancel", h.Leave.Cancel)
	g.GET("/approvals/inbox", h.Leave.Inbox)
	g.GET("/approvals/:id", h.Leave.ApprovalShow)
	g.POST("/approvals/:id/approve", h.Leave.Approve)
	return e, h, head, staff
}

func Test95_161B_ListUnregisteredDash(t *testing.T) {
	e, _, _, staff := new95_161LeaveApp(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/leaves", nil)
	req.AddCookie(jwtCookieUserClaims(t, staff))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "— (연차 미등록)") {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
}

func Test95_161B_FormRemainAndPreviewShortage(t *testing.T) {
	e, h, _, staff := new95_161LeaveApp(t)
	led := leaveledger.New(h.Leave.svc.DB)
	if err := led.Enable(staff.UserID, "O01", ""); err != nil {
		t.Fatal(err)
	}
	if err := led.AddOpening(staff.UserID, "2026-01-01", "2026-12-31", 480, "1"); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/leaves/new", nil)
	req.AddCookie(jwtCookieUserClaims(t, staff))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "1일") {
		t.Fatalf("form remain %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "absence_consent") {
		t.Fatal("consent field")
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/leaves/preview?type_key=annual&period_mode=days&start_date=2026-08-10&end_date=2026-08-14", nil)
	req.AddCookie(jwtCookieUserClaims(t, staff))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview %d", rec.Code)
	}
	var pv leaveapp.Preview
	if err := json.Unmarshal(rec.Body.Bytes(), &pv); err != nil {
		t.Fatal(err)
	}
	if !pv.Shortage || !strings.Contains(pv.Message, "결근 처리") {
		t.Fatalf("%+v", pv)
	}
	form := url.Values{
		"type_key": {"annual"}, "period_mode": {"days"},
		"start_date": {"2026-08-10"}, "end_date": {"2026-08-14"}, "reason": {"부족테스트"},
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/leaves", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieUserClaims(t, staff))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("no consent loc=%s", rec.Header().Get("Location"))
	}
	form.Set("absence_consent", "1")
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/leaves", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieUserClaims(t, staff))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || !strings.HasPrefix(strings.Split(rec.Header().Get("Location"), "?")[0], "/leaves/LR-") {
		t.Fatalf("with consent %s", rec.Header().Get("Location"))
	}
}

func Test95_161B_ApproveReducesThenCancelRestores(t *testing.T) {
	e, h, head, staff := new95_161LeaveApp(t)
	led := leaveledger.New(h.Leave.svc.DB)
	if err := led.Enable(staff.UserID, "O01", "2026-01-01"); err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"type_key": {"annual"}, "period_mode": {"days"},
		"start_date": {"2026-08-10"}, "end_date": {"2026-08-10"}, "reason": {"잔여감소"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/leaves", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieUserClaims(t, staff))
	e.ServeHTTP(rec, req)
	leavePath := strings.Split(rec.Header().Get("Location"), "?")[0]
	leaveID := strings.TrimPrefix(leavePath, "/leaves/")
	var arID string
	_ = h.Leave.svc.DB.QueryRow(`SELECT approval_request_id FROM leave_requests WHERE leave_req_id=?`, leaveID).Scan(&arID)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/approvals/"+arID+"/approve", strings.NewReader(""))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieUserClaims(t, head))
	e.ServeHTTP(rec, req)
	sn, err := led.Snapshot(staff.UserID, "2026-08-10")
	if err != nil || sn.Balance.AvailableMinutes != 14*480 {
		t.Fatalf("after approve %+v err=%v", sn, err)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/leaves/"+leaveID+"/cancel", nil)
	req.AddCookie(jwtCookieUserClaims(t, staff))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("cancel %d", rec.Code)
	}
	sn, err = led.Snapshot(staff.UserID, "2026-08-10")
	if err != nil || sn.Balance.AvailableMinutes != 15*480 {
		t.Fatalf("after cancel %+v", sn)
	}
}

func Test95_161B_BrowserRemainConsentCancel(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	e, h, head, staff := new95_161LeaveApp(t)
	led := leaveledger.New(h.Leave.svc.DB)
	if err := led.Enable(staff.UserID, "O01", ""); err != nil {
		t.Fatal(err)
	}
	if err := led.AddOpening(staff.UserID, "2026-01-01", "2026-12-31", 5*480, "1"); err != nil {
		t.Fatal(err)
	}
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	shot := filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-161B")
	shot, _ = filepath.Abs(shot)
	cmd := exec.Command("node", "95_161b_leave_remain_browser.mjs")
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

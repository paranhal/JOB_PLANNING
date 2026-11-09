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
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
	"customer-support/internal/service/leaveledger"
)

func mountLeave161C(e *echo.Echo, h *Handler) {
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/leaves/team", h.Leave.Team)
	g.GET("/admin/leaves", h.Leave.Admin)
	g.GET("/admin/leaves.xlsx", h.Leave.AbsenceXLSX)
	g.POST("/admin/leaves/hire", h.Leave.SetHire)
	g.POST("/admin/leaves/auto", h.Leave.SetAuto)
	g.POST("/admin/leaves/adjust", h.Leave.Adjust)
}

func Test95_161C_TeamForbiddenForStaff(t *testing.T) {
	e, h, _, staff := new95_161LeaveApp(t)
	mountLeave161C(e, h)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/leaves/team", nil)
	req.AddCookie(jwtCookieUserClaims(t, staff))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("staff team %d", rec.Code)
	}
}

func Test95_161C_TeamTableForHead(t *testing.T) {
	e, h, head, staff := new95_161LeaveApp(t)
	mountLeave161C(e, h)
	led := leaveledger.New(h.Leave.svc.DB)
	if err := led.Enable(staff.UserID, "O01", "2020-03-01"); err != nil {
		t.Fatal(err)
	}
	if err := led.AddOpening(staff.UserID, "2026-01-01", "2026-12-31", 5*480, "1"); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/leaves/team", nil)
	req.AddCookie(jwtCookieUserClaims(t, head))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), staff.FullName) {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "발생") || !strings.Contains(rec.Body.String(), "잔여") {
		t.Fatal("columns")
	}
}

func Test95_161C_AdminForbiddenStaffAndReadOnly(t *testing.T) {
	e, h, _, staff := new95_161LeaveApp(t)
	mountLeave161C(e, h)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/leaves", nil)
	req.AddCookie(jwtCookieUserClaims(t, staff))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("staff admin %d", rec.Code)
	}
	ro := &model.User{Username: "ro161c", PasswordHash: HashPassword("pw"), FullName: "읽기관리", Role: model.RoleOrgAdmin, IsActive: true, IsReadOnly: true, OrgID: "O01", ProfileDone: true}
	if err := repository.NewUserRepo(h.Leave.svc.DB).Create(ro); err != nil {
		t.Fatal(err)
	}
	form := url.Values{"user_id": {staff.UserID}, "hire_date": {"2020-01-01"}}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/admin/leaves/hire", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieUserClaims(t, ro))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("readonly write %d", rec.Code)
	}
}

func Test95_161C_AdminHireAutoAdjustHistoryXLSX(t *testing.T) {
	e, h, head, staff := new95_161LeaveApp(t)
	mountLeave161C(e, h)
	h.Leave.svc.Now = func() time.Time { return time.Date(2026, 8, 10, 9, 0, 0, 0, time.Local) }

	form := url.Values{"user_id": {staff.UserID}, "hire_date": {"2020-03-01"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/leaves/hire", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieUserClaims(t, head))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("hire %d", rec.Code)
	}

	form = url.Values{"user_id": {staff.UserID}, "auto": {"1"}}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/admin/leaves/auto", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieUserClaims(t, head))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("auto %d", rec.Code)
	}

	form = url.Values{"user_id": {staff.UserID}, "days": {"1"}, "note": {"테스트부여"}}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/admin/leaves/adjust", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieUserClaims(t, head))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("adjust %s", rec.Header().Get("Location"))
	}

	form = url.Values{"user_id": {staff.UserID}, "days": {"1"}, "note": {"이월"}, "expires_on": {"2027-12-31"}}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/admin/leaves/adjust", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieUserClaims(t, head))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("carry without approver %s", rec.Header().Get("Location"))
	}
	form.Set("carry_approver", "김승인")
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/admin/leaves/adjust", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieUserClaims(t, head))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("carry %s", rec.Header().Get("Location"))
	}

	_, _ = h.Leave.svc.DB.Exec(`INSERT INTO leave_absence_records(absence_id,user_id,leave_req_id,minutes,kind,note,created_at)
		VALUES('AB-1',?, '', 480, 'shortage', '테스트', '2026-08-10 09:00:00')`, staff.UserID)

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/admin/leaves?month=2026-08&user_id="+url.QueryEscape(staff.UserID), nil)
	req.AddCookie(jwtCookieUserClaims(t, head))
	e.ServeHTTP(rec, req)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "2020-03-01") || !strings.Contains(body, "테스트부여") {
		t.Fatalf("admin page %d %s", rec.Code, body)
	}
	if !strings.Contains(body, "연차 미등록") || !strings.Contains(body, "입사일 누락") || !strings.Contains(body, "퇴사 정산표") {
		t.Fatal("sections")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/admin/leaves.xlsx?month=2026-08", nil)
	req.AddCookie(jwtCookieUserClaims(t, head))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Type"), "spreadsheet") {
		t.Fatalf("xlsx %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	if rec.Body.Len() < 100 {
		t.Fatal("xlsx empty")
	}
}

func Test95_161C_OrgAdminDoesNotSeeOtherOrg(t *testing.T) {
	e, h, head, _ := new95_161LeaveApp(t)
	mountLeave161C(e, h)
	other := &model.User{Username: "o161c", PasswordHash: HashPassword("pw"), FullName: "다른팀원", Role: model.RoleTech, IsActive: true, OrgID: "O02", ProfileDone: true}
	if err := repository.NewUserRepo(h.Leave.svc.DB).Create(other); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/leaves/team", nil)
	req.AddCookie(jwtCookieUserClaims(t, head))
	e.ServeHTTP(rec, req)
	if strings.Contains(rec.Body.String(), "다른팀원") {
		t.Fatal("other org visible")
	}
	form := url.Values{"user_id": {other.UserID}, "hire_date": {"2020-01-01"}}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/admin/leaves/hire", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieUserClaims(t, head))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("cross org %d", rec.Code)
	}
}

func Test95_161C_BrowserTeamAdmin(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	e, h, head, staff := new95_161LeaveApp(t)
	mountLeave161C(e, h)
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	shot := filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-161C")
	shot, _ = filepath.Abs(shot)
	cmd := exec.Command("node", "95_161c_leave_admin_browser.mjs")
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

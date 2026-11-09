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
)

func postLeaveForm(t *testing.T, e *echo.Echo, staffCookie *http.Cookie, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/leaves", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(staffCookie)
	e.ServeHTTP(rec, req)
	return rec
}

func Test95_163_CreateKeepsInputOnError(t *testing.T) {
	e, _, head, staff := new95_161LeaveApp(t)
	ck := jwtCookieUserClaims(t, staff)
	form := url.Values{
		"type_key": {"annual"}, "start_date": {"2026-08-10"}, "end_date": {"2026-08-11"},
		"reason": {""}, "destination": {"현장A"}, "contact_phone": {"010-1111-2222"},
		"deputy_user_id": {head.UserID},
	}
	rec := postLeaveForm(t, e, ck, form)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("reason %d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cache %q", rec.Header().Get("Cache-Control"))
	}
	body := rec.Body.String()
	if !strings.Contains(body, "사유를 입력하세요") || strings.Contains(rec.Header().Get("Location"), "/leaves/new") {
		t.Fatalf("redirect or msg %s", body)
	}
	if !strings.Contains(body, `value="annual"`) || !strings.Contains(body, "2026-08-10") || !strings.Contains(body, "2026-08-11") {
		t.Fatal("기간 유지")
	}
	if !strings.Contains(body, "현장A") || !strings.Contains(body, "010-1111-2222") || !strings.Contains(body, head.UserID) {
		t.Fatalf("연락처·대행 %s", body)
	}

	form.Set("reason", "겹침유지")
	ok := postLeaveForm(t, e, ck, form)
	if ok.Code != http.StatusSeeOther {
		t.Fatalf("first ok %d %s", ok.Code, ok.Header().Get("Location"))
	}
	dup := postLeaveForm(t, e, ck, form)
	if dup.Code != http.StatusUnprocessableEntity || !strings.Contains(dup.Body.String(), "이미 신청") {
		t.Fatalf("overlap %d %s", dup.Code, dup.Body.String())
	}
	if !strings.Contains(dup.Body.String(), "겹침유지") || !strings.Contains(dup.Body.String(), "현장A") {
		t.Fatal("겹침 후 사유 유지")
	}
}

func Test95_163_HalfAndEarlyLeaveKeepComputedTimes(t *testing.T) {
	e, _, _, staff := new95_161LeaveApp(t)
	ck := jwtCookieUserClaims(t, staff)
	half := url.Values{
		"type_key": {"half"}, "start_date": {"2026-08-10"}, "start_time": {"12:30"},
		"reason": {""}, "destination": {"반차현장"},
	}
	rec := postLeaveForm(t, e, ck, half)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("half %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `value="half"`) || !strings.Contains(body, "checked") {
		t.Fatal("반차 선택 유지")
	}
	if !strings.Contains(body, "13:00") || !strings.Contains(body, "17:00") || !strings.Contains(body, "오후") {
		t.Fatalf("반차 서버 계산 %s", body)
	}
	if !strings.Contains(body, "반차현장") {
		t.Fatal("반차 행선지")
	}

	early := url.Values{
		"type_key": {"early_leave"}, "start_date": {"2026-08-11"},
		"start_time": {"14:00"}, "end_time": {"11:00"}, "reason": {"조퇴시각"},
	}
	rec = postLeaveForm(t, e, ck, early)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("early %d", rec.Code)
	}
	body = rec.Body.String()
	if !strings.Contains(body, `value="early_leave"`) || !strings.Contains(body, "14:00") || !strings.Contains(body, "11:00") {
		t.Fatalf("조퇴 시각 유지 %s", body)
	}
	if !strings.Contains(body, "조퇴시각") {
		t.Fatal("조퇴 사유")
	}
}

func Test95_163_BrowserKeepThenResubmit(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	e, _, head, staff := new95_161LeaveApp(t)
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	shot := filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-163")
	shot, _ = filepath.Abs(shot)
	cmd := exec.Command("node", "95_163_leave_keep_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(),
		"PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1",
		"BASE_URL="+srv.URL,
		"STAFF_COOKIE="+jwtCookieVerified(t, staff).Value,
		"HEAD_ID="+head.UserID,
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

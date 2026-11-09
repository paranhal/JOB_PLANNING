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

func Test95_162_FormHasNoAlwaysRequiredHoursDate(t *testing.T) {
	e, _, _, staff := new95_161LeaveApp(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/leaves/new", nil)
	req.AddCookie(jwtCookieUserClaims(t, staff))
	e.ServeHTTP(rec, req)
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("%d", rec.Code)
	}
	if strings.Contains(body, "시간 (반차") || strings.Contains(body, "일 단위") {
		t.Fatal("기간 라디오가 남아 있다")
	}
	if !strings.Contains(body, `data-days-start`) || !strings.Contains(body, `data-half-time`) {
		t.Fatal("구분별 기간 칸이 없다")
	}
}

func Test95_162_AnnualDateOnlySubmit(t *testing.T) {
	e, _, _, staff := new95_161LeaveApp(t)
	form := url.Values{"type_key": {"annual"}, "period_mode": {"hours"}, "start_date": {"2026-08-10"}, "reason": {"날짜만"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/leaves", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieUserClaims(t, staff))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "/leaves/LR-") {
		t.Fatalf("%d %s", rec.Code, rec.Header().Get("Location"))
	}
}

func Test95_162_HalfServerTimesHTTP(t *testing.T) {
	e, h, _, staff := new95_161LeaveApp(t)
	form := url.Values{"type_key": {"half"}, "start_date": {"2026-08-11"}, "start_time": {"09:00"}, "end_time": {"18:00"}, "half_part": {"pm"}, "reason": {"반차서버"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/leaves", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieUserClaims(t, staff))
	e.ServeHTTP(rec, req)
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || !strings.Contains(loc, "/leaves/LR-") {
		t.Fatalf("%d %s", rec.Code, loc)
	}
	id := strings.TrimPrefix(loc, "/leaves/")
	if i := strings.IndexByte(id, '?'); i >= 0 {
		id = id[:i]
	}
	got, err := h.Leave.svc.Get(id)
	if err != nil || got == nil || got.EndTime != "14:00" || got.HalfPart != "am" {
		t.Fatalf("%+v err=%v id=%s", got, err, id)
	}
}

func Test95_162_BrowserFormPeriods(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	e, h, _, staff := new95_161LeaveApp(t)
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	shot := filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-162")
	shot, _ = filepath.Abs(shot)
	cmd := exec.Command("node", "95_162_leave_form_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(),
		"PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1",
		"BASE_URL="+srv.URL,
		"STAFF_COOKIE="+jwtCookieVerified(t, staff).Value,
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
	_ = h
}

package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func newFlashErrorEcho() *echo.Echo {
	e := echo.New()
	e.Renderer = NewRenderer()
	AttachHTTPErrorHandler(e)
	e.POST("/demo/err", func(c echo.Context) error {
		return echo.NewHTTPError(http.StatusBadRequest, "작업명을 입력하세요")
	})
	e.POST("/demo/boom", func(c echo.Context) error {
		return echo.NewHTTPError(http.StatusInternalServerError, "sql: secret-detail")
	})
	e.GET("/demo/back", func(c echo.Context) error {
		return c.Render(http.StatusOK, "error/not_found.html", map[string]interface{}{
			"Title": "복귀", "Active": NavNone, "HideNav": true,
		})
	})
	return e
}

func htmlFormPOST(e *echo.Echo, path string, form url.Values, referer string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost"+path, strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.Header.Set(echo.HeaderAccept, "text/html,application/xhtml+xml")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	e.ServeHTTP(rec, req)
	return rec
}

func TestHTMLFormHTTPErrorFlashRedirect(t *testing.T) {
	e := newFlashErrorEcho()
	rec := htmlFormPOST(e, "/demo/err", url.Values{"back": {"/demo/back"}}, "http://localhost/demo/back")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get(echo.HeaderLocation); loc != "/demo/back" {
		t.Fatalf("location=%q", loc)
	}
	if strings.Contains(rec.Body.String(), `"message"`) {
		t.Fatal("HTML 폼 오류가 JSON 본문")
	}
	ck := flashCookieFrom(rec)
	if ck == nil || strings.TrimSpace(ck.Value) == "" {
		t.Fatal("flash 쿠키 없음")
	}
	if ck.HttpOnly != true || ck.SameSite != http.SameSiteLaxMode {
		t.Fatalf("쿠키 속성 HttpOnly=%v SameSite=%v", ck.HttpOnly, ck.SameSite)
	}

	page := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/demo/back", nil)
	req.AddCookie(ck)
	e.ServeHTTP(page, req)
	body := page.Body.String()
	if !strings.Contains(body, "확인해 주세요") || !strings.Contains(body, "작업명을 입력하세요") {
		t.Fatalf("팝업 없음: %s", clipBody(body))
	}
	if !strings.Contains(body, `role="alertdialog"`) {
		t.Fatal("alertdialog 없음")
	}
	if strings.Contains(body, "<script>작업명") {
		t.Fatal("메시지가 스크립트로 삽입됨")
	}

	cleared := false
	for _, c := range page.Result().Cookies() {
		if c.Name == flashErrCookie && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("flash 쿠키가 1회 후 삭제되지 않음")
	}

	again := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "http://localhost/demo/back", nil)
	e.ServeHTTP(again, req2)
	if strings.Contains(again.Body.String(), "작업명을 입력하세요") {
		t.Fatal("쿠키 없이 팝업이 남음")
	}
}

func TestHTMLFormHTTPErrorIgnoresExternalBack(t *testing.T) {
	e := newFlashErrorEcho()
	rec := htmlFormPOST(e, "/demo/err", url.Values{"back": {"https://evil.example/phish"}}, "http://localhost/demo/back")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d", rec.Code)
	}
	loc := rec.Header().Get(echo.HeaderLocation)
	if strings.Contains(loc, "evil") || strings.HasPrefix(loc, "//") {
		t.Fatalf("외부 back 허용: %s", loc)
	}
	if loc != "/demo/back" {
		t.Fatalf("Referer 복귀 실패 loc=%s", loc)
	}
}

func TestHTMLFormHTTPError500Generic(t *testing.T) {
	e := newFlashErrorEcho()
	rec := htmlFormPOST(e, "/demo/boom", url.Values{"redirect": {"/demo/back"}}, "http://localhost/demo/back")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d", rec.Code)
	}
	ck := flashCookieFrom(rec)
	if ck == nil {
		t.Fatal("flash 없음")
	}
	page := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/demo/back", nil)
	req.AddCookie(ck)
	e.ServeHTTP(page, req)
	body := page.Body.String()
	if !strings.Contains(body, "처리 중 오류가 발생했습니다") {
		t.Fatalf("일반 문구 없음: %s", clipBody(body))
	}
	if strings.Contains(body, "sql: secret") {
		t.Fatal("500 상세가 팝업에 노출")
	}
}

func TestJSONHTTPErrorStaysJSON(t *testing.T) {
	e := newFlashErrorEcho()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/demo/err", strings.NewReader("title="))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.Header.Set(echo.HeaderAccept, echo.MIMEApplicationJSON)
	req.Header.Set("X-Requested-With", "fetch")
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rec.Code)
	}
	if rec.Header().Get(echo.HeaderLocation) != "" {
		t.Fatal("JSON 요청이 리다이렉트됨")
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json: %v %s", err, rec.Body.String())
	}
	if payload["message"] != "작업명을 입력하세요" {
		t.Fatalf("message=%v", payload["message"])
	}
}

func flashCookieFrom(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == flashErrCookie && c.MaxAge >= 0 && c.Value != "" {
			return c
		}
	}
	return nil
}

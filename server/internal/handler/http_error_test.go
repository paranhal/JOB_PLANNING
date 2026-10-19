package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestHTML404PageAndJSON404(t *testing.T) {
	e := echo.New()
	e.Renderer = NewRenderer()
	AttachHTTPErrorHandler(e)

	htmlRec := httptest.NewRecorder()
	htmlReq := httptest.NewRequest(http.MethodGet, "http://localhost/this-path-does-not-exist", nil)
	htmlReq.Header.Set(echo.HeaderAccept, "text/html,application/xhtml+xml")
	e.ServeHTTP(htmlRec, htmlReq)
	if htmlRec.Code != http.StatusNotFound {
		t.Fatalf("html status=%d", htmlRec.Code)
	}
	body := htmlRec.Body.String()
	if strings.Contains(body, `"message":"Not Found"`) {
		t.Fatal("HTML 요청에 JSON 404")
	}
	if !strings.Contains(body, "페이지를 찾을 수 없습니다") || !strings.Contains(body, `href="/"`) {
		t.Fatalf("404 화면 없음: %s", body[:minInt(400, len(body))])
	}

	jsRec := httptest.NewRecorder()
	jsReq := httptest.NewRequest(http.MethodGet, "http://localhost/this-path-does-not-exist", nil)
	jsReq.Header.Set(echo.HeaderAccept, echo.MIMEApplicationJSON)
	e.ServeHTTP(jsRec, jsReq)
	if jsRec.Code != http.StatusNotFound {
		t.Fatalf("json status=%d", jsRec.Code)
	}
	if !strings.Contains(jsRec.Body.String(), "Not Found") {
		t.Fatalf("JSON 404 본문: %s", jsRec.Body.String())
	}
}

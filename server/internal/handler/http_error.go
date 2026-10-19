package handler

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// AttachHTTPErrorHandler HTML 요청의 404는 레이아웃 화면, JSON은 Echo 기본 본문. D-95
func AttachHTTPErrorHandler(e *echo.Echo) {
	if e == nil {
		return
	}
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}
		code := http.StatusInternalServerError
		if he, ok := err.(*echo.HTTPError); ok {
			code = he.Code
		}
		if code == http.StatusNotFound && wantsHTML(c) {
			data := map[string]interface{}{
				"Title":  "페이지를 찾을 수 없습니다",
				"Active": NavNone,
			}
			if rendErr := c.Render(http.StatusNotFound, "error/not_found.html", data); rendErr == nil {
				return
			}
		}
		e.DefaultHTTPErrorHandler(err, c)
	}
}

func wantsHTML(c echo.Context) bool {
	if c == nil || c.Request() == nil {
		return false
	}
	a := c.Request().Header.Get(echo.HeaderAccept)
	if strings.Contains(a, echo.MIMEApplicationJSON) && !strings.Contains(a, echo.MIMETextHTML) {
		return false
	}
	return strings.Contains(a, echo.MIMETextHTML)
}

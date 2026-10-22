package handler

import (
	"html"
	"log"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/labstack/echo/v4"
)

const (
	flashErrCookie     = "jp_flash_err"
	flashErrMaxRunes   = 300
	flashGeneric500Msg = "처리 중 오류가 발생했습니다"
)

// AttachHTTPErrorHandler HTML 폼 오류는 flash+303, JSON/fetch는 본문 유지, HTML 404 GET은 화면.
func AttachHTTPErrorHandler(e *echo.Echo) {
	if e == nil {
		return
	}
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}
		he, isHE := err.(*echo.HTTPError)
		code := http.StatusInternalServerError
		msg := flashGeneric500Msg
		if isHE {
			code = he.Code
			msg = httpErrorMessage(he)
		} else if err != nil {
			msg = err.Error()
		}
		if wantsJSONError(c) {
			e.DefaultHTTPErrorHandler(err, c)
			return
		}
		if code == http.StatusNotFound && isSafeMethod(c) && wantsHTML(c) {
			data := map[string]interface{}{
				"Title":  "페이지를 찾을 수 없습니다",
				"Active": NavNone,
			}
			if rendErr := c.Render(http.StatusNotFound, "error/not_found.html", data); rendErr == nil {
				return
			}
		}
		if isFormMethod(c) && wantsHTML(c) && flashStatus(code) {
			if code >= 500 {
				log.Printf("폼 오류 500 [%s %s]: %v", c.Request().Method, c.Request().URL.Path, err)
				msg = flashGeneric500Msg
			}
			setFlashErrCookie(c, msg)
			_ = c.Redirect(http.StatusSeeOther, formReturnURL(c))
			return
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

func wantsJSONError(c echo.Context) bool {
	if c == nil || c.Request() == nil {
		return false
	}
	h := c.Request().Header
	if strings.EqualFold(h.Get("HX-Request"), "true") {
		return true
	}
	acc := h.Get(echo.HeaderAccept)
	if strings.Contains(acc, echo.MIMEApplicationJSON) {
		return true
	}
	xrw := strings.ToLower(strings.TrimSpace(h.Get("X-Requested-With")))
	if xrw == "xmlhttprequest" || xrw == "fetch" {
		return true
	}
	path := ""
	if c.Request().URL != nil {
		path = c.Request().URL.Path
	}
	return strings.HasPrefix(path, "/api/") || path == "/api"
}

func isFormMethod(c echo.Context) bool {
	if c == nil || c.Request() == nil {
		return false
	}
	switch strings.ToUpper(c.Request().Method) {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

func isSafeMethod(c echo.Context) bool {
	if c == nil || c.Request() == nil {
		return true
	}
	m := strings.ToUpper(c.Request().Method)
	return m == http.MethodGet || m == http.MethodHead
}

func flashStatus(code int) bool {
	switch code {
	case http.StatusBadRequest, http.StatusForbidden, http.StatusNotFound,
		http.StatusConflict, http.StatusUnprocessableEntity, http.StatusInternalServerError:
		return true
	}
	return code >= 500
}

func httpErrorMessage(he *echo.HTTPError) string {
	if he == nil {
		return ""
	}
	switch m := he.Message.(type) {
	case string:
		return m
	case error:
		return m.Error()
	default:
		if he.Internal != nil {
			return he.Internal.Error()
		}
		return http.StatusText(he.Code)
	}
}

func setFlashErrCookie(c echo.Context, msg string) {
	msg = clipRunes(strings.TrimSpace(html.UnescapeString(msg)), flashErrMaxRunes)
	ck := sessionCookie(flashErrCookie, url.QueryEscape(msg), 120)
	c.SetCookie(ck)
}

func readFlashErrCookie(c echo.Context) string {
	if c == nil || c.Request() == nil {
		return ""
	}
	ck, err := c.Cookie(flashErrCookie)
	if err != nil || ck == nil {
		return ""
	}
	raw, err := url.QueryUnescape(ck.Value)
	if err != nil {
		raw = ck.Value
	}
	return clipRunes(strings.TrimSpace(raw), flashErrMaxRunes)
}

func clearFlashErrCookie(c echo.Context) {
	if c == nil || c.Response().Committed {
		return
	}
	c.SetCookie(sessionCookie(flashErrCookie, "", -1))
}

func injectFlash(c echo.Context, data map[string]interface{}) {
	if data == nil {
		return
	}
	msg := readFlashErrCookie(c)
	if msg != "" {
		data["FlashPopup"] = msg
		clearFlashErrCookie(c)
	}
}

func formReturnURL(c echo.Context) string {
	if c != nil && c.Request() != nil {
		_ = c.Request().ParseForm()
	}
	if u := safeSameOriginPath(c, c.FormValue("back")); u != "" {
		return u
	}
	if u := safeSameOriginPath(c, c.FormValue("redirect")); u != "" {
		return u
	}
	if c != nil && c.Request() != nil {
		if u := safeSameOriginPath(c, c.Request().Referer()); u != "" {
			return u
		}
	}
	return moduleFallback(c)
}

func safeSameOriginPath(c echo.Context, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "//") || strings.Contains(raw, "://") && !strings.HasPrefix(raw, "/") {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return ""
		}
		if strings.EqualFold(u.Scheme, "javascript") || strings.EqualFold(u.Scheme, "data") {
			return ""
		}
		reqHost := ""
		if c != nil && c.Request() != nil {
			reqHost = c.Request().Host
		}
		if !strings.EqualFold(u.Host, reqHost) {
			return ""
		}
		p := u.EscapedPath()
		if p == "" {
			p = "/"
		}
		if !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") {
			return ""
		}
		if u.RawQuery != "" {
			p += "?" + u.RawQuery
		}
		return p
	}
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return ""
	}
	if strings.ContainsAny(raw, "\r\n") {
		return ""
	}
	return raw
}

func moduleFallback(c echo.Context) string {
	path := ""
	if c != nil && c.Request() != nil && c.Request().URL != nil {
		path = c.Request().URL.Path
	}
	switch {
	case strings.HasPrefix(path, "/workboard"):
		return "/workboard/register"
	case strings.HasPrefix(path, "/as"):
		return "/as"
	case strings.HasPrefix(path, "/maintenance"):
		return "/maintenance"
	case strings.HasPrefix(path, "/admin"):
		return "/admin/users"
	case strings.HasPrefix(path, "/projects"):
		return "/projects"
	default:
		return "/"
	}
}

func clipRunes(s string, n int) string {
	if n <= 0 || utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n])
}

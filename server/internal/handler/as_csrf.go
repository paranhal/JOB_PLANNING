package handler

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/config"
)

const csrfCookieName = "jp_csrf"

func randomHexToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// csrfCookie 더블 서브밋용. JS가 읽을 수 있게 HttpOnly 가 아니다(세션 token 과 다름).
func csrfCookie(value string, maxAge int) *http.Cookie {
	ck := &http.Cookie{
		Name:     csrfCookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	}
	if config.Load().CookieSecure {
		ck.Secure = true
	}
	return ck
}

func ensureCSRFToken(c echo.Context) string {
	if c == nil || c.Request() == nil {
		return ""
	}
	ck, err := c.Cookie(csrfCookieName)
	if err == nil && ck != nil && len(strings.TrimSpace(ck.Value)) >= 16 {
		if !c.Response().Committed {
			c.SetCookie(csrfCookie(ck.Value, 0))
		}
		return ck.Value
	}
	tok := randomHexToken()
	if !c.Response().Committed {
		c.SetCookie(csrfCookie(tok, 0))
	}
	return tok
}

// CSRFJSON GET /csrf — 쿠키가 없거나 화면 토큰이 오래됐을 때 재발급.
func CSRFJSON(c echo.Context) error {
	tok := ensureCSRFToken(c)
	return c.JSON(http.StatusOK, map[string]string{"csrf": tok})
}

func requireCSRF(c echo.Context) bool {
	if c == nil || c.Request() == nil {
		return false
	}
	ck, err := c.Cookie(csrfCookieName)
	got := strings.TrimSpace(c.FormValue("csrf"))
	if got == "" && c.Request() != nil {
		got = strings.TrimSpace(c.Request().Header.Get("X-CSRF-Token"))
	}
	if err != nil || ck == nil || strings.TrimSpace(ck.Value) == "" || got == "" {
		return false
	}
	a := []byte(ck.Value)
	b := []byte(got)
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare(a, b) == 1
}

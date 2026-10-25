package handler

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

const csrfCookieName = "jp_csrf"

func randomHexToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func ensureCSRFToken(c echo.Context) string {
	if c == nil || c.Request() == nil {
		return ""
	}
	ck, err := c.Cookie(csrfCookieName)
	if err == nil && ck != nil && len(strings.TrimSpace(ck.Value)) >= 16 {
		return ck.Value
	}
	tok := randomHexToken()
	if !c.Response().Committed {
		c.SetCookie(sessionCookie(csrfCookieName, tok, 0))
	}
	return tok
}

func requireCSRF(c echo.Context) bool {
	if c == nil || c.Request() == nil {
		return false
	}
	ck, err := c.Cookie(csrfCookieName)
	got := strings.TrimSpace(c.FormValue("csrf"))
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

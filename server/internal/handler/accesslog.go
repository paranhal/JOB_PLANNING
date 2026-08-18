package handler

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"

	"customer-support/internal/auditlog"
	"customer-support/internal/model"
)

// accessLog 접속기록 한 줄을 남긴다. 실패해도 업무를 막지 않는다(§25.3.1).
func accessLog(c echo.Context, rec auditlog.Record) {
	if c == nil {
		auditlog.Append(rec)
		return
	}
	fillActor(c, &rec)
	fillRequest(c, &rec)
	auditlog.Append(rec)
}

func fillActor(c echo.Context, rec *auditlog.Record) {
	if rec.UserID == "" {
		rec.UserID = ctxString(c, "user_id")
	}
	if rec.Username == "" {
		rec.Username = ctxString(c, "username")
	}
	if rec.FullName == "" {
		rec.FullName = ctxString(c, "user_name")
	}
	if rec.Role == "" {
		rec.Role = ctxString(c, "role")
	}
}

func fillRequest(c echo.Context, rec *auditlog.Record) {
	req := c.Request()
	if rec.ClientIP == "" {
		rec.ClientIP = clientIP(c)
	}
	if rec.ForwardedFor == "" {
		rec.ForwardedFor = req.Header.Get("X-Forwarded-For")
	}
	if rec.UserAgent == "" {
		rec.UserAgent = req.UserAgent()
	}
	if rec.SessionID == "" {
		if ck, err := c.Cookie("token"); err == nil {
			rec.SessionID = auditlog.SessionFingerprint(ck.Value)
		}
	}
}

func clientIP(c echo.Context) string {
	host, _, err := net.SplitHostPort(c.Request().RemoteAddr)
	if err != nil {
		return c.Request().RemoteAddr
	}
	return host
}

func toJSON(v any) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func userPublic(u *model.User) map[string]any {
	if u == nil {
		return nil
	}
	return map[string]any{
		"user_id":     u.UserID,
		"username":    u.Username,
		"full_name":   u.FullName,
		"role":        u.Role,
		"permissions": u.Permissions,
		"is_active":   u.IsActive,
	}
}

func (h *AuthHandler) fillActorFromTokenCookie(c echo.Context, rec *auditlog.Record) {
	ck, err := c.Cookie("token")
	if err != nil || ck == nil || ck.Value == "" {
		return
	}
	if rec.SessionID == "" {
		rec.SessionID = auditlog.SessionFingerprint(ck.Value)
	}
	token, err := jwt.Parse(ck.Value, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return h.jwtSecret, nil
	})
	if err != nil || token == nil || !token.Valid {
		return
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return
	}
	if rec.UserID == "" {
		rec.UserID = claimString(claims, "user_id")
	}
	if rec.Username == "" {
		rec.Username = claimString(claims, "username")
	}
	if rec.FullName == "" {
		rec.FullName = claimString(claims, "name")
	}
	if rec.Role == "" {
		rec.Role = claimString(claims, "role")
	}
}

func accessReason(c echo.Context, fallback string) string {
	if c == nil {
		return fallback
	}
	if s := strings.TrimSpace(c.FormValue("reason")); s != "" {
		return s
	}
	return fallback
}

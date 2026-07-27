package handler

import (
	"net/http"
	"strings"
	"time"

	"customer-support/internal/model"

	"github.com/labstack/echo/v4"
)

// Role helpers
func currentRole(c echo.Context) string {
	return ctxString(c, "role")
}

func isAdminRole(c echo.Context) bool {
	return currentRole(c) == "admin"
}

func isTechRole(c echo.Context) bool {
	return currentRole(c) == "tech"
}

func isReceiptRole(c echo.Context) bool {
	return currentRole(c) == "receipt"
}

func isSuspendedRole(c echo.Context) bool {
	r := currentRole(c)
	return r == "sales" || r == "viewer"
}

func canEditVisitDate(c echo.Context, as *model.ASReceipt) bool {
	if isAdminRole(c) {
		return true
	}
	if as == nil {
		return false
	}
	uid := currentUserID(c)
	if as.AssignedUserID != "" && as.AssignedUserID == uid {
		return true
	}
	name := ctxString(c, "user_name")
	username := ctxString(c, "username")
	if as.AssignedTo != "" {
		if as.AssignedTo == name || as.AssignedTo == username {
			return true
		}
	}
	return false
}

func normalizeVisitDate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.Format("2006-01-02")
	}
	return ""
}

func canReceiveAS(c echo.Context) bool {
	r := currentRole(c)
	return r == "admin" || r == "receipt" || r == "tech"
}

func canProcessAS(c echo.Context) bool {
	r := currentRole(c)
	return r == "admin" || r == "tech"
}

func canWriteMaster(c echo.Context) bool {
	return currentRole(c) == "admin"
}

func canManageCodes(c echo.Context) bool {
	return currentRole(c) == "admin"
}

func canViewAnalysis(c echo.Context) bool {
	return currentRole(c) == "admin"
}

func canViewMaintenance(c echo.Context) bool {
	r := currentRole(c)
	return r == "admin" || r == "tech"
}

func assigneeKeys(c echo.Context) []string {
	return []string{ctxString(c, "user_name"), ctxString(c, "username")}
}

func currentUserID(c echo.Context) string {
	return ctxString(c, "user_id")
}

// RequireActiveRole sales/viewer 업무 접근 차단 (account/logout 제외는 라우트에서 분리)
func (h *AuthHandler) RequireActiveRole(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if isSuspendedRole(c) {
			path := c.Request().URL.Path
			if path == "/" || path == "/account" || strings.HasPrefix(path, "/account/") {
				return next(c)
			}
			return c.Redirect(http.StatusSeeOther, "/")
		}
		return next(c)
	}
}

func (h *AuthHandler) RequireAdminMW(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if !isAdminRole(c) {
			return h.forbidden(c)
		}
		return next(c)
	}
}

func (h *AuthHandler) RequireReceiveAS(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if !canReceiveAS(c) {
			return h.forbidden(c)
		}
		return next(c)
	}
}

func (h *AuthHandler) RequireProcessAS(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if !canProcessAS(c) {
			return h.forbidden(c)
		}
		return next(c)
	}
}

func (h *AuthHandler) RequireMasterWrite(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if !canWriteMaster(c) {
			return h.forbidden(c)
		}
		return next(c)
	}
}

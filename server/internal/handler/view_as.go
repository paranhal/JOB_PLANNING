package handler

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/audit"
)

const viewAsCookie = "view_as_user_id"

func canUseViewAs(c echo.Context) bool {
	return isAdminRole(c) || isObserverRole(c)
}

func identityUserID(c echo.Context) string {
	if v := strings.TrimSpace(ctxString(c, "view_as_user_id")); v != "" {
		return v
	}
	return ctxString(c, "user_id")
}

func identityKeys(c echo.Context) []string {
	if strings.TrimSpace(ctxString(c, "view_as_user_id")) != "" {
		return []string{ctxString(c, "view_as_name"), ctxString(c, "view_as_username")}
	}
	return assigneeKeys(c)
}

func (h *AuthHandler) InjectViewAs(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if !canUseViewAs(c) {
			return next(c)
		}
		if list, err := h.userRepo.ListAssignable(); err == nil {
			c.Set("view_as_users", list)
		}
		var uid string
		if ck, err := c.Cookie(viewAsCookie); err == nil && ck != nil {
			uid = strings.TrimSpace(ck.Value)
		}
		if uid == "" || uid == ctxString(c, "user_id") {
			return next(c)
		}
		u, err := h.userRepo.GetByID(uid)
		if err != nil || u == nil || !u.IsActive {
			c.SetCookie(&http.Cookie{Name: viewAsCookie, Value: "", Path: "/", MaxAge: -1})
			return next(c)
		}
		c.Set("view_as_user_id", u.UserID)
		c.Set("view_as_name", u.FullName)
		c.Set("view_as_username", u.Username)
		return next(c)
	}
}

func (h *AuthHandler) GuardViewAsWrite(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if strings.TrimSpace(ctxString(c, "view_as_user_id")) == "" {
			return next(c)
		}
		m := c.Request().Method
		if m == http.MethodGet || m == http.MethodHead {
			return next(c)
		}
		path := c.Request().URL.Path
		if strings.HasPrefix(path, "/view-as") {
			return next(c)
		}
		return echo.ErrForbidden
	}
}

func (h *AuthHandler) SetViewAs(c echo.Context) error {
	if !canUseViewAs(c) {
		return echo.ErrForbidden
	}
	id := strings.TrimSpace(c.FormValue("user_id"))
	if id == "" {
		return h.ClearViewAs(c)
	}
	u, err := h.userRepo.GetByID(id)
	if err != nil || u == nil {
		return echo.ErrNotFound
	}
	c.SetCookie(&http.Cookie{Name: viewAsCookie, Value: u.UserID, Path: "/", HttpOnly: true})
	audit.LogWithReason(audit.ActionUpdate, "users", "user_id", u.UserID, u.FullName, "", "", "시점 보기 시작")
	ret := strings.TrimSpace(c.FormValue("return"))
	if !strings.HasPrefix(ret, "/") {
		ret = "/"
	}
	return c.Redirect(http.StatusSeeOther, ret)
}

func (h *AuthHandler) ClearViewAs(c echo.Context) error {
	if !canUseViewAs(c) {
		return echo.ErrForbidden
	}
	prev := ""
	if ck, err := c.Cookie(viewAsCookie); err == nil && ck != nil {
		prev = strings.TrimSpace(ck.Value)
	}
	c.SetCookie(&http.Cookie{Name: viewAsCookie, Value: "", Path: "/", MaxAge: -1})
	if prev != "" {
		audit.LogWithReason(audit.ActionUpdate, "users", "user_id", prev, "시점 보기", "", "", "시점 보기 종료")
	}
	ret := strings.TrimSpace(c.FormValue("return"))
	if !strings.HasPrefix(ret, "/") {
		ret = "/"
	}
	return c.Redirect(http.StatusSeeOther, ret)
}

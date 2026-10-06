package handler

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/audit"
	"customer-support/internal/model"
)

const viewAsCookie = "view_as_user_id"

func canUseViewAs(c echo.Context) bool {
	return model.IsAdminGrade(loginRole(c))
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

func applySimulation(c echo.Context, role, storedPerms, orgID, baseRole string) {
	role = model.NormalizeRole(role)
	baseRole = model.NormalizeRole(baseRole)
	c.Set("sim_role", role)
	c.Set("sim_base_role", baseRole)
	c.Set("sim_permissions", model.EffectivePermissions(role, storedPerms))
	c.Set("sim_org_id", strings.TrimSpace(orgID))
}

func applyUserSimulation(c echo.Context, u *model.User) {
	if u == nil {
		return
	}
	applySimulation(c, u.Role, u.Permissions, u.OrgID, u.BaseRole)
	c.Set("view_as_user_id", u.UserID)
	c.Set("view_as_name", u.FullName)
	c.Set("view_as_username", u.Username)
}

func viewAsCandidates(users []model.User, selfID string) []model.User {
	out := make([]model.User, 0, len(users))
	selfID = strings.TrimSpace(selfID)
	for _, u := range users {
		if !u.IsActive {
			continue
		}
		if strings.TrimSpace(u.UserID) == selfID {
			continue
		}
		if u.IsReadOnly {
			continue
		}
		out = append(out, u)
	}
	return out
}

func (h *AuthHandler) InjectViewAs(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if !canUseViewAs(c) {
			return next(c)
		}
		if h.userRepo != nil {
			if list, err := h.userRepo.ListAll(); err == nil {
				c.Set("view_as_users", viewAsCandidates(list, ctxString(c, "user_id")))
			}
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
			c.SetCookie(sessionCookie(viewAsCookie, "", -1))
			return next(c)
		}
		applyUserSimulation(c, u)
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
	c.SetCookie(sessionCookie(viewAsCookie, u.UserID, 86400))
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
	c.SetCookie(sessionCookie(viewAsCookie, "", -1))
	if prev != "" {
		audit.LogWithReason(audit.ActionUpdate, "users", "user_id", prev, "시점 보기", "", "", "시점 보기 종료")
	}
	ret := strings.TrimSpace(c.FormValue("return"))
	if !strings.HasPrefix(ret, "/") {
		ret = "/"
	}
	return c.Redirect(http.StatusSeeOther, ret)
}

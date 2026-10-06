package handler

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/auditlog"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func (h *AuthHandler) PermissionsPage(c echo.Context) error {
	if currentRole(c) != model.RoleVisionAdmin {
		return echo.ErrForbidden
	}
	tab := strings.TrimSpace(c.QueryParam("tab"))
	if tab != "grade" {
		tab = "work"
	}
	return c.Render(http.StatusOK, "admin/permissions.html", map[string]interface{}{
		"Title":    "역할 및 권한",
		"Active":   NavPermissions,
		"Tab":      tab,
		"Groups":   model.PermissionGroups(),
		"FlashOK":  strings.TrimSpace(c.QueryParam("ok")),
		"FlashErr": strings.TrimSpace(c.QueryParam("err")),
	})
}

func (h *AuthHandler) PermissionsSave(c echo.Context) error {
	if currentRole(c) != model.RoleVisionAdmin || isReadOnly(c) {
		return echo.ErrForbidden
	}
	_ = c.Request().ParseForm()
	roles := c.Request().PostForm["role"]
	keys := c.Request().PostForm["perm_key"]
	accs := c.Request().PostForm["access"]
	if len(roles) != len(keys) || len(keys) != len(accs) {
		return c.Redirect(http.StatusSeeOther, "/admin/permissions?err="+url.QueryEscape("저장 형식이 어긋났습니다."))
	}
	repo := repository.NewRolePermRepo(h.db)
	by := ctxString(c, "user_name")
	for i := range roles {
		role := model.NormalizeRole(roles[i])
		key := model.CanonicalPerm(keys[i])
		if role == model.RoleVisionAdmin || role == "" || key == "" {
			continue
		}
		ai, _ := strconv.Atoi(accs[i])
		if ai < 0 || ai > 3 {
			continue
		}
		acc := model.Access(ai)
		def := model.BuiltinAccess(role, key)
		var err error
		if acc == def {
			err = repo.Delete(role, key)
		} else {
			err = repo.Save(role, key, acc, by)
		}
		if err != nil {
			return c.Redirect(http.StatusSeeOther, "/admin/permissions?err="+url.QueryEscape("저장하지 못했습니다."))
		}
		accessLog(c, auditlog.Record{
			Action: auditlog.ActionUpdate, Result: auditlog.ResultOK,
			TargetTable: "role_permissions", SubjectName: role,
			Detail: "권한 " + key + " " + model.AccessLabel(def) + " → " + model.AccessLabel(acc),
		})
	}
	if m, err := repo.Load(); err == nil {
		model.SetAccessOverrides(m)
	}
	tab := strings.TrimSpace(c.FormValue("tab"))
	if tab != "grade" {
		tab = "work"
	}
	return c.Redirect(http.StatusSeeOther, "/admin/permissions?tab="+url.QueryEscape(tab)+"&ok="+url.QueryEscape("권한 행렬을 저장했습니다."))
}

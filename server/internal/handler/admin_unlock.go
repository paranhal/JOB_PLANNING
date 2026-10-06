package handler

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/repository"
)

func adminSectionExempt(path string) bool {
	p := strings.TrimSpace(path)
	if p == "/admin/unlock" || strings.HasPrefix(p, "/admin/unlock?") {
		return true
	}
	if p == "/admin/orgs/switch" {
		return true
	}
	return false
}

func isAdminSectionPath(path string) bool {
	p := strings.TrimSpace(path)
	if adminSectionExempt(p) {
		return false
	}
	if strings.HasPrefix(p, "/codes") || strings.HasPrefix(p, "/users") {
		return true
	}
	if strings.HasPrefix(p, "/admin/") || p == "/admin" {
		return true
	}
	return false
}

func skipsAdminSectionUnlock(c echo.Context) bool {
	if isReadOnly(c) {
		return true
	}
	v, _ := c.Get("is_test").(bool)
	return v
}

func (h *AuthHandler) adminSectionOpen(c echo.Context) bool {
	if h == nil || h.adminUnlock == nil {
		return false
	}
	ok, _, err := h.adminUnlock.HasActive(currentUserID(c))
	return err == nil && ok
}

func (h *AuthHandler) RequireAdminSection(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if !isAdminSectionPath(c.Request().URL.Path) {
			return next(c)
		}
		if skipsAdminSectionUnlock(c) {
			return next(c)
		}
		if h.adminSectionOpen(c) {
			return next(c)
		}
		ret := c.Request().URL.RequestURI()
		if c.Request().Method != http.MethodGet && c.Request().Method != http.MethodHead {
			ret = c.Request().URL.Path
		}
		return c.Redirect(http.StatusSeeOther, "/admin/unlock?return="+url.QueryEscape(ret))
	}
}

func (h *AuthHandler) AdminUnlockForm(c echo.Context) error {
	if skipsAdminSectionUnlock(c) || h.adminSectionOpen(c) {
		back := safeAdminReturn(c.QueryParam("return"))
		return c.Redirect(http.StatusSeeOther, back)
	}
	errMsg := c.QueryParam("err")
	return c.Render(http.StatusOK, "auth/admin_unlock.html", map[string]interface{}{
		"Title": "관리 섹션 잠금", "Active": NavNone, "ReturnPath": safeAdminReturn(c.QueryParam("return")),
		"Error": errMsg,
	})
}

func (h *AuthHandler) AdminUnlock(c echo.Context) error {
	back := safeAdminReturn(c.FormValue("return"))
	if skipsAdminSectionUnlock(c) {
		return c.Redirect(http.StatusSeeOther, back)
	}
	pw := strings.TrimSpace(c.FormValue("unlock_password"))
	ok := h.verifyVisionAdminPassword(c, pw)
	if h.adminUnlock != nil {
		_ = h.adminUnlock.LogAttempt(currentUserID(c), ctxString(c, "username"), c.RealIP(), ok)
	}
	if !ok {
		return c.Redirect(http.StatusSeeOther, "/admin/unlock?return="+url.QueryEscape(back)+"&err="+url.QueryEscape("비젼관리자 비밀번호가 올바르지 않습니다"))
	}
	if h.adminUnlock != nil {
		if _, err := h.adminUnlock.Grant(currentUserID(c)); err != nil {
			return err
		}
	}
	return c.Redirect(http.StatusSeeOther, back)
}

func (h *AuthHandler) verifyVisionAdminPassword(c echo.Context, pw string) bool {
	pw = strings.TrimSpace(pw)
	if pw == "" {
		return false
	}
	if h.settingsRepo != nil {
		hash, _ := h.settingsRepo.Get(repository.SettingVisionAdminPassword)
		if strings.TrimSpace(hash) != "" {
			return verifyAndUpgradeSetting(h.settingsRepo, repository.SettingVisionAdminPassword, pw)
		}
	}
	return false
}

func safeAdminReturn(raw string) string {
	back := strings.TrimSpace(raw)
	if !strings.HasPrefix(back, "/") || strings.HasPrefix(back, "//") {
		return "/users"
	}
	if strings.HasPrefix(back, "/admin/unlock") {
		return "/users"
	}
	return back
}

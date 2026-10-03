package handler

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/passwd"
	"customer-support/internal/repository"
)

func (h *AuthHandler) visionPasswordMustChange(user *model.User) bool {
	if h == nil || h.settingsRepo == nil || user == nil {
		return false
	}
	return model.NormalizeRole(user.Role) == model.RoleVisionAdmin && h.settingsRepo.VisionAdminMustChange()
}

func (h *AuthHandler) AccountVisionPasswordForm(c echo.Context) error {
	if loginRole(c) != model.RoleVisionAdmin {
		return h.forbidden(c)
	}
	must := h.settingsRepo != nil && h.settingsRepo.VisionAdminMustChange()
	return c.Render(http.StatusOK, "auth/vision_password.html", map[string]interface{}{
		"Title": "비젼관리자 비밀번호", "Active": NavAccount,
		"MustChange": must, "PasswordMinLen": passwd.MinLen,
	})
}

func (h *AuthHandler) AccountVisionPassword(c echo.Context) error {
	if loginRole(c) != model.RoleVisionAdmin {
		return h.forbidden(c)
	}
	renderErr := func(msg string) error {
		must := h.settingsRepo != nil && h.settingsRepo.VisionAdminMustChange()
		return c.Render(http.StatusOK, "auth/vision_password.html", map[string]interface{}{
			"Title": "비젼관리자 비밀번호", "Active": NavAccount,
			"MustChange": must, "Error": msg, "PasswordMinLen": passwd.MinLen,
		})
	}
	if h.settingsRepo == nil {
		return renderErr("설정을 저장할 수 없습니다.")
	}
	cur := strings.TrimSpace(c.FormValue("current_password"))
	nw := strings.TrimSpace(c.FormValue("password"))
	confirm := strings.TrimSpace(c.FormValue("password_confirm"))
	if !h.verifyVisionAdminPassword(c, cur) {
		return renderErr("현재 비젼관리자 비밀번호가 올바르지 않습니다.")
	}
	if passwordTooShort(nw) {
		return renderErr(passwordMinLenMsg())
	}
	if nw != confirm {
		return renderErr("새 비밀번호와 확인 입력이 일치하지 않습니다.")
	}
	nh := HashPassword(nw)
	if nh == "" {
		return renderErr("비밀번호를 저장하지 못했습니다.")
	}
	if err := h.settingsRepo.Set(repository.SettingVisionAdminPassword, nh); err != nil {
		return renderErr("비밀번호를 저장하지 못했습니다.")
	}
	_ = h.settingsRepo.ClearVisionAdminMustChange()
	return c.Redirect(http.StatusSeeOther, "/account?ok=vision")
}

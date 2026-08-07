package handler

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// canModifyAS 완료·종료 건은 관리자가 수정 비밀번호로 잠금 해제한 경우에만 수정 가능.
// 진행 중 건은 기존 권한(접수/조치)을 따른다.
func (h *ASHandler) canModifyAS(c echo.Context, as *model.ASReceipt) bool {
	if as == nil {
		return false
	}
	if !isASClosedStatus(as.Status) {
		return true
	}
	if !isAdminRole(c) {
		return false
	}
	ok, _, _ := h.unlockActive(c, as.ASID)
	return ok
}

func (h *ASHandler) unlockActive(c echo.Context, asID string) (bool, time.Time, error) {
	if h.unlockRepo == nil {
		return false, time.Time{}, nil
	}
	return h.unlockRepo.HasActive(asID, currentUserID(c))
}

func (h *ASHandler) redirectLocked(c echo.Context, id, page string) error {
	msg := url.QueryEscape("완료·종료 건은 읽기 전용입니다. 관리자는 수정 잠금 해제 후 이용하세요")
	if page == "action" {
		return c.Redirect(http.StatusSeeOther, "/as/"+id+"/action?err="+msg)
	}
	return c.Redirect(http.StatusSeeOther, "/as/"+id+"?err="+msg)
}

// UnlockEdit 관리자: 완료 건 수정 비밀번호 확인 후 잠금 해제(30분)
func (h *ASHandler) UnlockEdit(c echo.Context) error {
	if !isAdminRole(c) {
		return echo.ErrForbidden
	}
	id := c.Param("id")
	as, err := h.repo.GetByID(id)
	if err != nil || as == nil {
		return echo.ErrNotFound
	}
	if !isASClosedStatus(as.Status) {
		return c.Redirect(http.StatusSeeOther, "/as/"+id)
	}
	pw := strings.TrimSpace(c.FormValue("unlock_password"))
	redirect := strings.TrimSpace(c.FormValue("redirect"))
	if redirect == "" {
		redirect = "/as/" + id
	}
	hash, _ := h.settingsRepo.Get(repository.SettingASCompletedEditPassword)
	ok := pw != "" && hash != "" && HashPassword(pw) == hash
	_ = h.unlockRepo.LogAttempt(id, currentUserID(c), ctxString(c, "username"), c.RealIP(), ok)
	if !ok {
		sep := "?"
		if strings.Contains(redirect, "?") {
			sep = "&"
		}
		return c.Redirect(http.StatusSeeOther, redirect+sep+"err="+url.QueryEscape("수정 비밀번호가 올바르지 않습니다"))
	}
	if _, err := h.unlockRepo.Grant(id, currentUserID(c)); err != nil {
		return err
	}
	sep := "?"
	if strings.Contains(redirect, "?") {
		sep = "&"
	}
	return c.Redirect(http.StatusSeeOther, redirect+sep+"ok="+url.QueryEscape("수정 잠금이 해제되었습니다(30분)"))
}

// UpdateCompletedEditPassword 관리자: 완료 건 수정 비밀번호 변경
func (h *ASHandler) UpdateCompletedEditPassword(c echo.Context) error {
	if !isAdminRole(c) {
		return echo.ErrForbidden
	}
	cur := strings.TrimSpace(c.FormValue("current_password"))
	nw := strings.TrimSpace(c.FormValue("new_password"))
	confirm := strings.TrimSpace(c.FormValue("confirm_password"))
	if nw == "" || len(nw) < 4 {
		return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape("새 비밀번호는 4자 이상이어야 합니다"))
	}
	if nw != confirm {
		return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape("새 비밀번호 확인이 일치하지 않습니다"))
	}
	hash, _ := h.settingsRepo.Get(repository.SettingASCompletedEditPassword)
	if hash == "" || HashPassword(cur) != hash {
		return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape("현재 수정 비밀번호가 올바르지 않습니다"))
	}
	if err := h.settingsRepo.Set(repository.SettingASCompletedEditPassword, HashPassword(nw)); err != nil {
		return err
	}
	return c.Redirect(http.StatusSeeOther, "/users?ok="+url.QueryEscape("완료 건 수정 비밀번호를 변경했습니다"))
}

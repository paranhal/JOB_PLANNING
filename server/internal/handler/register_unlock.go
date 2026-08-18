package handler

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/repository"
)

// registerPastDayLockEnabled 지난 날짜 일일 업무 수정 잠금.
// 테스트 기간: false (별도 지시 시 true로 복구).
const registerPastDayLockEnabled = false

// registerUnlockKey 일일 업무 등록 과거일 잠금 해제 키 (as_edit_unlocks.as_id 재사용).
func registerUnlockKey(date string) string {
	return "wbreg:" + strings.TrimSpace(date)
}

func (h *WorkboardHandler) registerDayUnlocked(c echo.Context, date string) (bool, time.Time) {
	if !registerPastDayLockEnabled {
		return true, time.Time{}
	}
	if h == nil || h.unlockRepo == nil || date == "" {
		return false, time.Time{}
	}
	ok, exp, _ := h.unlockRepo.HasActive(registerUnlockKey(date), currentUserID(c))
	return ok, exp
}

// canEditRegisterDate 오늘 이후는 업무등록 권한만, 지난날은 관리자+수정비번 해제 후에만 가능.
// registerPastDayLockEnabled=false 이면 지난날도 업무등록 권한만으로 수정 가능.
func (h *WorkboardHandler) canEditRegisterDate(c echo.Context, date string) bool {
	date = strings.TrimSpace(date)
	if date == "" || !canWriteWorkboard(c) {
		return false
	}
	if !registerPastDayLockEnabled {
		return true
	}
	today := time.Now().Format(dateLayout)
	if date >= today {
		return true
	}
	if !isAdminRole(c) {
		return false
	}
	ok, _ := h.registerDayUnlocked(c, date)
	return ok
}

// UnlockPastRegister 관리자: 지난 날짜 일일 업무 등록 수정 잠금 해제(30분). AS 완료 건 수정 비밀번호와 동일.
func (h *WorkboardHandler) UnlockPastRegister(c echo.Context) error {
	if !isAdminRole(c) {
		return echo.ErrForbidden
	}
	date := strings.TrimSpace(c.FormValue("date"))
	if date == "" {
		date = strings.TrimSpace(c.QueryParam("date"))
	}
	if _, err := time.ParseInLocation(dateLayout, date, time.Local); err != nil {
		return c.Redirect(http.StatusSeeOther, "/workboard/register?err="+url.QueryEscape("날짜가 올바르지 않습니다"))
	}
	today := time.Now().Format(dateLayout)
	if date >= today {
		return c.Redirect(http.StatusSeeOther, "/workboard/register?view=day&date="+url.QueryEscape(date))
	}
	redirect := strings.TrimSpace(c.FormValue("redirect"))
	if redirect == "" {
		redirect = "/workboard/register?view=day&date=" + url.QueryEscape(date)
	}
	pw := strings.TrimSpace(c.FormValue("unlock_password"))
	ok := verifyAndUpgradeSetting(h.settingsRepo, repository.SettingASCompletedEditPassword, pw)
	key := registerUnlockKey(date)
	if h.unlockRepo != nil {
		_ = h.unlockRepo.LogAttempt(key, currentUserID(c), ctxString(c, "username"), c.RealIP(), ok)
	}
	if !ok {
		sep := "?"
		if strings.Contains(redirect, "?") {
			sep = "&"
		}
		return c.Redirect(http.StatusSeeOther, redirect+sep+"err="+url.QueryEscape("수정 비밀번호가 올바르지 않습니다"))
	}
	if h.unlockRepo != nil {
		if _, err := h.unlockRepo.Grant(key, currentUserID(c)); err != nil {
			return err
		}
	}
	sep := "?"
	if strings.Contains(redirect, "?") {
		sep = "&"
	}
	return c.Redirect(http.StatusSeeOther, redirect+sep+"ok="+url.QueryEscape("지난 날짜 수정 잠금이 해제되었습니다(30분)"))
}

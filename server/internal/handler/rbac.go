package handler

import (
	"net/http"
	"strings"

	"customer-support/internal/model"

	"github.com/labstack/echo/v4"
)

// Role helpers
func currentRole(c echo.Context) string {
	return model.NormalizeRole(ctxString(c, "role"))
}

func currentPerms(c echo.Context) []string {
	if v := c.Get("permissions"); v != nil {
		if p, ok := v.([]string); ok {
			return p
		}
	}
	return model.EffectivePermissions(currentRole(c), "")
}

func hasPerm(c echo.Context, key string) bool {
	if currentRole(c) == model.RoleAdmin {
		return true
	}
	return model.HasPermission(currentPerms(c), key)
}

func isAdminRole(c echo.Context) bool {
	return currentRole(c) == model.RoleAdmin
}

func isTechRole(c echo.Context) bool {
	return currentRole(c) == model.RoleTech
}

func isReceiptRole(c echo.Context) bool {
	// 행정 등급이 기존 접수담당 역할 대체
	return currentRole(c) == model.RoleOffice
}

func isOfficeRole(c echo.Context) bool {
	return currentRole(c) == model.RoleOffice
}

func isObserverRole(c echo.Context) bool {
	return currentRole(c) == model.RoleObserver
}

// isSuspendedRole 옵저버는 쓰기 메뉴·작업 제한(조회·계정만)
func isSuspendedRole(c echo.Context) bool {
	return isObserverRole(c)
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
	return model.NormalizeAppDate(s)
}

func canReceiveAS(c echo.Context) bool {
	if isObserverRole(c) {
		return false
	}
	return hasPerm(c, model.PermASReceive)
}

func canProcessAS(c echo.Context) bool {
	if isObserverRole(c) {
		return false
	}
	return hasPerm(c, model.PermASProcess)
}

// isASClosedStatus 완료·종료 — 기본 읽기 전용
func isASClosedStatus(status string) bool {
	return model.CanReopenAS(status)
}

func canWriteMaster(c echo.Context) bool {
	if isObserverRole(c) {
		return false
	}
	return hasPerm(c, model.PermMasterWrite)
}

func canManageCodes(c echo.Context) bool {
	return hasPerm(c, model.PermCodesUsers)
}

func canViewAnalysis(c echo.Context) bool {
	return hasPerm(c, model.PermAnalysis)
}

func canViewMaintenance(c echo.Context) bool {
	return hasPerm(c, model.PermMaintenance) || hasPerm(c, model.PermMaintenanceEdit)
}

// canEditMaintenanceSchedule 정기점검 방문 일정 수정
func canEditMaintenanceSchedule(c echo.Context) bool {
	if isObserverRole(c) {
		return false
	}
	return hasPerm(c, model.PermMaintenanceEdit)
}

// mntScopeAll 정기점검 일정 조회 범위 — 관리자는 항상 전체, 기술담당은 all=1 일 때만 전체
func mntScopeAll(c echo.Context) bool {
	if isAdminRole(c) {
		return true
	}
	if !isTechRole(c) {
		return true
	}
	v := c.QueryParam("all")
	if v == "" {
		v = c.FormValue("all")
	}
	return v == "1"
}

func (h *AuthHandler) RequireMaintenanceEdit(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if !canEditMaintenanceSchedule(c) {
			return h.forbidden(c)
		}
		return next(c)
	}
}

func assigneeKeys(c echo.Context) []string {
	return []string{ctxString(c, "user_name"), ctxString(c, "username")}
}

func currentUserID(c echo.Context) string {
	return ctxString(c, "user_id")
}

// RequireActiveRole 옵저버는 전체 화면 조회(GET) 가능, 쓰기는 계정만 허용
func (h *AuthHandler) RequireActiveRole(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if !isSuspendedRole(c) {
			return next(c)
		}
		path := c.Request().URL.Path
		method := c.Request().Method
		if method == http.MethodGet || method == http.MethodHead {
			return next(c)
		}
		// 계정 프로필·비밀번호 변경만 POST 허용
		if strings.HasPrefix(path, "/account/") {
			return next(c)
		}
		return c.Redirect(http.StatusSeeOther, path)
	}
}

func (h *AuthHandler) RequireAdminMW(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if isAdminRole(c) || hasPerm(c, model.PermCodesUsers) {
			return next(c)
		}
		// 옵저버: 관리 메뉴도 조회(GET)만
		if isObserverRole(c) && (c.Request().Method == http.MethodGet || c.Request().Method == http.MethodHead) {
			return next(c)
		}
		return h.forbidden(c)
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

// parsePermForm 체크박스 name=perm 수집
func parsePermForm(c echo.Context) string {
	form, err := c.FormParams()
	if err != nil || form == nil {
		return ""
	}
	return model.FormatPermissions(form["perm"])
}

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

func isSalesRole(c echo.Context) bool {
	return currentRole(c) == model.RoleSales
}

func canViewSales(c echo.Context) bool {
	r := currentRole(c)
	return r == model.RoleAdmin || r == model.RoleSales || r == model.RoleOffice ||
		r == model.RoleTech || r == model.RoleObserver
}

// canEditSalesActivity §39.5 · §37.3 — 본인 등록은 본인, 남의 것은 관리자·행정만.
func canEditSalesActivity(c echo.Context, createdBy string) bool {
	if !canWriteSales(c) || isObserverRole(c) {
		return false
	}
	if isAdminRole(c) || isOfficeRole(c) {
		return true
	}
	return assigneeIsMine(c, createdBy, "")
}

func canWriteSales(c echo.Context) bool {
	if isObserverRole(c) {
		return false
	}
	r := currentRole(c)
	return r == model.RoleAdmin || r == model.RoleSales || r == model.RoleOffice
}

func canSeeMargin(c echo.Context) bool {
	return model.CanSeeMargin(currentRole(c))
}

func canDeleteSales(c echo.Context) bool {
	return isAdminRole(c)
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

// assigneeIsMine 현재 사용자 이름·아이디가 담당자와 같으면 true. assigneeKeys 를 쓴다.
func assigneeIsMine(c echo.Context, assignee, assignedUserID string) bool {
	uid := strings.TrimSpace(currentUserID(c))
	if uid != "" && strings.TrimSpace(assignedUserID) == uid {
		return true
	}
	name := strings.TrimSpace(assignee)
	if name == "" {
		return false
	}
	for _, k := range assigneeKeys(c) {
		if strings.TrimSpace(k) != "" && strings.TrimSpace(k) == name {
			return true
		}
	}
	return false
}

// canEditTask §37.3 — 보는 것과 고치는 것을 나눈다.
// canWriteWorkboard 로 쓰기 권한을 열고, 기술·영업은 assigneeKeys 로 내 배정만 허용한다.
func canEditTask(c echo.Context, assignee, assignedUserID string) bool {
	if isObserverRole(c) {
		return false
	}
	if isAdminRole(c) || isOfficeRole(c) {
		return true
	}
	if isTechRole(c) {
		if !canWriteWorkboard(c) {
			return false
		}
		return assigneeIsMine(c, assignee, assignedUserID)
	}
	if isSalesRole(c) {
		return assigneeIsMine(c, assignee, assignedUserID)
	}
	return false
}

func denyUnlessCanEditTask(c echo.Context, t *model.WorkTask) error {
	if t == nil {
		return echo.ErrNotFound
	}
	if !canEditTask(c, t.Assignee, "") {
		return echo.ErrForbidden
	}
	return nil
}

func currentUserDisplayName(c echo.Context) string {
	n := strings.TrimSpace(ctxString(c, "user_name"))
	if n != "" {
		return n
	}
	return strings.TrimSpace(ctxString(c, "username"))
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

// RequireAdminOnly 관리자 등급만. 시스템 정보처럼 커밋 해시를 보여 주는 화면에 쓴다. §40.5.2
func (h *AuthHandler) RequireAdminOnly(next echo.HandlerFunc) echo.HandlerFunc {
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

// parsePermForm 체크박스 name=perm 수집
func parsePermForm(c echo.Context) string {
	form, err := c.FormParams()
	if err != nil || form == nil {
		return ""
	}
	return model.FormatPermissions(form["perm"])
}

package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/audit"
	"customer-support/internal/backup"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type DataResetHandler struct {
	cfg        backup.Config
	auth       *AuthHandler
	orgs       *repository.OrgRepo
	snapshotFn func() (string, error)
	restoreFn  func(name string) error
}

func canResetData(c echo.Context) bool {
	if isReadOnly(c) {
		return false
	}
	switch loginRole(c) {
	case model.RoleTester, model.RoleOrgAdmin, model.RoleVisionAdmin:
		return true
	default:
		return false
	}
}

func (h *AuthHandler) RequireDataResetMW(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if canResetData(c) {
			return next(c)
		}
		return h.forbidden(c)
	}
}

func (h *DataResetHandler) allowedScope(c echo.Context, scope string) bool {
	scope = repository.NormalizeResetScope(scope)
	role := loginRole(c)
	switch scope {
	case repository.ResetScopeTest:
		return role == model.RoleTester || role == model.RoleOrgAdmin || role == model.RoleVisionAdmin
	case repository.ResetScopeOrg:
		return role == model.RoleOrgAdmin || role == model.RoleVisionAdmin
	case repository.ResetScopeAll:
		return role == model.RoleVisionAdmin
	default:
		return false
	}
}

func appEnvProduction() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("APP_ENV")), "production")
}

func (h *DataResetHandler) expectedConfirmName(c echo.Context) string {
	orgID := currentOrg(c)
	if orgID == repository.OrgAll {
		return "전 조직"
	}
	if h.orgs != nil {
		if org, err := h.orgs.Get(orgID); err == nil && org != nil {
			return org.OrgName
		}
	}
	return model.OrgNameLibrary
}

func (h *DataResetHandler) snapshot() (string, error) {
	if h.snapshotFn != nil {
		return h.snapshotFn()
	}
	return backup.Snapshot(h.cfg, backup.KindManual)
}

func (h *DataResetHandler) restore(name string) error {
	if h.restoreFn != nil {
		return h.restoreFn(name)
	}
	return backup.Restore(h.cfg, name)
}

func (h *DataResetHandler) Page(c echo.Context) error {
	return h.render(c, repository.NormalizeResetScope(c.QueryParam("scope")), c.QueryParam("ok"), c.QueryParam("err"), c.QueryParam("snapshot"))
}

func (h *DataResetHandler) Preview(c echo.Context) error {
	scope := repository.NormalizeResetScope(c.FormValue("scope"))
	return h.render(c, scope, "", "", "")
}

func (h *DataResetHandler) render(c echo.Context, scope, okMsg, errMsg, snapshot string) error {
	if scope == "" {
		scope = repository.ResetScopeTest
	}
	orgID := currentOrg(c)
	if scope == repository.ResetScopeOrg && (orgID == "" || orgID == repository.OrgAll) {
		errMsg = firstNonBlank(errMsg, "조직 전체는 한 조직을 고른 뒤에만 됩니다")
	}
	counts, err := repository.PreviewReset(h.cfg.DB, scope, orgID)
	if err != nil && errMsg == "" {
		errMsg = err.Error()
	}
	role := loginRole(c)
	return c.Render(http.StatusOK, "admin/data_reset.html", map[string]interface{}{
		"Title":           "데이터 초기화",
		"Active":          NavData,
		"Tab":             "reset",
		"Scope":           scope,
		"Counts":          counts,
		"ConfirmName":     h.expectedConfirmName(c),
		"CanOrg":          role == model.RoleOrgAdmin || role == model.RoleVisionAdmin,
		"CanAll":          role == model.RoleVisionAdmin,
		"Production":      appEnvProduction(),
		"NeedProdConfirm": appEnvProduction() && scope != repository.ResetScopeTest,
		"OK":              okMsg,
		"Error":           errMsg,
		"Snapshot":        snapshot,
		"RestoreWarning":  "현재 데이터는 사라집니다",
	})
}

func firstNonBlank(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func (h *DataResetHandler) Execute(c echo.Context) error {
	scope := repository.NormalizeResetScope(c.FormValue("scope"))
	redirect := func(ok, errMsg, snap string) error {
		q := url.Values{}
		q.Set("scope", scope)
		if ok != "" {
			q.Set("ok", ok)
		}
		if errMsg != "" {
			q.Set("err", errMsg)
		}
		if snap != "" {
			q.Set("snapshot", snap)
		}
		return c.Redirect(http.StatusSeeOther, "/admin/data/reset?"+q.Encode())
	}
	if !h.allowedScope(c, scope) {
		return h.auth.forbidden(c)
	}
	pw := c.FormValue("vision_password")
	if h.auth == nil || !h.auth.verifyVisionAdminPassword(c, pw) {
		return redirect("", "비젼관리자 비밀번호가 필요합니다", "")
	}
	typed := strings.TrimSpace(c.FormValue("confirm_name"))
	want := h.expectedConfirmName(c)
	if typed != want {
		return redirect("", "조직명을 화면에 적힌 그대로 입력하세요", "")
	}
	if appEnvProduction() && scope != repository.ResetScopeTest {
		if c.FormValue("confirm_prod") != "1" {
			return redirect("", "운영 환경에서는 한 번 더 확인해야 합니다", "")
		}
	}
	orgID := currentOrg(c)
	name, err := h.snapshot()
	if err != nil {
		return redirect("", fmt.Sprintf("백업 실패: %v", err), "")
	}
	counts, err := repository.ExecuteReset(h.cfg.DB, scope, orgID)
	if err != nil {
		return redirect("", fmt.Sprintf("초기화 실패: %v", err), name)
	}
	audit.Use(h.cfg.DB)
	for _, row := range counts {
		audit.LogWithReason("delete", row.Table, "", "reset", row.Table, "", fmt.Sprintf("%d", row.N), "데이터 초기화 "+scope)
	}
	return redirect("초기화했습니다. 백업 폴더: "+name, "", name)
}

func (h *DataResetHandler) Restore(c echo.Context) error {
	name := strings.TrimSpace(c.FormValue("snapshot"))
	redirect := func(ok, errMsg, snap string) error {
		q := url.Values{}
		q.Set("scope", repository.ResetScopeTest)
		if ok != "" {
			q.Set("ok", ok)
		}
		if errMsg != "" {
			q.Set("err", errMsg)
		}
		if snap != "" {
			q.Set("snapshot", snap)
		}
		return c.Redirect(http.StatusSeeOther, "/admin/data/reset?"+q.Encode())
	}
	if h.auth == nil || !h.auth.verifyVisionAdminPassword(c, c.FormValue("vision_password")) {
		return redirect("", "비젼관리자 비밀번호가 필요합니다", name)
	}
	if c.FormValue("confirm_restore") != "1" {
		return redirect("", "복구하면 현재 데이터는 사라집니다. 확인란을 선택하세요", name)
	}
	if err := h.restore(name); err != nil {
		return redirect("", err.Error(), name)
	}
	return redirect("백업으로 되돌렸습니다", "", "")
}

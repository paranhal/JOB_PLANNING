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

// 95-224 §3: 사이트 등록 권한이 없는 조직원의 「1회 등록 요청」.
// 비밀번호는 AS 수정 잠금과 같은 설정값으로 검증하고, 맞으면 이 사업에 대해서만 30분짜리 권한을 준다.
// 사이트 등록이 1번 성공하면 바로 지운다. 비밀번호는 로그·응답에 남기지 않는다.

// canRequestSiteAddOnce 1회 등록 요청 대상: 업무 권한이 있고 읽기 전용·C레벨이 아닌 사용자.
func (h *ProjectHandler) canRequestSiteAddOnce(c echo.Context) bool {
	if h == nil || h.settings == nil || h.siteUnlock == nil {
		return false
	}
	if isReadOnly(c) || currentRole(c) == model.RoleCLevel {
		return false
	}
	return strings.TrimSpace(currentUserID(c)) != "" && hasPerm(c, model.PermWorkboard)
}

// siteAddGrantActive 이 사업에 대한 1회 권한이 살아 있는지.
func (h *ProjectHandler) siteAddGrantActive(c echo.Context, p *model.WorkProject) bool {
	if p == nil || !h.canRequestSiteAddOnce(c) {
		return false
	}
	ok, _, err := h.siteUnlock.HasActive(repository.SiteAddKey(p.ProjectID), currentUserID(c))
	return err == nil && ok
}

// siteAddAllowed 정식 권한(canRegisterProjectSites) 또는 1회 권한. 수행 구조 설계(canDesignProjectWork)에는 쓰지 않는다.
func (h *ProjectHandler) siteAddAllowed(c echo.Context, p *model.WorkProject, org string) bool {
	return canRegisterProjectSites(c, h, p, org) || h.siteAddGrantActive(c, p)
}

// finishSiteAddByGrant 1회 권한으로 등록에 성공했으면 권한을 지우고 변경 이력을 남긴다. 정식 권한자는 건드리지 않는다.
func (h *ProjectHandler) finishSiteAddByGrant(c echo.Context, p *model.WorkProject, org string, count int) {
	if p == nil || count <= 0 || canRegisterProjectSites(c, h, p, org) || !h.siteAddGrantActive(c, p) {
		return
	}
	uid := currentUserID(c)
	_ = h.siteUnlock.Revoke(repository.SiteAddKey(p.ProjectID), uid)
	h.siteUnlock.LogSiteAddByUnlock(p.ProjectID, uid, count)
}

// SiteAddUnlock POST /projects/:id/sites/unlock — 1회 등록 요청.
func (h *ProjectHandler) SiteAddUnlock(c echo.Context) error {
	p, org, err := h.loadEquipProject(c)
	if err != nil {
		return err
	}
	back := "/projects/" + p.ProjectID + "/sites"
	jsonOut := wantsJSON(c) || wantsJSONError(c)
	fail := func(code int, msg string) error {
		if jsonOut {
			return c.JSON(code, map[string]string{"error": msg})
		}
		return redirectEquipErr(c, back, msg)
	}
	if !h.canRequestSiteAddOnce(c) {
		if jsonOut {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "이 사업의 사이트를 등록할 권한이 없습니다"})
		}
		return echo.ErrForbidden
	}
	if canRegisterProjectSites(c, h, p, org) {
		if jsonOut {
			return c.JSON(http.StatusOK, map[string]interface{}{"granted": false, "needed": false})
		}
		return c.Redirect(http.StatusSeeOther, back)
	}
	uid := currentUserID(c)
	key := repository.SiteAddKey(p.ProjectID)
	if until, locked := h.siteUnlock.SiteAddLockedUntil(uid, time.Now()); locked {
		return fail(http.StatusTooManyRequests, "비밀번호를 5회 틀려 잠겼습니다. "+until.Format("15:04")+" 이후 다시 시도하세요")
	}
	pw := strings.TrimSpace(c.FormValue("unlock_password"))
	ok := verifyAndUpgradeSetting(h.settings, repository.SettingASCompletedEditPassword, pw)
	_ = h.siteUnlock.LogAttempt(key, uid, ctxString(c, "username"), c.RealIP(), ok)
	if !ok {
		return fail(http.StatusForbidden, "수정 비밀번호가 올바르지 않습니다")
	}
	exp, err := h.siteUnlock.Grant(key, uid)
	if err != nil {
		return err
	}
	if jsonOut {
		return c.JSON(http.StatusOK, map[string]interface{}{"granted": true, "expires_at": exp.Format(time.RFC3339)})
	}
	return c.Redirect(http.StatusSeeOther, back+"?ok="+url.QueryEscape("1회 등록 권한이 부여되었습니다(30분, 1회 사용)"))
}

// SitesQuick GET /projects/:id/sites/quick — 업무 등록 화면의 「사이트 간이 등록」 팝업 데이터. (95-224 §2)
// 검색 대상은 이 사업의 고객에 이미 연결된 하위 기관뿐이다(마스터 전체 검색은 이 팝업에서 하지 않는다).
// 등록은 기존 POST /projects/:id/sites/add-schools 를 그대로 쓴다.
func (h *ProjectHandler) SitesQuick(c echo.Context) error {
	p, org, err := h.loadEquipProject(c)
	if err != nil {
		return err
	}
	_, total, _ := h.pe.ListSites(p.ProjectID, "", "", "", 1, 1)
	cid, _ := h.pe.ProjectSiteCustomerID(p.ProjectID)
	children, err := h.pe.ListChildCustomers(p.ProjectID, cid)
	if err != nil {
		return err
	}
	allowed := h.siteAddAllowed(c, p, org)
	return c.JSON(http.StatusOK, map[string]interface{}{
		"projectId":      p.ProjectID,
		"siteTotal":      total,
		"canRegister":    allowed,
		"canRequestOnce": !allowed && h.canRequestSiteAddOnce(c),
		"children":       children,
	})
}

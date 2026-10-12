package handler

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/auditlog"
	"customer-support/internal/model"
	"customer-support/internal/repository"
	"customer-support/internal/service/approval"
)

type ApprovalSettingsHandler struct {
	eng *approval.Engine
}

func (h *ApprovalSettingsHandler) engine() *approval.Engine {
	if h == nil {
		return nil
	}
	return h.eng
}

func (h *ApprovalSettingsHandler) settingsOrg(c echo.Context) (string, error) {
	if !isAdminRole(c) && !hasPerm(c, model.PermApprovalAdmin) {
		return "", echo.ErrForbidden
	}
	org := currentOrg(c)
	if org == "" || org == repository.OrgAll {
		return "", echo.ErrNotFound
	}
	if currentRole(c) == model.RoleOrgAdmin {
		home := strings.TrimSpace(ctxString(c, "org_id"))
		if home != "" && home != org {
			return "", echo.ErrNotFound
		}
	}
	return org, nil
}

func (h *ApprovalSettingsHandler) Page(c echo.Context) error {
	org, err := h.settingsOrg(c)
	if err != nil {
		return err
	}
	p, err := h.engine().GetPolicy(org)
	if err != nil {
		return err
	}
	return c.Render(http.StatusOK, "admin/approval_settings.html", map[string]interface{}{
		"Title":  "결재 설정",
		"Active": NavApproval,
		"OrgID":  org,
		"Policy": p,
		"OK":     strings.TrimSpace(c.QueryParam("ok")),
		"Error":  strings.TrimSpace(c.QueryParam("err")),
	})
}

func (h *ApprovalSettingsHandler) Save(c echo.Context) error {
	org, err := h.settingsOrg(c)
	if err != nil {
		return err
	}
	if isReadOnly(c) || viewAsActive(c) {
		return echo.ErrForbidden
	}
	p := approval.Policy{
		OrgID:                  org,
		DelegByHead:            c.FormValue("deleg_by_head") == "1",
		DelegByAdmin:           c.FormValue("deleg_by_admin") == "1",
		DelegWithLeave:         c.FormValue("deleg_with_leave") == "1",
		AdminEmergencyApprove:  c.FormValue("admin_emergency_approve") == "1",
		SelfApproveLeave:       c.FormValue("self_approve_leave") == "1",
		SelfApproveQuote:       c.FormValue("self_approve_quote") == "1",
		NotifyAdminSelfApprove: c.FormValue("notify_admin_self_approve") == "1",
	}
	a := approval.Actor{
		UserID: currentUserID(c), Username: ctxString(c, "username"),
		Name: currentUserDisplayName(c), OrgID: org, Role: currentRole(c),
	}
	if err := h.engine().SavePolicy(a, org, p); err != nil {
		return c.Redirect(http.StatusSeeOther, "/admin/approval-settings?err="+url.QueryEscape(err.Error()))
	}
	accessLog(c, auditlog.Record{
		Action: auditlog.ActionUpdate, Result: auditlog.ResultOK,
		TargetTable: "approval_policies", SubjectName: org, Detail: "결재 설정 저장",
	})
	return c.Redirect(http.StatusSeeOther, "/admin/approval-settings?ok="+url.QueryEscape("저장했습니다."))
}

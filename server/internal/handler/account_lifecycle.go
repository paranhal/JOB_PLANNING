package handler

import (
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"customer-support/internal/auditlog"
	"customer-support/internal/model"
)

var onceTempPW sync.Map

func putOncePassword(pw string) string {
	tok := uuid.NewString()
	onceTempPW.Store(tok, pw)
	return tok
}

func takeOncePassword(tok string) string {
	tok = strings.TrimSpace(tok)
	if tok == "" {
		return ""
	}
	v, ok := onceTempPW.LoadAndDelete(tok)
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

func (h *AuthHandler) requireVision(c echo.Context) bool {
	return currentRole(c) == model.RoleVisionAdmin
}

func redirectUsersErr(c echo.Context, msg string) error {
	return c.Redirect(http.StatusSeeOther, "/admin/users?err="+url.QueryEscape(msg))
}

func (h *AuthHandler) MembersRequestCreate(c echo.Context) error {
	if !h.isAdmin(c) || isReadOnly(c) {
		return h.forbidden(c)
	}
	if currentRole(c) != model.RoleOrgAdmin {
		return h.forbidden(c)
	}
	home := sessionHomeOrgID(c)
	if home == "" {
		return c.Redirect(http.StatusSeeOther, "/admin/my-org/members?err="+url.QueryEscape("세션에 조직이 없습니다."))
	}
	role := model.NormalizeRole(c.FormValue("job"))
	if !model.RequestRoleOK(role) {
		return c.Redirect(http.StatusSeeOther, "/admin/my-org/members?err="+url.QueryEscape("일반 업무만 요청할 수 있습니다."))
	}
	name := strings.TrimSpace(c.FormValue("full_name"))
	user := strings.TrimSpace(c.FormValue("username"))
	if name == "" || user == "" {
		return c.Redirect(http.StatusSeeOther, "/admin/my-org/members?err="+url.QueryEscape("이름과 아이디를 입력하세요."))
	}
	taken, err := h.userRepo.UsernameTaken(user, "")
	if err != nil || taken {
		return c.Redirect(http.StatusSeeOther, "/admin/my-org/members?err="+url.QueryEscape(model.UsernameReservedMsg))
	}
	req := &model.AccountRequest{
		OrgID: home, RequestedBy: ctxString(c, "user_id"), FullName: name, Username: user,
		Role: role, Mobile: strings.TrimSpace(c.FormValue("mobile")), Reason: strings.TrimSpace(c.FormValue("reason")),
	}
	if h.accounts == nil {
		return c.Redirect(http.StatusSeeOther, "/admin/my-org/members?err="+url.QueryEscape("요청을 남기지 못했습니다."))
	}
	if err := h.accounts.CreateRequest(req); err != nil {
		return c.Redirect(http.StatusSeeOther, "/admin/my-org/members?err="+url.QueryEscape("요청을 남기지 못했습니다."))
	}
	accessLog(c, auditlog.Record{
		Action: auditlog.ActionCreate, Result: auditlog.ResultOK,
		TargetTable: "account_requests", TargetID: req.RequestID, SubjectName: name,
		Detail: "등록 요청 @" + user,
	})
	return c.Redirect(http.StatusSeeOther, "/admin/my-org/members?ok="+url.QueryEscape("등록 요청을 보냈습니다."))
}

func (h *AuthHandler) AccountRequestCancel(c echo.Context) error {
	if currentRole(c) != model.RoleOrgAdmin || isReadOnly(c) {
		return h.forbidden(c)
	}
	a, _ := h.accounts.GetRequest(c.Param("id"))
	if a == nil || a.RequestedBy != ctxString(c, "user_id") || a.Status != model.AccountReqPending {
		return echo.ErrNotFound
	}
	_ = h.accounts.Decide(a.RequestID, model.AccountReqCancelled, ctxString(c, "user_id"), "요청 취소", "", "")
	accessLog(c, auditlog.Record{
		Action: auditlog.ActionUpdate, Result: auditlog.ResultOK,
		TargetTable: "account_requests", TargetID: a.RequestID, Detail: "요청 취소",
	})
	return c.Redirect(http.StatusSeeOther, "/admin/my-org/members?ok="+url.QueryEscape("요청을 취소했습니다."))
}

func (h *AuthHandler) AccountRequestList(c echo.Context) error {
	if !h.requireVision(c) {
		return h.forbidden(c)
	}
	st := strings.TrimSpace(c.QueryParam("status"))
	if st == "" {
		st = model.AccountReqPending
	}
	items, _ := h.accounts.ListRequests(st, "", "")
	for i := range items {
		if d := h.userRepo.FindDeletedByNameMobile(items[i].FullName, items[i].Mobile); d != nil {
			items[i].RestoreUserID = d.UserID
		}
	}
	return c.Render(http.StatusOK, "admin/account_requests.html", map[string]interface{}{
		"Title": "등록 요청", "Active": NavUsers, "Items": items,
		"OK": c.QueryParam("ok"), "Error": c.QueryParam("err"),
		"TempPW": takeOncePassword(c.QueryParam("once")),
	})
}

func (h *AuthHandler) AccountRequestDecide(c echo.Context) error {
	if !h.requireVision(c) || isReadOnly(c) {
		return h.forbidden(c)
	}
	a, _ := h.accounts.GetRequest(c.Param("id"))
	if a == nil || a.Status != model.AccountReqPending {
		return echo.ErrNotFound
	}
	how := strings.TrimSpace(c.FormValue("how"))
	if how == "reject" {
		note := strings.TrimSpace(c.FormValue("decision_note"))
		if note == "" {
			return c.Redirect(http.StatusSeeOther, "/admin/users/requests?err="+url.QueryEscape("반려 사유가 필요합니다."))
		}
		_ = h.accounts.Decide(a.RequestID, model.AccountReqRejected, ctxString(c, "user_id"), note, "", "")
		accessLog(c, auditlog.Record{
			Action: auditlog.ActionUpdate, Result: auditlog.ResultOK,
			TargetTable: "account_requests", TargetID: a.RequestID, Detail: "반려 " + note,
		})
		return c.Redirect(http.StatusSeeOther, "/admin/users/requests?ok="+url.QueryEscape("반려했습니다."))
	}
	taken, err := h.userRepo.UsernameTaken(a.Username, strings.TrimSpace(c.FormValue("restore_user_id")))
	if err != nil || taken {
		return c.Redirect(http.StatusSeeOther, "/admin/users/requests?err="+url.QueryEscape(model.UsernameReservedMsg))
	}
	pw, err := randomTempPassword()
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/admin/users/requests?err="+url.QueryEscape("임시 비밀번호를 만들지 못했습니다."))
	}
	restoreID := strings.TrimSpace(c.FormValue("restore_user_id"))
	actor := ctxString(c, "user_id")
	if restoreID != "" {
		if err := h.userRepo.Restore(restoreID, actor, a.Role, a.OrgID, "등록 요청 복구 승인"); err != nil {
			return c.Redirect(http.StatusSeeOther, "/admin/users/requests?err="+url.QueryEscape(err.Error()))
		}
		_ = h.userRepo.UpdatePassword(restoreID, HashPassword(pw))
		_ = h.userRepo.SetMustChangePassword(restoreID, true)
		_ = h.accounts.Decide(a.RequestID, model.AccountReqApproved, actor, "", restoreID, restoreID)
		tok := putOncePassword(pw)
		accessLog(c, auditlog.Record{
			Action: auditlog.ActionUpdate, Result: auditlog.ResultOK,
			TargetTable: "users", TargetID: restoreID, SubjectName: a.FullName, Detail: "복구 승인",
		})
		return c.Redirect(http.StatusSeeOther, "/admin/users/requests?ok="+url.QueryEscape("복구했습니다. 임시 비밀번호는 한 번만 보입니다.")+"&once="+url.QueryEscape(tok))
	}
	u := &model.User{
		Username: a.Username, PasswordHash: HashPassword(pw), FullName: a.FullName,
		Role: a.Role, IsActive: true, OrgID: a.OrgID, Mobile: a.Mobile, MustChangePassword: true,
	}
	if err := h.userRepo.Create(u); err != nil {
		return c.Redirect(http.StatusSeeOther, "/admin/users/requests?err="+url.QueryEscape("계정을 만들지 못했습니다."))
	}
	_ = h.userRepo.SetMustChangePassword(u.UserID, true)
	_ = h.accounts.Decide(a.RequestID, model.AccountReqApproved, actor, "", u.UserID, "")
	tok := putOncePassword(pw)
	accessLog(c, auditlog.Record{
		Action: auditlog.ActionCreate, Result: auditlog.ResultOK,
		TargetTable: "users", TargetID: u.UserID, SubjectName: u.FullName, Detail: "요청 승인 계정 생성",
	})
	return c.Redirect(http.StatusSeeOther, "/admin/users/requests?ok="+url.QueryEscape("승인했습니다. 임시 비밀번호는 한 번만 보입니다.")+"&once="+url.QueryEscape(tok))
}

func (h *AuthHandler) DeletedUsers(c echo.Context) error {
	if !h.requireVision(c) {
		return h.forbidden(c)
	}
	items, _ := h.userRepo.ListDeleted()
	return c.Render(http.StatusOK, "admin/users_deleted.html", map[string]interface{}{
		"Title": "삭제 내역", "Active": NavUsers, "Items": items,
		"OK": c.QueryParam("ok"), "Error": c.QueryParam("err"),
		"TempPW": takeOncePassword(c.QueryParam("once")),
		"Orgs": func() []model.Org {
			if h.orgRepo == nil {
				return nil
			}
			o, _ := h.orgRepo.ListActive()
			return o
		}(),
	})
}

func (h *AuthHandler) RestoreUser(c echo.Context) error {
	if !h.requireVision(c) || isReadOnly(c) {
		return h.forbidden(c)
	}
	if !h.verifyVisionAdminPassword(c, c.FormValue("vision_password")) {
		return c.Redirect(http.StatusSeeOther, "/admin/users/deleted?err="+url.QueryEscape("비젼관리자 비밀번호가 올바르지 않습니다."))
	}
	id := strings.TrimSpace(c.Param("id"))
	role := model.NormalizeRole(c.FormValue("job"))
	orgID := strings.TrimSpace(c.FormValue("org_id"))
	note := strings.TrimSpace(c.FormValue("reason"))
	if note == "" {
		return c.Redirect(http.StatusSeeOther, "/admin/users/deleted?err="+url.QueryEscape("복구 사유가 필요합니다."))
	}
	if err := h.userRepo.Restore(id, ctxString(c, "user_id"), role, orgID, note); err != nil {
		return c.Redirect(http.StatusSeeOther, "/admin/users/deleted?err="+url.QueryEscape(err.Error()))
	}
	pw, err := randomTempPassword()
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/admin/users/deleted?err="+url.QueryEscape("임시 비밀번호를 만들지 못했습니다."))
	}
	_ = h.userRepo.UpdatePassword(id, HashPassword(pw))
	_ = h.userRepo.SetMustChangePassword(id, true)
	tok := putOncePassword(pw)
	accessLog(c, auditlog.Record{
		Action: auditlog.ActionUpdate, Result: auditlog.ResultOK,
		TargetTable: "users", TargetID: id, Detail: "계정 복구", Reason: note,
	})
	return c.Redirect(http.StatusSeeOther, "/admin/users/deleted?ok="+url.QueryEscape("복구했습니다. 임시 비밀번호는 한 번만 보입니다.")+"&once="+url.QueryEscape(tok))
}


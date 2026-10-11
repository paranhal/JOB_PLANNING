package handler

import (
	"crypto/rand"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/auditlog"
	"customer-support/internal/mailer"
	"customer-support/internal/model"
	"customer-support/internal/passwd"
)

func sessionHomeOrgID(c echo.Context) string {
	return strings.TrimSpace(ctxString(c, "org_id"))
}

func (h *AuthHandler) memberInHomeOrg(c echo.Context, u *model.User) bool {
	if u == nil {
		return false
	}
	home := sessionHomeOrgID(c)
	if home == "" || u.OrgID != home {
		return false
	}
	return true
}

// adminVisibleUser D-32: 조직관리자(세션 조직이 있을 때)는 자기 조직원만. 비젼관리자는 전체.
func (h *AuthHandler) adminVisibleUser(c echo.Context, u *model.User) bool {
	if u == nil {
		return false
	}
	if currentRole(c) != model.RoleOrgAdmin {
		return true
	}
	home := sessionHomeOrgID(c)
	if home == "" {
		return true
	}
	return strings.TrimSpace(u.OrgID) == home
}

func (h *AuthHandler) loadAdminScopedUser(c echo.Context, id string) (*model.User, error) {
	u, _ := h.userRepo.GetByID(strings.TrimSpace(id))
	if u == nil || !h.adminVisibleUser(c, u) {
		return nil, echo.ErrNotFound
	}
	return u, nil
}

func (h *AuthHandler) MembersList(c echo.Context) error {
	if !h.isAdmin(c) {
		return h.forbidden(c)
	}
	home := sessionHomeOrgID(c)
	orgName := home
	if home != "" && h.orgRepo != nil {
		if o, _ := h.orgRepo.Get(home); o != nil {
			orgName = o.OrgName
		}
	}
	var members []model.User
	if home != "" {
		all, _ := h.userRepo.ListAll()
		for _, u := range all {
			if u.OrgID == home && !model.IsAdminGrade(u.Role) {
				members = append(members, u)
			}
		}
	}
	var reqs []model.AccountRequest
	if h.accounts != nil {
		reqs, _ = h.accounts.ListRequests("", home, ctxString(c, "user_id"))
	}
	return c.Render(http.StatusOK, "admin/members.html", map[string]interface{}{
		"Title":    "소속 인원 관리",
		"Active":   NavMembers,
		"OrgID":    home,
		"OrgName":  orgName,
		"Members":  members,
		"Requests": reqs,
		"CanEdit":  !isReadOnly(c),
		"FlashOK":  strings.TrimSpace(c.QueryParam("ok")),
		"FlashErr": strings.TrimSpace(c.QueryParam("err")),
		"TempPW":   takeOncePassword(c.QueryParam("once")),
	})
}

func (h *AuthHandler) MemberCreate(c echo.Context) error {
	return h.MembersRequestCreate(c)
}

func (h *AuthHandler) MemberUpdate(c echo.Context) error {
	if !h.isAdmin(c) || isReadOnly(c) {
		return h.forbidden(c)
	}
	u, _ := h.userRepo.GetByID(c.Param("id"))
	if !h.memberInHomeOrg(c, u) {
		return echo.ErrNotFound
	}
	if name := strings.TrimSpace(c.FormValue("full_name")); name != "" {
		u.FullName = name
	}
	u.Role = resolveAdminRole(c, u.Role)
	u.OrgID = sessionHomeOrgID(c)
	u.IsReadOnly = c.FormValue("is_readonly") == "1"
	u.IsTest = c.FormValue("is_test") == "1"
	u.IsActive = c.FormValue("is_active") != "0"
	bindUserContact(c, u)
	if err := h.userRepo.Update(u); err != nil {
		return c.Redirect(http.StatusSeeOther, "/admin/my-org/members?err="+url.QueryEscape("저장하지 못했습니다."))
	}
	return c.Redirect(http.StatusSeeOther, "/admin/my-org/members?ok=saved")
}

func (h *AuthHandler) MemberSetWork(c echo.Context) error {
	if !h.isAdmin(c) || isReadOnly(c) {
		return h.forbidden(c)
	}
	u, _ := h.userRepo.GetByID(c.Param("id"))
	if !h.memberInHomeOrg(c, u) {
		return echo.ErrNotFound
	}
	job := model.NormalizeRole(c.FormValue("job"))
	if job != model.RoleSales && job != model.RoleTech && job != model.RoleSupport {
		return c.Redirect(http.StatusSeeOther, "/admin/my-org/members?err="+url.QueryEscape("업무를 고르세요."))
	}
	u.Role = job
	u.OrgID = sessionHomeOrgID(c)
	_ = h.userRepo.Update(u)
	accessLog(c, auditlog.Record{
		Action: auditlog.ActionUpdate, Result: auditlog.ResultOK,
		TargetTable: "users", TargetID: u.UserID, SubjectName: u.FullName,
		Detail: "소속 인원 업무 " + model.RoleLabel(job),
	})
	return c.Redirect(http.StatusSeeOther, "/admin/my-org/members?ok=saved")
}

func (h *AuthHandler) MemberResetPassword(c echo.Context) error {
	if !h.isAdmin(c) || isReadOnly(c) {
		return h.forbidden(c)
	}
	u, _ := h.userRepo.GetByID(c.Param("id"))
	if !h.memberInHomeOrg(c, u) {
		return echo.ErrNotFound
	}
	how := strings.TrimSpace(c.FormValue("how"))
	if how == "link" && strings.TrimSpace(u.Email) == "" {
		return c.Redirect(http.StatusSeeOther, "/admin/my-org/members?err="+url.QueryEscape("이메일이 없어 링크를 보낼 수 없습니다."))
	}
	pw, err := randomTempPassword()
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/admin/my-org/members?err="+url.QueryEscape("임시 비밀번호를 만들지 못했습니다."))
	}
	if err := h.userRepo.UpdatePassword(u.UserID, HashPassword(pw)); err != nil {
		return c.Redirect(http.StatusSeeOther, "/admin/my-org/members?err="+url.QueryEscape("비밀번호를 바꾸지 못했습니다."))
	}
	_ = h.userRepo.SetMustChangePassword(u.UserID, true)
	if how == "link" && h.db != nil {
		_, _ = mailer.Enqueue(h.db, mailer.FromApp(h.cfg), mailer.Item{
			ToAddr:    u.Email,
			Subject:   "비밀번호 초기화",
			Body:      "관리자가 비밀번호를 초기화했습니다. 안내받은 임시 비밀번호로 로그인한 뒤 바로 바꾸세요.",
			Purpose:   "password_reset",
			CreatedBy: ctxString(c, "user_name"),
			OrgID:     sessionHomeOrgID(c),
		})
	}
	accessLog(c, auditlog.Record{
		Action: auditlog.ActionUpdate, Result: auditlog.ResultOK,
		TargetTable: "users", TargetID: u.UserID, SubjectName: u.FullName,
		Detail: "비밀번호 초기화 방법=" + how,
	})
	tok := putOncePassword(pw)
	return c.Redirect(http.StatusSeeOther, "/admin/my-org/members?ok="+url.QueryEscape("임시 비밀번호를 한 번만 보여 줍니다.")+"&once="+url.QueryEscape(tok))
}

func (h *AuthHandler) MemberDelete(c echo.Context) error {
	return h.forbidden(c)
}

func randomTempPassword() (string, error) {
	const letters = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	buf := make([]byte, passwd.MinLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, passwd.MinLen)
	for i, b := range buf {
		out[i] = letters[int(b)%len(letters)]
	}
	return string(out), nil
}

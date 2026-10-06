package handler

import (
	"crypto/rand"
	"fmt"
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
	return c.Render(http.StatusOK, "admin/members.html", map[string]interface{}{
		"Title":    "소속 인원 관리",
		"Active":   NavMembers,
		"OrgID":    home,
		"OrgName":  orgName,
		"Members":  members,
		"CanEdit":  !isReadOnly(c),
		"FlashOK":  strings.TrimSpace(c.QueryParam("ok")),
		"FlashErr": strings.TrimSpace(c.QueryParam("err")),
		"TempPW":   strings.TrimSpace(c.QueryParam("pw")),
	})
}

func (h *AuthHandler) MemberCreate(c echo.Context) error {
	if !h.isAdmin(c) || isReadOnly(c) {
		return h.forbidden(c)
	}
	home := sessionHomeOrgID(c)
	if home == "" {
		return c.Redirect(http.StatusSeeOther, "/admin/my-org/members?err="+url.QueryEscape("세션에 조직이 없습니다."))
	}
	role := resolveAdminRole(c, "")
	if passwordTooShort(c.FormValue("password")) {
		return c.Redirect(http.StatusSeeOther, "/admin/my-org/members?err="+url.QueryEscape(passwordMinLenMsg()))
	}
	u := &model.User{
		Username:     strings.TrimSpace(c.FormValue("username")),
		PasswordHash: HashPassword(c.FormValue("password")),
		FullName:     strings.TrimSpace(c.FormValue("full_name")),
		Role:         role,
		IsActive:     true,
		OrgID:        home,
		IsReadOnly:   c.FormValue("is_readonly") == "1",
		IsTest:       c.FormValue("is_test") == "1",
	}
	bindUserContact(c, u)
	if u.Username == "" || u.FullName == "" {
		return c.Redirect(http.StatusSeeOther, "/admin/my-org/members?err="+url.QueryEscape("이름과 아이디를 입력하세요."))
	}
	if taken, err := h.userRepo.UsernameTaken(u.Username, ""); err != nil || taken {
		return c.Redirect(http.StatusSeeOther, "/admin/my-org/members?err="+url.QueryEscape("이미 있는 아이디입니다."))
	}
	if err := h.userRepo.Create(u); err != nil {
		return c.Redirect(http.StatusSeeOther, "/admin/my-org/members?err="+url.QueryEscape("계정을 만들지 못했습니다."))
	}
	accessLog(c, auditlog.Record{
		Action: auditlog.ActionCreate, Result: auditlog.ResultOK,
		TargetTable: "users", TargetID: u.UserID, SubjectType: "user",
		SubjectID: u.UserID, SubjectName: u.FullName,
		Detail: "소속 인원 추가 조직=" + home,
	})
	return c.Redirect(http.StatusSeeOther, "/admin/my-org/members?ok=saved")
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
	return c.Redirect(http.StatusSeeOther, "/admin/my-org/members?ok="+url.QueryEscape("임시 비밀번호를 한 번만 보여 줍니다.")+"&pw="+url.QueryEscape(pw))
}

func (h *AuthHandler) MemberDelete(c echo.Context) error {
	if !h.isAdmin(c) || isReadOnly(c) {
		return h.forbidden(c)
	}
	u, _ := h.userRepo.GetByID(c.Param("id"))
	if !h.memberInHomeOrg(c, u) {
		return echo.ErrNotFound
	}
	n := h.userRepo.AssignedWorkCount(u.UserID)
	if n > 0 {
		return c.Redirect(http.StatusSeeOther, "/admin/my-org/members?err="+url.QueryEscape(fmt.Sprintf("담당 업무 %d건이 있습니다.", n)))
	}
	if err := h.userRepo.Delete(u.UserID); err != nil {
		return c.Redirect(http.StatusSeeOther, "/admin/my-org/members?err="+url.QueryEscape("지우지 못했습니다."))
	}
	return c.Redirect(http.StatusSeeOther, "/admin/my-org/members?ok=deleted")
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

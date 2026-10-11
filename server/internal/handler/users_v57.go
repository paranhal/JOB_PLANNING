package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/auditlog"
	"customer-support/internal/model"
)

func (h *AuthHandler) UserSetWork(c echo.Context) error {
	if !h.isAdmin(c) || isReadOnly(c) {
		return h.forbidden(c)
	}
	u, err := h.loadAdminScopedUser(c, c.Param("id"))
	if err != nil {
		return err
	}
	if model.IsAdminGrade(u.Role) {
		return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape(
			"막힘 — 관리 등급에는 업무를 둘 수 없습니다: "+u.FullName))
	}
	job := model.NormalizeRole(c.FormValue("job"))
	if job != model.RoleSales && job != model.RoleTech && job != model.RoleSupport {
		return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape("업무를 고르세요."))
	}
	orig := *u
	u.Role = job
	sum := fmt.Sprintf("등급·업무 %s → %s", model.RoleLabel(orig.Role), model.RoleLabel(job))
	if err := h.userRepo.UpdateAudited(u, "", sum, toJSON(userPublic(&orig)), toJSON(userPublic(u))); err != nil {
		return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape("감사 기록을 남기지 못해 저장을 되돌렸습니다."))
	}
	accessLog(c, auditlog.Record{
		Action: auditlog.ActionUpdate, Result: auditlog.ResultOK,
		TargetTable: "users", TargetID: u.UserID, SubjectType: "user",
		SubjectID: u.UserID, SubjectName: u.FullName,
		Detail: sum, BeforeJSON: toJSON(userPublic(&orig)), AfterJSON: toJSON(userPublic(u)),
	})
	return c.Redirect(http.StatusSeeOther, "/users?ok=saved")
}

func (h *AuthHandler) UsersBulkOrg(c echo.Context) error {
	if !h.isAdmin(c) || isReadOnly(c) {
		return h.forbidden(c)
	}
	_ = c.Request().ParseForm()
	ids := c.Request().PostForm["user_ids"]
	orgID := strings.TrimSpace(c.FormValue("org_id"))
	users, _, err := h.usersForBulk(c, ids)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape(err.Error()))
	}
	var blocked []string
	for _, u := range users {
		if u.Role == model.RoleVisionAdmin {
			blocked = append(blocked, u.FullName)
		}
	}
	if len(blocked) > 0 {
		return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape(
			fmt.Sprintf("막힘 — 비젼관리자는 조직에 속하지 않습니다. %d명: %s", len(blocked), strings.Join(blocked, ", "))))
	}
	if orgID == "" {
		return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape("조직을 고르세요."))
	}
	for i := range users {
		orig := users[i]
		users[i].OrgID = orgID
		sum := fmt.Sprintf("조직 %s → %s (%s)", orgLabel(orig.OrgID), orgLabel(orgID), orig.FullName)
		if err := h.userRepo.UpdateAudited(&users[i], "", sum, toJSON(userPublic(&orig)), toJSON(userPublic(&users[i]))); err != nil {
			return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape("감사 기록을 남기지 못해 저장을 되돌렸습니다: "+orig.FullName))
		}
		accessLog(c, auditlog.Record{
			Action: auditlog.ActionUpdate, Result: auditlog.ResultOK,
			TargetTable: "users", TargetID: orig.UserID, SubjectType: "user",
			SubjectID: orig.UserID, SubjectName: orig.FullName, Detail: sum,
		})
	}
	return c.Redirect(http.StatusSeeOther, "/users?ok=saved")
}

func (h *AuthHandler) UsersBulkJob(c echo.Context) error {
	if !h.isAdmin(c) || isReadOnly(c) {
		return h.forbidden(c)
	}
	_ = c.Request().ParseForm()
	ids := c.Request().PostForm["user_ids"]
	job := model.NormalizeRole(c.FormValue("job"))
	users, _, err := h.usersForBulk(c, ids)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape(err.Error()))
	}
	var blocked []string
	for _, u := range users {
		if model.IsAdminGrade(u.Role) {
			blocked = append(blocked, u.FullName)
		}
	}
	if len(blocked) > 0 {
		return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape(
			fmt.Sprintf("막힘 — 관리 등급에는 업무를 둘 수 없습니다. %d명: %s. 업무를 주려면 먼저 관리 등급을 「일반」으로 바꾸세요.",
				len(blocked), strings.Join(blocked, ", "))))
	}
	if job != model.RoleSales && job != model.RoleTech && job != model.RoleSupport {
		return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape("업무를 고르세요."))
	}
	for i := range users {
		orig := users[i]
		users[i].Role = job
		sum := fmt.Sprintf("등급·업무 %s → %s (%s)", model.RoleLabel(orig.Role), model.RoleLabel(job), orig.FullName)
		if err := h.userRepo.UpdateAudited(&users[i], "", sum, toJSON(userPublic(&orig)), toJSON(userPublic(&users[i]))); err != nil {
			return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape("감사 기록을 남기지 못해 저장을 되돌렸습니다: "+orig.FullName))
		}
		accessLog(c, auditlog.Record{
			Action: auditlog.ActionUpdate, Result: auditlog.ResultOK,
			TargetTable: "users", TargetID: orig.UserID, SubjectType: "user",
			SubjectID: orig.UserID, SubjectName: orig.FullName, Detail: sum,
		})
	}
	return c.Redirect(http.StatusSeeOther, "/users?ok=saved")
}

func (h *AuthHandler) UsersBulkReadonly(c echo.Context) error {
	if !h.isAdmin(c) || isReadOnly(c) {
		return h.forbidden(c)
	}
	_ = c.Request().ParseForm()
	ids := c.Request().PostForm["user_ids"]
	reason := strings.TrimSpace(c.FormValue("reason"))
	on := c.FormValue("on") != "0"
	users, _, err := h.usersForBulk(c, ids)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape(err.Error()))
	}
	if !on && reason == "" {
		return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape("읽기전용을 끄려면 사유가 필요합니다."))
	}
	if on && reason == "" {
		return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape("읽기전용을 켜려면 사유가 필요합니다."))
	}
	verb := "읽기전용 끄기 → 켜기"
	if !on {
		verb = "읽기전용 켜기 → 끄기"
	}
	for i := range users {
		orig := users[i]
		if orig.IsReadOnly == on {
			continue
		}
		users[i].IsReadOnly = on
		sum := verb + " (" + orig.FullName + ")"
		if err := h.userRepo.UpdateAudited(&users[i], reason, sum, toJSON(userPublic(&orig)), toJSON(userPublic(&users[i]))); err != nil {
			return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape("감사 기록을 남기지 못해 저장을 되돌렸습니다: "+orig.FullName))
		}
		accessLog(c, auditlog.Record{
			Action: auditlog.ActionUpdate, Result: auditlog.ResultOK,
			TargetTable: "users", TargetID: orig.UserID, SubjectType: "user",
			SubjectID: orig.UserID, SubjectName: orig.FullName, Detail: sum, Reason: reason,
		})
	}
	return c.Redirect(http.StatusSeeOther, "/users?ok=saved")
}

func (h *AuthHandler) UsersBulkDeactivate(c echo.Context) error {
	if !h.isAdmin(c) || isReadOnly(c) {
		return h.forbidden(c)
	}
	_ = c.Request().ParseForm()
	ids := c.Request().PostForm["user_ids"]
	users, names, err := h.usersForBulk(c, ids)
	if err != nil {
		return c.Redirect(http.StatusSeeOther, "/users?err="+url.QueryEscape(err.Error()))
	}
	for i := range users {
		users[i].IsActive = false
		_ = h.userRepo.Update(&users[i])
	}
	accessLog(c, auditlog.Record{
		Action: auditlog.ActionUpdate, Result: auditlog.ResultOK,
		TargetTable: "users", Detail: "일괄 비활성화 " + strings.Join(names, ", "),
	})
	return c.Redirect(http.StatusSeeOther, "/users?ok=saved")
}

func (h *AuthHandler) usersForBulk(c echo.Context, ids []string) ([]model.User, []string, error) {
	if len(ids) == 0 {
		return nil, nil, fmt.Errorf("사람을 고르세요.")
	}
	var out []model.User
	var names []string
	for _, id := range ids {
		u, _ := h.userRepo.GetByID(strings.TrimSpace(id))
		if u == nil || !h.adminVisibleUser(c, u) {
			continue
		}
		out = append(out, *u)
		names = append(names, u.FullName)
	}
	if len(out) == 0 {
		return nil, nil, fmt.Errorf("사람을 고르세요.")
	}
	return out, names, nil
}

package handler

import (
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

// canPickWorkTodayAssignee §38.8 관리자·접수담당만 오늘 내 업무 담당자를 고른다.
func canPickWorkTodayAssignee(c echo.Context) bool {
	return isAdminRole(c) || isOfficeRole(c)
}

func isWorkTodayForbiddenAll(raw string) bool {
	s := strings.TrimSpace(raw)
	switch strings.ToLower(s) {
	case "all", "전체", "팀전체", "team", "__all__", workAssigneeNone:
		return true
	}
	return s == model.StatsTeamLabel
}

func findAssignableUser(users []model.User, q string) *model.User {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil
	}
	for i := range users {
		u := &users[i]
		if strings.TrimSpace(u.FullName) == q || strings.TrimSpace(u.Username) == q || strings.TrimSpace(u.UserID) == q {
			return u
		}
	}
	return nil
}

func findSelfAssignable(c echo.Context, users []model.User) *model.User {
	if u := findAssignableUser(users, currentUserID(c)); u != nil {
		return u
	}
	if u := findAssignableUser(users, currentUserDisplayName(c)); u != nil {
		return u
	}
	return findAssignableUser(users, ctxString(c, "username"))
}

// resolveWorkTodayAssignee 관리자·접수담당의 ?assignee=. 「전체」는 본인으로 되돌린다. §38.8
func resolveWorkTodayAssignee(c echo.Context, users []model.User) (picked *model.User, viewingOther bool) {
	self := findSelfAssignable(c, users)
	raw := strings.TrimSpace(c.QueryParam("assignee"))
	sel := self
	if raw != "" && !isWorkTodayForbiddenAll(raw) {
		if found := findAssignableUser(users, raw); found != nil {
			sel = found
		}
	}
	if sel == nil {
		return nil, false
	}
	return sel, !assigneeIsMine(c, sel.FullName, sel.UserID)
}

func lockWorkTodayOtherView(items []model.WorkListItem) {
	for i := range items {
		items[i].EditLocked = true
		if strings.TrimSpace(items[i].Assignee) != "" {
			items[i].AssigneeOther = true
		}
	}
}

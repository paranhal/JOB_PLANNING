package handler

import (
	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// boardScope 일일 업무 등록과 업무 관리가 같은 범위 규칙을 쓴다.
type boardScope struct {
	Role           string
	RoleUnresolved bool
	MineParam      string
	AssigneeFilter string
	ScopeAll       bool
	ShowToggle     bool
}

// resolveBoardScope 구 화면 Register 의 담당자·전체 규칙을 그대로 옮긴다.
func resolveBoardScope(c echo.Context) boardScope {
	role := currentRole(c)
	s := boardScope{
		Role:           role,
		RoleUnresolved: !model.IsKnownRole(role),
		MineParam:      c.QueryParam("mine"),
		AssigneeFilter: normalizeAssigneeFilter(c.QueryParam("assignee")),
		ScopeAll:       true,
	}
	if s.RoleUnresolved {
		s.ScopeAll = true
		return s
	}
	if model.IsAdminGrade(role) || role == model.RoleSupport {
		s.ScopeAll = s.AssigneeFilter == ""
		return s
	}
	if role == model.RoleTech || role == model.RoleSales {
		s.ShowToggle = true
		if s.MineParam == "0" {
			s.ScopeAll = true
		} else {
			s.ScopeAll = false
			if s.AssigneeFilter == "" {
				s.AssigneeFilter = currentUserDisplayName(c)
			}
		}
	}
	return s
}

func boardCanWrite(c echo.Context) bool {
	if !canWriteWorkboard(c) {
		return false
	}
	if currentOrg(c) == repository.OrgAll {
		return false
	}
	return true
}

package handler

import (
	"strings"
	"unicode"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// boardScope 일일 업무 등록과 업무 관리가 같은 범위 규칙을 쓴다.
type boardScope struct {
	Role             string
	RoleUnresolved   bool
	MineParam        string
	AssigneeFilter   string
	ScopeAll         bool
	ShowToggle       bool
	TeamMembers      []model.User
	MemberIDs        []string
	MembersRequested bool
}

func showBoardMemberFilter(role, view string) bool {
	return model.NormalizeRole(role) == model.RoleOrgAdmin && (view == "" || view == boardViewList || view == boardViewKanban)
}

func parseMembersQuery(c echo.Context) []string {
	if c == nil {
		return nil
	}
	return splitMemberIDs(c.QueryParams()["members"])
}

func splitMemberIDs(vals []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range vals {
		for _, id := range strings.FieldsFunc(v, func(r rune) bool {
			return r == ',' || r == ';' || r == '|' || unicode.IsSpace(r)
		}) {
			id = strings.TrimSpace(id)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func sanitizeMemberIDs(raw []string, team []model.User) []string {
	allow := map[string]bool{}
	for _, u := range team {
		if id := strings.TrimSpace(u.UserID); id != "" {
			allow[id] = true
		}
	}
	var out []string
	seen := map[string]bool{}
	for _, id := range raw {
		id = strings.TrimSpace(id)
		if id == "" || !allow[id] || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func (s boardScope) memberNames() []string {
	want := map[string]bool{}
	for _, id := range s.MemberIDs {
		want[id] = true
	}
	var names []string
	seen := map[string]bool{}
	for _, u := range s.TeamMembers {
		if !want[strings.TrimSpace(u.UserID)] {
			continue
		}
		n := strings.TrimSpace(u.FullName)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		names = append(names, n)
	}
	return names
}

func (h *WorkboardHandler) withBoardMemberFilter(c echo.Context, s boardScope) boardScope {
	if h == nil || h.userRepo == nil || model.NormalizeRole(currentRole(c)) != model.RoleOrgAdmin {
		return s
	}
	team, err := h.userRepo.ListAssignable(currentOrg(c))
	if err != nil {
		team = nil
	}
	s.TeamMembers = team
	raw := parseMembersQuery(c)
	s.MembersRequested = len(raw) > 0
	s.MemberIDs = sanitizeMemberIDs(raw, team)
	if s.MembersRequested {
		s.ScopeAll = false
	}
	return s
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

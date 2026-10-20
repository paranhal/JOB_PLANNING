package handler

import (
	"sort"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type meetingPickerOpt struct {
	Value        string
	Label        string
	Unregistered bool
}

func meetingTeamRole(role string) bool {
	switch model.NormalizeRole(role) {
	case model.RoleOrgAdmin, model.RoleAdmin, model.RoleVisionAdmin, model.RoleCLevel:
		return true
	default:
		return false
	}
}

// meetingTeamScope 회의 팀 자료 권한. view-as 중이면 보고 있는 역할 기준. D-108(94-3)
func meetingTeamScope(c echo.Context) (canTeam bool, orgScope string) {
	role := currentRole(c)
	if model.NormalizeRole(role) == model.RoleCLevel {
		return true, currentOrg(c)
	}
	if isReadOnly(c) {
		return false, currentOrg(c)
	}
	return meetingTeamRole(role), currentOrg(c)
}

func meetingSelfName(c echo.Context) string {
	if strings.TrimSpace(ctxString(c, "view_as_user_id")) != "" {
		if n := strings.TrimSpace(ctxString(c, "view_as_name")); n != "" {
			return n
		}
	}
	return strings.TrimSpace(ctxString(c, "user_name"))
}

func listMeetingPicker(users *repository.UserRepo, orgs *repository.OrgRepo, c echo.Context) []meetingPickerOpt {
	canTeam, org := meetingTeamScope(c)
	if !canTeam || users == nil {
		return nil
	}
	list, err := users.ListAssignable(org)
	if err != nil {
		return nil
	}
	orgNames := map[string]string{}
	if orgs != nil {
		if rows, err := orgs.List(); err == nil {
			for _, o := range rows {
				orgNames[o.OrgID] = o.OrgName
			}
		}
	}
	showOrg := org == repository.OrgAll
	seen := map[string]bool{}
	var out []meetingPickerOpt
	for _, u := range list {
		n := strings.TrimSpace(u.FullName)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		label := n
		if showOrg {
			on := orgNames[strings.TrimSpace(u.OrgID)]
			if on == "" && strings.TrimSpace(u.OrgID) == "" {
				on = model.OrgNameLibrary
			}
			if on != "" {
				label = n + " (" + on + ")"
			}
		}
		out = append(out, meetingPickerOpt{Value: n, Label: label})
	}
	extras := users.AssigneeNamesOnWork(org)
	for _, n := range extras {
		if seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, meetingPickerOpt{Value: n, Label: n, Unregistered: true})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Value < out[j].Value })
	return out
}

func meetingPickerValues(opts []meetingPickerOpt) []model.User {
	var users []model.User
	for _, o := range opts {
		users = append(users, model.User{FullName: o.Value})
	}
	return users
}

func meetingPickerNames(opts []meetingPickerOpt) []string {
	var names []string
	for _, o := range opts {
		if o.Value != "" {
			names = append(names, o.Value)
		}
	}
	return names
}

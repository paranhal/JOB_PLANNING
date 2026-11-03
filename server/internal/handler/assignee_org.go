package handler

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

const errAssigneeSameOrg = "같은 조직의 담당자만 지정할 수 있습니다"

func errAssigneeOrg() error {
	return echo.NewHTTPError(http.StatusBadRequest, errAssigneeSameOrg)
}

func assignableOrgID(c echo.Context) string {
	if c != nil && canSwitchOrg(c) && c.Request() != nil && c.Request().URL != nil {
		if strings.HasPrefix(c.Request().URL.Path, "/exec/work") {
			if q := strings.TrimSpace(c.QueryParam("org")); q != "" {
				return q
			}
		}
	}
	return currentOrg(c)
}

func assigneePickDisabled(c echo.Context) bool {
	org := assignableOrgID(c)
	return org == "" || org == repository.OrgAll
}

func listAssignableForPicker(users *repository.UserRepo, c echo.Context) []model.User {
	if users == nil || c == nil || assigneePickDisabled(c) {
		return nil
	}
	list, err := users.ListAssignable(assignableOrgID(c))
	if err != nil {
		list = nil
	}
	return ensurePickerUsers(users, list, currentUserID(c))
}

const pickerNameUnregistered = "이름 미등록"

func pickerDisplayName(u model.User) string {
	n := strings.TrimSpace(u.FullName)
	if n != "" && !strings.HasPrefix(n, "USR-") && n != "담당자" {
		return n
	}
	n = strings.TrimSpace(u.Username)
	if n != "" && !strings.HasPrefix(n, "USR-") && n != "담당자" {
		return n
	}
	if id := strings.TrimSpace(u.UserID); id != "" {
		return id
	}
	return pickerNameUnregistered
}

func pickerNamesMap(users []model.User) map[string]string {
	out := map[string]string{}
	for _, u := range users {
		id := strings.TrimSpace(u.UserID)
		if id == "" {
			continue
		}
		out[id] = pickerDisplayName(u)
	}
	return out
}

func ensurePickerUsers(users *repository.UserRepo, list []model.User, extraIDs ...string) []model.User {
	seen := map[string]bool{}
	for _, u := range list {
		if id := strings.TrimSpace(u.UserID); id != "" {
			seen[id] = true
		}
	}
	if users == nil {
		return list
	}
	for _, raw := range extraIDs {
		id := strings.TrimSpace(raw)
		if id == "" || seen[id] {
			continue
		}
		u, err := users.GetByID(id)
		if err != nil || u == nil {
			continue
		}
		list = append(list, *u)
		seen[id] = true
	}
	return list
}

func pickerNamesFor(users *repository.UserRepo, list []model.User, extraIDs ...string) map[string]string {
	names := pickerNamesMap(list)
	if users == nil {
		return names
	}
	for _, raw := range extraIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if n := strings.TrimSpace(names[id]); n != "" && !strings.HasPrefix(n, "USR-") {
			continue
		}
		u, err := users.GetByID(id)
		if err != nil || u == nil {
			continue
		}
		names[id] = pickerDisplayName(*u)
	}
	return names
}

func taskOrgID(c echo.Context, t *model.WorkTask) string {
	if t != nil {
		if o := strings.TrimSpace(t.OrgID); o != "" {
			return o
		}
	}
	return assignableOrgID(c)
}

func denyOtherOrgTask(c echo.Context, t *model.WorkTask) error {
	if t == nil {
		return echo.ErrNotFound
	}
	org := currentOrg(c)
	if org == "" || org == repository.OrgAll {
		return nil
	}
	to := strings.TrimSpace(t.OrgID)
	if to == "" {
		return nil
	}
	if to != org {
		return echo.ErrNotFound
	}
	return nil
}

func checkAssigneeInOrg(users *repository.UserRepo, orgID, name, uid, prev string) error {
	name, uid, prev = strings.TrimSpace(name), strings.TrimSpace(uid), strings.TrimSpace(prev)
	if name == "" && uid == "" {
		return nil
	}
	if prev != "" && name == prev {
		return nil
	}
	orgID = strings.TrimSpace(orgID)
	if orgID == "" || orgID == repository.OrgAll {
		return errAssigneeOrg()
	}
	if users == nil {
		return nil
	}
	var u *model.User
	if uid != "" {
		u = users.FindAssignable(orgID, uid)
	}
	if u == nil && name != "" {
		u = users.FindAssignable(orgID, name)
	}
	if u == nil {
		return errAssigneeOrg()
	}
	uo := strings.TrimSpace(u.OrgID)
	if uo != orgID && !(orgID == model.OrgIDLibrary && uo == "") {
		return errAssigneeOrg()
	}
	return nil
}

func checkAssigneeNamesInOrg(users *repository.UserRepo, orgID string, names, allowedPrev []string) error {
	allowed := map[string]bool{}
	for _, p := range allowedPrev {
		if n := strings.TrimSpace(p); n != "" {
			allowed[n] = true
		}
	}
	for _, raw := range names {
		n := strings.TrimSpace(raw)
		if n == "" || allowed[n] {
			continue
		}
		if err := checkAssigneeInOrg(users, orgID, n, "", ""); err != nil {
			return err
		}
	}
	return nil
}

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
const assigneeUnspecified = "담당자 미지정"

func pickerPersonLabel(s string) string {
	n := strings.TrimSpace(s)
	if n == "" || n == "담당자" || strings.HasPrefix(n, "USR-") {
		return ""
	}
	return n
}

func pickerDisplayName(u model.User) string {
	if n := pickerPersonLabel(u.FullName); n != "" {
		return n
	}
	if n := pickerPersonLabel(u.Username); n != "" {
		return n
	}
	return pickerNameUnregistered
}

func resolvePersonLabel(users *repository.UserRepo, uid, name string) string {
	n := userDisplayName(users, uid, name)
	if n == assigneeUnspecified {
		return ""
	}
	return n
}

// userDisplayName 화면용 담당자 이름. USR- 번호는 쓰지 않는다.
// 우선순위: 사용자 ID 로 찾은 이름 → 저장된 담당자 이름 → 「담당자 미지정」.
func userDisplayName(users *repository.UserRepo, uid, storedName string) string {
	id := strings.TrimSpace(uid)
	if id == "" {
		raw := strings.TrimSpace(storedName)
		if strings.HasPrefix(raw, "USR-") {
			id = raw
		}
	}
	if users != nil && id != "" {
		if u, err := users.GetByID(id); err == nil && u != nil {
			n := pickerDisplayName(*u)
			if n != "" && n != pickerNameUnregistered {
				return n
			}
		}
	}
	if n := pickerPersonLabel(storedName); n != "" {
		return n
	}
	return assigneeUnspecified
}

func pickerIDList(owner any, ids any) []string {
	var out []string
	seen := map[string]bool{}
	add := func(raw string) {
		id := strings.TrimSpace(raw)
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		out = append(out, id)
	}
	if s, ok := owner.(string); ok {
		add(s)
	}
	switch v := ids.(type) {
	case []string:
		for _, id := range v {
			add(id)
		}
	}
	return out
}

func pickerButtonLabel(names any, owner any, ids any, external any) string {
	nm, _ := names.(map[string]string)
	for _, id := range pickerIDList(owner, ids) {
		if n := pickerPersonLabel(nm[id]); n != "" {
			return n
		}
	}
	if s, ok := external.(string); ok {
		if n := pickerPersonLabel(s); n != "" {
			return n
		}
	}
	return "— 선택 —"
}

func compactPickerIDs(vals ...string) []string {
	return pickerIDList("", vals)
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
	return ensurePickerUsersNamed(users, list, nil, extraIDs...)
}

func ensurePickerUsersNamed(users *repository.UserRepo, list []model.User, stored map[string]string, extraIDs ...string) []model.User {
	seen := map[string]bool{}
	for _, u := range list {
		if id := strings.TrimSpace(u.UserID); id != "" {
			seen[id] = true
		}
	}
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		if users != nil {
			if u, err := users.GetByID(id); err == nil && u != nil {
				if pickerPersonLabel(pickerDisplayName(*u)) == "" {
					u.FullName = userDisplayName(users, id, stored[id])
				}
				list = append(list, *u)
				return
			}
		}
		list = append(list, model.User{UserID: id, FullName: userDisplayName(users, id, stored[id]), IsActive: true})
	}
	for _, raw := range extraIDs {
		add(raw)
	}
	for id := range stored {
		add(id)
	}
	return list
}

func overlayPickerStoredNames(users *repository.UserRepo, names map[string]string, stored map[string]string) map[string]string {
	if names == nil {
		names = map[string]string{}
	}
	for id, raw := range stored {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		cur := names[id]
		if pickerPersonLabel(cur) != "" && cur != pickerNameUnregistered {
			continue
		}
		names[id] = userDisplayName(users, id, raw)
	}
	return names
}

func storedPickerNamesFromTask(t *model.WorkTask, members []model.WorkTaskMember) map[string]string {
	out := map[string]string{}
	if t != nil {
		if id := strings.TrimSpace(t.AssigneeUserID); id != "" {
			out[id] = t.Assignee
		}
	}
	for _, m := range members {
		if id := strings.TrimSpace(m.UserID); id != "" {
			out[id] = m.Assignee
		}
	}
	return out
}

func pickerOwnerID(users []model.User, as *model.ASReceipt) string {
	if as == nil {
		return ""
	}
	if id := strings.TrimSpace(as.AssignedUserID); id != "" {
		return id
	}
	want := strings.TrimSpace(as.AssignedTo)
	if want == "" {
		return ""
	}
	for _, u := range users {
		if strings.TrimSpace(u.FullName) == want || strings.TrimSpace(u.Username) == want {
			return strings.TrimSpace(u.UserID)
		}
	}
	return ""
}

func pickerSelectedIDs(users []model.User, as *model.ASReceipt) []string {
	if id := pickerOwnerID(users, as); id != "" {
		return []string{id}
	}
	return nil
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
		if n := pickerPersonLabel(names[id]); n != "" && n != pickerNameUnregistered {
			continue
		}
		names[id] = userDisplayName(users, id, names[id])
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

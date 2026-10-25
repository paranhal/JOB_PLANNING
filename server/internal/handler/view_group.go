package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

const (
	viewGroupCookie = "view_group"
	viewGroupWith   = "with"
	viewGroupMine   = "personal"
)

func viewGroupInclude(c echo.Context) bool {
	q := strings.TrimSpace(c.QueryParam("scope"))
	switch q {
	case viewGroupMine:
		return false
	case viewGroupWith, "group":
		return true
	}
	ck, err := c.Cookie(viewGroupCookie)
	if err == nil && ck != nil && strings.TrimSpace(ck.Value) == viewGroupMine {
		return false
	}
	return true
}

func persistViewGroupCookie(c echo.Context) {
	q := strings.TrimSpace(c.QueryParam("scope"))
	if q != viewGroupMine && q != viewGroupWith && q != "group" {
		return
	}
	val := viewGroupWith
	if q == viewGroupMine {
		val = viewGroupMine
	}
	ck := &http.Cookie{
		Name:     viewGroupCookie,
		Value:    val,
		Path:     "/",
		MaxAge:   86400 * 365,
		Expires:  time.Now().Add(365 * 24 * time.Hour),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
	c.SetCookie(ck)
}

func viewGroupScope(c echo.Context) string {
	if viewGroupInclude(c) {
		return viewGroupWith
	}
	return viewGroupMine
}

func attachViewGroup(c echo.Context, data map[string]interface{}) map[string]interface{} {
	if data == nil {
		data = map[string]interface{}{}
	}
	persistViewGroupCookie(c)
	inc := viewGroupInclude(c)
	data["ViewGroupWith"] = inc
	data["ViewGroupScope"] = viewGroupScope(c)
	data["ViewGroupToggle"] = true
	return data
}

func filterPersonalOnlyWork(items []model.WorkListItem, userID string, keys []string) []model.WorkListItem {
	keys = append([]string{userID}, keys...)
	want := map[string]bool{}
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k != "" {
			want[k] = true
		}
	}
	var out []model.WorkListItem
	for _, it := range items {
		if strings.TrimSpace(it.WorkGroupID) != "" {
			continue
		}
		if len(want) == 0 || want[strings.TrimSpace(it.Assignee)] {
			out = append(out, it)
		}
	}
	return out
}

func applyViewGroupToTasks(c echo.Context, all, filtered []model.WorkTask, scope boardScope) []model.WorkTask {
	include := viewGroupInclude(c)
	if !include {
		var out []model.WorkTask
		for _, t := range filtered {
			if strings.TrimSpace(t.WorkGroupID) == "" {
				out = append(out, t)
			}
		}
		return out
	}
	if scope.ScopeAll {
		return filtered
	}
	seen := map[string]bool{}
	var out []model.WorkTask
	for _, t := range filtered {
		seen[t.TaskID] = true
		out = append(out, t)
	}
	for _, t := range all {
		if seen[t.TaskID] {
			continue
		}
		if strings.TrimSpace(t.WorkGroupID) != "" && memberOfGroup(c, t.WorkGroupID) {
			out = append(out, t)
			seen[t.TaskID] = true
		}
	}
	return out
}

func injectGroupMaps(c echo.Context, dbGroups *repository.WorkGroupRepo) {
	uid := strings.TrimSpace(currentUserID(c))
	if uid == "" || dbGroups == nil {
		c.Set("member_group_ids", map[string]bool{})
		c.Set("leader_group_ids", map[string]bool{})
		return
	}
	mem, lead := dbGroups.ActiveRolesForUser(uid, "")
	c.Set("member_group_ids", mem)
	c.Set("leader_group_ids", lead)
}

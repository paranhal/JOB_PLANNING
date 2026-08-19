package handler

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

func taskIDsOf(tasks []model.WorkTask) []string {
	ids := make([]string, 0, len(tasks))
	for _, t := range tasks {
		if strings.TrimSpace(t.TaskID) != "" {
			ids = append(ids, t.TaskID)
		}
	}
	return ids
}

func parseSupportMembers(c echo.Context, owner string) []model.WorkTaskMember {
	owner = strings.TrimSpace(owner)
	_ = c.Request().ParseForm()
	names := c.Request().PostForm["member_name"]
	mins := c.Request().PostForm["member_min"]
	var out []model.WorkTaskMember
	seen := map[string]bool{owner: true}
	for i, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		m := model.WorkTaskMember{
			Assignee:  name,
			Role:      model.WBMemberSupport,
			SortOrder: i + 1,
		}
		if i < len(mins) {
			if n, err := strconv.Atoi(strings.TrimSpace(mins[i])); err == nil && n > 0 {
				m.DurationMin = n
			}
		}
		out = append(out, m)
	}
	return out
}

func occupancyNames(owner string, supports []model.WorkTaskMember) []string {
	seen := map[string]bool{}
	var out []string
	add := func(n string) {
		n = strings.TrimSpace(n)
		if n == "" || seen[n] {
			return
		}
		seen[n] = true
		out = append(out, n)
	}
	add(owner)
	for _, m := range supports {
		add(m.Assignee)
	}
	return out
}

func (h *WorkboardHandler) firstParticipantOverlap(date, start, end, excludeID string, names []string) string {
	if strings.TrimSpace(date) == "" || strings.TrimSpace(start) == "" {
		return ""
	}
	for _, name := range names {
		ok, err := h.repo.AssigneeTimeOverlaps(date, name, start, end, excludeID)
		if err != nil {
			continue
		}
		if ok {
			return assigneeOverlapMessage(name, start, end)
		}
	}
	return ""
}

func (h *WorkboardHandler) saveSupportMembers(c echo.Context, taskID, owner string) error {
	return h.repo.ReplaceSupportMembers(taskID, parseSupportMembers(c, owner))
}

func overlapRedirect(c echo.Context, msg string) error {
	return c.Redirect(http.StatusSeeOther, redirectBack(c, "err="+url.QueryEscape(msg)))
}

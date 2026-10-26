package handler

import (
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

const pwMaxSupporters = 20

func defaultActionStartLocal(t *model.WorkTask, now time.Time) string {
	if t == nil {
		return now.Format("2006-01-02T15:04")
	}
	if v := normalizeDatetimeLocal(t.ActionStartAt); v != "" {
		return v
	}
	day := strings.TrimSpace(t.WorkDate)
	tm := strings.TrimSpace(t.StartTime)
	if day != "" && tm != "" {
		if len(tm) >= 5 {
			tm = tm[:5]
		}
		return day + "T" + tm
	}
	if day != "" {
		return day + "T09:00"
	}
	return now.Format("2006-01-02T15:04")
}

func normalizeDatetimeLocal(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "T")
	if len(s) >= 16 {
		s = s[:16]
	}
	if _, err := time.Parse("2006-01-02T15:04", s); err != nil {
		return ""
	}
	return s
}

func parseActionStartAt(s string) (local, day string, code string) {
	local = normalizeDatetimeLocal(s)
	if local == "" {
		return "", "", "action_start"
	}
	return local, local[:10], ""
}

func parseActionDurationMin(c echo.Context) (int, string) {
	raw := strings.TrimSpace(c.FormValue("action_duration_min"))
	if raw == "custom" {
		raw = strings.TrimSpace(c.FormValue("action_duration_custom"))
	}
	if raw == "" {
		return 0, "action_duration"
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, "action_duration"
	}
	return n, ""
}

func resolveProcessWorkDate(startDay, formWorkDate, planned, completeDT string) string {
	if d := strings.TrimSpace(startDay); d != "" {
		return d
	}
	if d := strings.TrimSpace(formWorkDate); d != "" {
		return d
	}
	if d := strings.TrimSpace(planned); d != "" {
		return d
	}
	c := strings.TrimSpace(completeDT)
	if len(c) >= 10 {
		return c[:10]
	}
	return ""
}

func completeDatetimeBeforeStart(completeDT, startAt string) bool {
	c := normalizeDatetimeLocal(completeDT)
	s := normalizeDatetimeLocal(startAt)
	if c == "" || s == "" {
		return false
	}
	return c < s
}

func startDayDiffersPlan(startDay, planned string) bool {
	startDay = strings.TrimSpace(startDay)
	planned = strings.TrimSpace(planned)
	return startDay != "" && planned != "" && startDay != planned
}

func defaultActionDuration(t *model.WorkTask) int {
	if t != nil && t.ActionDurationMin > 0 {
		return t.ActionDurationMin
	}
	if t != nil && t.DurationMin > 0 {
		return t.DurationMin
	}
	return 30
}

func supportMembersOnly(members []model.WorkTaskMember) []model.WorkTaskMember {
	var out []model.WorkTaskMember
	for _, m := range members {
		if m.IsSupport() {
			out = append(out, m)
		}
	}
	return out
}

func collectProjectWorkSupports(c echo.Context, ownerUID, ownerName string, users []model.User) ([]model.WorkTaskMember, string) {
	_ = c.Request().ParseForm()
	form := c.Request().PostForm
	ids := form["supporter_user_id"]
	names := form["supporter_name"]
	n := len(ids)
	if len(names) > n {
		n = len(names)
	}
	seen := map[string]bool{}
	ownerUID = strings.TrimSpace(ownerUID)
	ownerName = strings.TrimSpace(ownerName)
	var out []model.WorkTaskMember
	for i := 0; i < n; i++ {
		uid, name := "", ""
		if i < len(ids) {
			uid = strings.TrimSpace(ids[i])
		}
		if i < len(names) {
			name = strings.TrimSpace(names[i])
		}
		if uid != "" && name == "" {
			for _, u := range users {
				if u.UserID == uid {
					name = u.FullName
					break
				}
			}
		}
		if uid == "" && name == "" {
			continue
		}
		if (ownerUID != "" && uid == ownerUID) || (ownerName != "" && name == ownerName && uid == "") {
			return nil, "supporter_owner"
		}
		key := uid
		if key == "" {
			key = "n:" + name
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, model.WorkTaskMember{UserID: uid, Assignee: name, Role: model.WBMemberSupport, SortOrder: len(out) + 1})
	}
	custom := strings.TrimSpace(form.Get("supporter_custom"))
	if custom == "" {
		custom = strings.Join(form["supporter_custom"], ",")
	}
	for _, part := range splitSupportNames(custom) {
		if (ownerName != "" && part == ownerName) || seen["n:"+part] {
			if ownerName != "" && part == ownerName {
				return nil, "supporter_owner"
			}
			continue
		}
		seen["n:"+part] = true
		out = append(out, model.WorkTaskMember{Assignee: part, Role: model.WBMemberSupport, SortOrder: len(out) + 1})
	}
	if len(out) > pwMaxSupporters {
		return nil, "supporters_max"
	}
	return out, ""
}

func splitSupportNames(s string) []string {
	s = strings.ReplaceAll(s, "\n", ",")
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func overlaySupportDraft(c echo.Context, users []model.User) {
	_ = c.Request().ParseForm()
	if _, ok := c.Request().PostForm["supporter_user_id"]; !ok && c.FormValue("supporter_custom") == "" && c.FormValue("supporter_name") == "" {
		if _, ok := c.Request().PostForm["supporter_name"]; !ok {
			return
		}
	}
	ms, _ := collectProjectWorkSupports(c, "", "", users)
	c.Set("pw_supports", ms)
}

func projectWorkCancelURL(c echo.Context, taskID, back string) string {
	q := strings.TrimSpace(c.QueryParam("back"))
	if q == "" {
		q = strings.TrimSpace(c.FormValue("back"))
	}
	if q != "" {
		return q
	}
	if strings.TrimSpace(taskID) != "" {
		return "/workboard/tasks/" + taskID
	}
	return back
}

package handler

import (
	"net/url"
	"sort"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

const meetingAssigneeAll = "all"

type meetingAssigneeGroup struct {
	Label      string
	Count      int
	Unassigned bool
	Items      []model.WorkListItem
}

// resolveMeetingAssignee 드롭다운 선택. 빈 값=팀전체.
// 쿼리 없으면 기술담당은 본인, 그 외는 팀전체. mine= 은 하위호환.
func resolveMeetingAssignee(c echo.Context, users []model.User) string {
	raw := strings.TrimSpace(c.QueryParam("assignee"))
	mine := strings.TrimSpace(c.QueryParam("mine"))
	self := strings.TrimSpace(ctxString(c, "user_name"))
	role := currentRole(c)

	if raw != "" {
		if raw == meetingAssigneeAll || raw == model.StatsTeamLabel {
			return ""
		}
		if isAssignableName(users, raw) {
			return raw
		}
		if self != "" && raw == self {
			return self
		}
		return ""
	}

	if mine == "0" {
		return ""
	}
	if mine == "1" && self != "" {
		return self
	}
	if role == model.RoleTech && self != "" {
		return self
	}
	return ""
}

func isAssignableName(users []model.User, name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, u := range users {
		if strings.TrimSpace(u.FullName) == name {
			return true
		}
	}
	return false
}

func assignableNameOrder(users []model.User) []string {
	var names []string
	seen := map[string]bool{}
	for _, u := range users {
		n := strings.TrimSpace(u.FullName)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		names = append(names, n)
	}
	sort.Strings(names) // §7.6.3 가나다. 등록 화면 열 순서와 같음
	return names
}

func meetingAssigneeQuery(selected string) string {
	return meetingNavQuery(selected, "")
}

func meetingNavQuery(selected, display string) string {
	v := url.Values{}
	if selected == "" {
		v.Set("assignee", meetingAssigneeAll)
	} else {
		v.Set("assignee", selected)
	}
	if display == "kanban" {
		v.Set("display", "kanban")
	}
	return v.Encode()
}

func meetingFilterQuery(date, selected, display string) string {
	v := url.Values{}
	if date != "" {
		v.Set("date", date)
	}
	if selected == "" {
		v.Set("assignee", meetingAssigneeAll)
	} else {
		v.Set("assignee", selected)
	}
	if display == "kanban" || display == "list" {
		v.Set("display", display)
	}
	return v.Encode()
}

func meetingAssigneeKeys(selected string, users []model.User) (userID string, keys []string) {
	selected = strings.TrimSpace(selected)
	if selected == "" {
		return "", nil
	}
	keys = []string{selected}
	for _, u := range users {
		if strings.TrimSpace(u.FullName) == selected {
			return strings.TrimSpace(u.UserID), keys
		}
	}
	return "", keys
}

func groupWorkItemsByAssignee(items []model.WorkListItem, order []string) []meetingAssigneeGroup {
	idx := map[string]int{}
	for i, n := range order {
		idx[n] = i
	}
	sorted := append([]model.WorkListItem(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool {
		ri, rj := assigneeSortRank(sorted[i].Assignee, idx), assigneeSortRank(sorted[j].Assignee, idx)
		if ri != rj {
			return ri < rj
		}
		ai, aj := strings.TrimSpace(sorted[i].Assignee), strings.TrimSpace(sorted[j].Assignee)
		if ai != aj {
			return ai < aj
		}
		if sorted[i].ScheduledDate != sorted[j].ScheduledDate {
			return sorted[i].ScheduledDate < sorted[j].ScheduledDate
		}
		pi, pj := workPrefixRank(sorted[i].Prefix), workPrefixRank(sorted[j].Prefix)
		if pi != pj {
			return pi < pj
		}
		return sorted[i].RefNumber < sorted[j].RefNumber
	})

	var groups []meetingAssigneeGroup
	for _, it := range sorted {
		label := strings.TrimSpace(it.Assignee)
		unassigned := label == ""
		if unassigned {
			label = "미배정"
		}
		if n := len(groups); n > 0 && groups[n-1].Label == label && groups[n-1].Unassigned == unassigned {
			groups[n-1].Items = append(groups[n-1].Items, it)
			groups[n-1].Count++
			continue
		}
		groups = append(groups, meetingAssigneeGroup{
			Label: label, Count: 1, Unassigned: unassigned,
			Items: []model.WorkListItem{it},
		})
	}
	return groups
}

func assigneeSortRank(name string, order map[string]int) int {
	name = strings.TrimSpace(name)
	if name == "" {
		return 1_000_000
	}
	if i, ok := order[name]; ok {
		return i
	}
	return 10_000
}

func workPrefixRank(p string) int {
	switch p {
	case model.WorkPrefixAS:
		return 0
	case model.WorkPrefixMaintenance:
		return 1
	case model.WorkPrefixConfirm:
		return 2
	default:
		return 3
	}
}

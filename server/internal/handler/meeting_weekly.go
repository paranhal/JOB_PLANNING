package handler

import (
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

// Weekly GET /meeting/weekly 팀 자료. 로그인한 전원(조직 범위). D-95
func (h *MeetingHandler) Weekly(c echo.Context) error {
	return h.renderWeekly(c, true)
}

// WeeklyMe GET /meeting/weekly/me 본인 자료. assignee/mine 쿼리는 무시. D-95
func (h *MeetingHandler) WeeklyMe(c echo.Context) error {
	return h.renderWeekly(c, false)
}

func (h *MeetingHandler) renderWeekly(c echo.Context, team bool) error {
	anchor := parseMeetingAnchor(c.QueryParam("date"))
	mon, fri := meetingWeekMonFri(anchor)
	prevMon := mon.AddDate(0, 0, -7)
	nextMon := mon.AddDate(0, 0, 7)
	lastFrom, lastTo := model.LastISOWeekRange(mon)

	var users []model.User
	if h.userRepo != nil {
		users, _ = h.userRepo.ListAssignable()
	}

	pagePath := "/meeting/weekly/me"
	title := "주간 업무 회의 — 내 자료"
	active := NavMeetingWeeklyMe
	selected := ""
	navQ := template.URL("")
	reports := ""
	var mineUID string
	var mineKeys []string
	if team {
		pagePath = "/meeting/weekly"
		title = "주간 업무 회의 — 팀 자료"
		active = NavMeetingWeekly
		selected = resolveMeetingAssignee(c, users)
		mineUID, mineKeys = meetingAssigneeKeys(selected, users)
		navQ = template.URL(meetingNavQuery(selected, ""))
		reports = "/stats/reports"
	} else {
		mineUID, mineKeys = meetingSelfKeys(c, users)
		reports = "/meeting/weekly/me.xlsx?date=" + mon.Format("2006-01-02")
		users = nil
	}

	lastDone, err := h.collectCompletedRange(lastFrom, lastTo, mineUID, mineKeys)
	if err != nil {
		return err
	}
	planned, err := h.collectScheduledRange(mon, fri, mineUID, mineKeys)
	if err != nil {
		return err
	}
	overdue, err := h.collectWeekOverdue(mon, mineUID, mineKeys)
	if err != nil {
		return err
	}
	progress := meetingWeekProgress(planned)
	unplannedRaw, _, err := h.work.ListUnplanned(mineUID, mineKeys, "")
	if err != nil {
		return err
	}
	var unplannedNeed []model.WorkListItem
	for _, u := range unplannedRaw {
		if !u.NeedPlanKind() {
			continue
		}
		it := u.WorkListItem
		it.NeedPlan = true
		unplannedNeed = append(unplannedNeed, it)
	}

	h.markWeeklyCanEdit(c, lastDone)
	h.markWeeklyCanEdit(c, planned)
	h.markWeeklyCanEdit(c, overdue)

	order := assignableNameOrder(users)
	if !team {
		selfName := strings.TrimSpace(ctxString(c, "user_name"))
		if strings.TrimSpace(ctxString(c, "view_as_user_id")) != "" {
			selfName = strings.TrimSpace(ctxString(c, "view_as_name"))
		}
		if selfName != "" {
			order = []string{selfName}
		}
	}

	data := map[string]interface{}{
		"Title":         title,
		"Active":        active,
		"Team":          team,
		"CanSeeTeam":    true,
		"PagePath":      pagePath,
		"WeekStart":     mon.Format("2006-01-02"),
		"WeekEnd":       fri.Format("2006-01-02"),
		"PrevWeek":      prevMon.Format("2006-01-02"),
		"NextWeek":      nextMon.Format("2006-01-02"),
		"Users":         users,
		"Assignee":      selected,
		"NavQ":          navQ,
		"ScopeNote":     weeklyScopeNote(team, selected),
		"DoneN":         len(lastDone),
		"ProgressN":     len(progress),
		"OverdueN":      len(overdue),
		"UnplannedN":    len(unplannedNeed),
		"UnplannedHref": planUnplannedURL(false, currentRole(c), ""),
		"ReportsHref":   reports,
		"ExcelLabel":    weeklyExcelLabel(team),
		"DoneGroups":    groupWorkItemsByAssignee(lastDone, order),
		"PlanGroups":    groupWorkItemsByAssignee(planned, order),
		"OverdueGroups": groupWorkItemsByAssignee(overdue, order),
		"CanWrite":      !isReadOnly(c) && !viewAsActive(c),
		"Role":          currentRole(c),
		"Notice":        strings.TrimSpace(c.QueryParam("notice")),
		"ShowLeave":     false,
		"ShowInbox":     false,
		"ShowCoop":      false,
		"LeaveRemain":   "—",
	}
	if !team {
		h.fillWeeklyPersonalCards(c, mon, fri, data)
	}
	return c.Render(http.StatusOK, "meeting/weekly.html", data)
}

func weeklyExcelLabel(team bool) string {
	if team {
		return "주간업무보고 엑셀"
	}
	return "내 주간 자료 엑셀"
}

func weeklyScopeNote(team bool, selected string) string {
	if !team {
		return "내 담당 업무"
	}
	return meetingScopeNote(selected)
}

func meetingSelfKeys(c echo.Context, users []model.User) (string, []string) {
	uid := strings.TrimSpace(identityUserID(c))
	name := strings.TrimSpace(ctxString(c, "user_name"))
	if strings.TrimSpace(ctxString(c, "view_as_user_id")) != "" {
		name = strings.TrimSpace(ctxString(c, "view_as_name"))
	}
	_, keys := meetingAssigneeKeys(name, users)
	if uid != "" {
		return uid, keys
	}
	return "", keys
}

func meetingWeekMonFri(anchor time.Time) (time.Time, time.Time) {
	mon := model.CalendarMonday(anchor)
	return mon, mon.AddDate(0, 0, 4)
}

func (h *MeetingHandler) collectCompletedRange(from, to, uid string, keys []string) ([]model.WorkListItem, error) {
	start, err := time.ParseInLocation("2006-01-02", from, time.Local)
	if err != nil {
		return nil, err
	}
	end, err := time.ParseInLocation("2006-01-02", to, time.Local)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []model.WorkListItem
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		items, err := h.work.ListCompletedOn(d.Format("2006-01-02"), uid, keys, 300)
		if err != nil {
			return nil, err
		}
		for _, it := range items {
			k := model.WorkListItemKey(it)
			if seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, it)
		}
	}
	return out, nil
}

func (h *MeetingHandler) collectScheduledRange(from, to time.Time, uid string, keys []string) ([]model.WorkListItem, error) {
	seen := map[string]bool{}
	var out []model.WorkListItem
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		items, err := h.work.ListScheduledOn(d.Format("2006-01-02"), uid, keys, 300)
		if err != nil {
			return nil, err
		}
		for _, it := range items {
			k := model.WorkListItemKey(it)
			if seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, it)
		}
	}
	return out, nil
}

func (h *MeetingHandler) collectWeekOverdue(weekMon time.Time, uid string, keys []string) ([]model.WorkListItem, error) {
	mon := weekMon.Format("2006-01-02")
	items, err := h.work.ListUnified(uid, keys, true, mon, mon)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []model.WorkListItem
	for _, it := range items {
		if meetingItemComplete(it) {
			continue
		}
		sched := model.WorkItemScheduledDate(it)
		if it.DaysOverdue < 1 && (sched == "" || sched >= mon) {
			continue
		}
		k := model.WorkListItemKey(it)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, it)
	}
	return out, nil
}

func meetingItemComplete(it model.WorkListItem) bool {
	m := model.MapASStatusToWB(it.Status)
	return m == model.WBTaskComplete
}

func meetingWeekProgress(planned []model.WorkListItem) []model.WorkListItem {
	var out []model.WorkListItem
	for _, it := range planned {
		if model.WBKanbanBucket(model.MapASStatusToWB(it.Status)) == model.WBTaskInProgress {
			out = append(out, it)
		}
	}
	return out
}

func meetingGroupCount(groups []meetingAssigneeGroup) int {
	n := 0
	for _, g := range groups {
		n += g.Count
	}
	return n
}

func meetingWeeklyHasPost(body string) bool {
	i := strings.Index(body, `data-weekly-tabs`)
	if i >= 0 {
		body = body[i:]
	}
	s := strings.ToLower(body)
	return strings.Contains(s, `method="post"`) || strings.Contains(s, `method='post'`)
}

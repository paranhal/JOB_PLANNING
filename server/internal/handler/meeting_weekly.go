package handler

import (
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

// Weekly GET /meeting/weekly?date=YYYY-MM-DD&assignee=  주 단위 조회 전용(D-77).
func (h *MeetingHandler) Weekly(c echo.Context) error {
	anchor := parseMeetingAnchor(c.QueryParam("date"))
	mon, fri := meetingWeekMonFri(anchor)
	prevMon := mon.AddDate(0, 0, -7)
	nextMon := mon.AddDate(0, 0, 7)
	lastFrom, lastTo := model.LastISOWeekRange(mon) // 직전 주 월~일 (이번 주 월요일 기준)

	var users []model.User
	if h.userRepo != nil {
		users, _ = h.userRepo.ListAssignable()
	}
	selected := resolveMeetingAssignee(c, users)
	order := assignableNameOrder(users)
	mineUID, mineKeys := meetingAssigneeKeys(selected, users)

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

	doneGroups := groupWorkItemsByAssignee(lastDone, order)
	planGroups := groupWorkItemsByAssignee(planned, order)
	overdueGroups := groupWorkItemsByAssignee(overdue, order)

	return c.Render(http.StatusOK, "meeting/weekly.html", map[string]interface{}{
		"Title":         "주간 업무 회의",
		"Active":        NavMeetingWeekly,
		"WeekStart":     mon.Format("2006-01-02"),
		"WeekEnd":       fri.Format("2006-01-02"),
		"PrevWeek":      prevMon.Format("2006-01-02"),
		"NextWeek":      nextMon.Format("2006-01-02"),
		"Users":         users,
		"Assignee":      selected,
		"NavQ":          template.URL(meetingNavQuery(selected, "")),
		"ScopeNote":     meetingScopeNote(selected),
		"DoneN":         len(lastDone),
		"ProgressN":     len(progress),
		"OverdueN":      len(overdue),
		"UnplannedN":    len(unplannedNeed),
		"UnplannedHref": planUnplannedURL(false, currentRole(c), ""),
		"ReportsHref":   "/stats/reports",
		"DoneGroups":    doneGroups,
		"PlanGroups":    planGroups,
		"OverdueGroups": overdueGroups,
		"CanWrite":      false,
		"Role":          currentRole(c),
	})
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
	s := strings.ToLower(body)
	return strings.Contains(s, `method="post"`) || strings.Contains(s, `method='post'`)
}

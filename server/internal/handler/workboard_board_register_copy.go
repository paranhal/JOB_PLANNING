package handler

import (
	"sort"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

// attachCopiedRegisterTimetable register 일 보기 일정표 데이터를 board에 채운다.
// register.html 은 바꾸지 않고, 마크업만 board에 복사한다(95-132).
func (h *WorkboardHandler) attachCopiedRegisterTimetable(c echo.Context, data map[string]interface{}, dateStr, assigneeFilter, org string, dayItems []boardItem) error {
	if data == nil {
		return nil
	}
	today := time.Now().Format(dateLayout)
	base, err := time.ParseInLocation(dateLayout, dateStr, time.Local)
	if err != nil {
		base = time.Now()
	}
	base = time.Date(base.Year(), base.Month(), base.Day(), 0, 0, 0, 0, time.Local)
	period := buildRegisterPeriod(regViewDay, base, today)

	placed := ensureTimelineDisplayTimes(registerTasksFromBoardItems(dayItems, dateStr))
	h.attachProjectWorkSites(placed)

	memberMap := map[string][]model.WorkTaskMember{}
	if ids := taskIDsOf(placed); len(ids) > 0 {
		if m, err := h.repo.ListMembersByTaskIDs(ids); err == nil {
			memberMap = m
		}
	}
	placed = filterTasksByParticipants(placed, memberMap, assigneeFilter)

	assignees := listAssignableForPicker(h.userRepo, c)
	slotTimes := registerSlotTimes()
	var order []string
	for _, u := range assignees {
		order = append(order, u.FullName)
	}
	sort.Strings(order)
	dateCol := RegisterColumn{Date: dateStr, From: dateStr, To: dateStr}
	if len(period.Columns) > 0 {
		dateCol = period.Columns[0]
	}
	dayCols := buildRegisterAssigneeColumnsMembers(dateCol, placed, h.taskCard, order, assigneeFilter, memberMap)
	holi := loadHolidays(h.mntRepo.Holidays(), period.From, period.To)
	var leaveItems []model.StaffLeave
	if h.mntRepo != nil {
		if lr := h.mntRepo.Leaves(); lr != nil {
			leaveItems, _ = lr.ListInRange(period.From, period.To)
		}
	}
	leaveMap := leaveBadgesByDate(leaveItems, currentUserID(c))
	decorateRegisterDayColumns(dayCols, today, holi)
	var dayLeaveNames []string
	leaveHit := map[string]bool{}
	for _, b := range leaveMap[dateStr] {
		if b.Name != "" && !leaveHit[b.Name] {
			leaveHit[b.Name] = true
			dayLeaveNames = append(dayLeaveNames, b.Name)
		}
	}
	regNames := map[string]bool{}
	for _, u := range assignees {
		regNames[strings.TrimSpace(u.FullName)] = true
	}
	for i := range dayCols {
		n := strings.TrimSpace(dayCols[i].Assignee)
		if leaveHit[n] {
			dayCols[i].OnLeave = true
		}
		if n != "" && !dayCols[i].Unassigned && !regNames[n] {
			dayCols[i].Unregistered = true
		}
	}
	gridHeightStyle, slotTopStyles := registerGridStyles(len(slotTimes))
	undatedTasks, _ := h.repo.ListUndatedASTasksForOrg(org)
	undatedTasks = filterTasksByAssignee(undatedTasks, assigneeFilter)
	undatedCards := undatedASCards(undatedTasks)
	redir, _ := data["ScheduleRedirect"].(string)
	data["DayColumns"] = dayCols
	data["SlotTimes"] = slotTimes
	data["SlotRem"] = registerSlotRem
	data["GridHeightStyle"] = gridHeightStyle
	data["SlotTopStyles"] = slotTopStyles
	data["UndatedCards"] = undatedCards
	data["ExtraAssignees"] = extraAssigneesForDay(regViewDay, assigneeFilter, assignees, dayCols)
	data["DayLeaveNames"] = dayLeaveNames
	data["AssigneeQ"] = ""
	data["CopiedRegisterRedirect"] = redir
	data["RegisterCopy"] = true
	return nil
}

func registerTasksFromBoardItems(items []boardItem, day string) []model.WorkTask {
	day = strings.TrimSpace(day)
	var out []model.WorkTask
	seen := map[string]bool{}
	for _, it := range items {
		t := it.Task
		if t.Status == model.WBTaskCancelled {
			continue
		}
		place := strings.TrimSpace(it.PlaceDate)
		if place == "" {
			place = model.TaskPlanDate(t)
		}
		if place != day && !model.TaskSpansDate(t.WorkDate, t.DueDate, day) {
			continue
		}
		t.WorkDate = day
		if strings.TrimSpace(t.StartTime) == "" {
			t.StartTime = "09:00"
		}
		if strings.TrimSpace(t.EndTime) == "" {
			t.EndTime = "09:30"
		}
		k := strings.TrimSpace(t.TaskID)
		if k == "" {
			k = it.DedupKey
		}
		if boardItemIsExec(it) {
			k += "|act|" + day
		}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, t)
	}
	return out
}

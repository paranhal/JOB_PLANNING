package handler

import (
	"fmt"
	"html/template"
	"sort"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

type boardPersonCol struct {
	Assignee     string
	Label        string
	Sub          string
	Unassigned   bool
	OnLeave      bool
	Unregistered bool
	Count        int
	CellStyle    template.CSS
	Items        []boardItem
}

func parseBoardCols(raw string) string {
	if strings.TrimSpace(raw) == boardColsFlat {
		return boardColsFlat
	}
	return boardColsPerson
}

func boardItemDone(it boardItem) bool {
	switch strings.TrimSpace(it.Task.Status) {
	case model.WBTaskComplete, "completed", "closed", "done":
		return true
	default:
		return false
	}
}

func boardActionHref(t model.WorkTask) string {
	return (*WorkboardHandler)(nil).taskActionTarget(nil, &t).Href
}

func boardBarOpenHref(it boardItem) string {
	if it.Done && strings.TrimSpace(it.ActionHref) != "" {
		return it.ActionHref
	}
	if href := strings.TrimSpace(it.Href); href != "" {
		return href
	}
	return it.ActionHref
}

func boardPlaceDur(it boardItem) int {
	if it.PlaceDur > 0 {
		return it.PlaceDur
	}
	if it.HasTime && it.EndMin > it.StartMin {
		return it.EndMin - it.StartMin
	}
	if sm, okS := parseClockMin(it.StartHHMM); okS {
		if em, okE := parseClockMin(it.EndHHMM); okE && em > sm {
			return em - sm
		}
	}
	if it.Task.DurationMin > 0 {
		return it.Task.DurationMin
	}
	if d := model.DurationFromTimes(it.Task.StartTime, it.Task.EndTime); d > 0 {
		return d
	}
	return 30
}

func boardBarMinutes(it boardItem) int {
	return boardPlaceDur(it)
}

const (
	boardDayBarMinShort = 64
	boardDayBarMinLong  = 96
)

func boardDayBarHeight(startMin, endMin, dur int) (top, hgt int) {
	h0 := boardDayStartHour * 60
	h1 := (boardDayEndHour + 1) * 60
	s, e := startMin, endMin
	if s < h0 {
		s = h0
	}
	if e > h1 {
		e = h1
	}
	top = (s-h0)*boardDayPxPerHour/60 + 2
	hgt = (e-s)*boardDayPxPerHour/60 - 4
	minH := boardDayBarMinShort
	if dur > 30 {
		minH = boardDayBarMinLong
	}
	if hgt < minH {
		hgt = minH
	}
	return top, hgt
}

func formatHHMM(min int) string {
	if min < 0 {
		min = 0
	}
	return fmt.Sprintf("%02d:%02d", min/60, min%60)
}

func positionBoardBar(it *boardItem) {
	if it == nil {
		return
	}
	it.DayPosTop, it.DayPosHeight = boardDayBarHeight(it.StartMin, it.EndMin, boardPlaceDur(*it))
}

func (h *WorkboardHandler) stampBoardMove(c echo.Context, items []boardItem, write bool, back string) {
	for i := range items {
		it := &items[i]
		if strings.TrimSpace(it.PlaceKind) == "" && strings.TrimSpace(it.Task.TaskID) != "" {
			it.PlaceKind = "task"
			it.PlaceRef = it.Task.TaskID
		}
		if strings.TrimSpace(it.PlaceAssignee) == "" {
			it.PlaceAssignee = userDisplayName(h.userRepo, it.Task.AssigneeUserID, it.Task.Assignee)
			if it.PlaceAssignee == assigneeUnspecified {
				it.PlaceAssignee = ""
			}
		}
		it.PlaceDur = boardPlaceDur(*it)
		if it.HasTime {
			it.StartHHMM = formatHHMM(it.StartMin)
			if it.EndMin > it.StartMin {
				it.EndHHMM = formatHHMM(it.EndMin)
			}
		}
		if it.Task.TaskID != "" {
			it.EditHref = "/workboard/tasks/" + it.Task.TaskID + "/edit"
		}
		tgt := h.taskActionTarget(c, &it.Task)
		it.ActionLabel = tgt.Label
		it.CanAction = tgt.Can
		if tgt.Href != "" {
			it.ActionHref = hrefWithBack(tgt.Href, back)
		} else {
			it.ActionHref = ""
		}
		it.Done = boardItemDone(*it)
		can := write && !it.Done && it.Task.TaskID != "" && canEditTask(c, it.Task.Assignee, it.Task.AssigneeUserID, it.Task.WorkGroupID)
		it.CanMove = can
		it.CanUnschedule = can
		// 구간이 2개 이상이면 PlaceTask가 업무 전체 시각을 한 조각으로 덮어 다른 구간을 깨므로 손잡이를 숨긴다.
		it.CanResize = can && it.HasTime && it.SegCount <= 1 && !it.Fragment
	}
}

func boardPersonInstances(it boardItem) []boardItem {
	mems := it.Members
	if len(mems) == 0 {
		positionBoardBar(&it)
		return []boardItem{it}
	}
	extra := len(mems) - 1
	if extra < 0 {
		extra = 0
	}
	out := make([]boardItem, 0, len(mems))
	for _, m := range mems {
		clone := it
		name := strings.TrimSpace(m.Assignee)
		clone.PlaceAssignee = name
		clone.IsSupport = m.IsSupport()
		clone.ExtraCount = extra
		if clone.IsSupport {
			clone.CanMove = false
			clone.CanResize = false
			clone.CanUnschedule = false
		}
		positionBoardBar(&clone)
		out = append(out, clone)
	}
	return out
}

func markBoardOverlap(items []boardItem) {
	for i := range items {
		for j := i + 1; j < len(items); j++ {
			if items[i].StartMin < items[j].EndMin && items[j].StartMin < items[i].EndMin {
				items[i].OverlapWarn = true
				items[j].OverlapWarn = true
			}
		}
	}
}

func buildBoardPersonCols(items []boardItem, order []string, filter string, leave map[string]bool, registered map[string]bool) []boardPersonCol {
	filter = strings.TrimSpace(filter)
	byName := map[string][]boardItem{}
	var unassigned []boardItem
	seen := map[string]bool{}
	for _, it := range items {
		for _, inst := range boardPersonInstances(it) {
			name := strings.TrimSpace(inst.PlaceAssignee)
			if name == "" {
				unassigned = append(unassigned, inst)
				continue
			}
			seen[name] = true
			byName[name] = append(byName[name], inst)
		}
	}
	var names []string
	if filter != "" {
		names = []string{filter}
	} else {
		used := map[string]bool{}
		for _, n := range order {
			n = strings.TrimSpace(n)
			if n == "" || used[n] || !seen[n] {
				continue
			}
			used[n] = true
			names = append(names, n)
		}
		var extra []string
		for n := range seen {
			if !used[n] {
				extra = append(extra, n)
			}
		}
		sort.Strings(extra)
		names = append(names, extra...)
	}
	out := make([]boardPersonCol, 0, len(names)+1)
	for _, name := range names {
		list := byName[name]
		markBoardOverlap(list)
		dur := 0
		for _, it := range list {
			dur += boardPlaceDur(it)
		}
		border, soft, _ := model.AssigneeColor(name)
		out = append(out, boardPersonCol{
			Assignee:     name,
			Label:        name,
			Sub:          assigneeColumnSub(len(list), dur),
			Count:        len(list),
			OnLeave:      leave[name],
			Unregistered: registered != nil && !registered[name],
			CellStyle:    template.CSS(fmt.Sprintf("background-color:%s;border-color:%s", soft, border)),
			Items:        list,
		})
	}
	if filter == "" && len(unassigned) > 0 {
		markBoardOverlap(unassigned)
		dur := 0
		for _, it := range unassigned {
			dur += boardPlaceDur(it)
		}
		out = append(out, boardPersonCol{
			Assignee:   "",
			Label:      registerUnassignedLabel,
			Sub:        assigneeColumnSub(len(unassigned), dur),
			Unassigned: true,
			Count:      len(unassigned),
			Items:      unassigned,
		})
	}
	return out
}

func extraAssigneesForBoard(users []model.User, cols []boardPersonCol) []string {
	used := map[string]bool{}
	for _, c := range cols {
		if n := strings.TrimSpace(c.Assignee); n != "" {
			used[n] = true
		}
	}
	var out []string
	for _, u := range users {
		n := strings.TrimSpace(u.FullName)
		if n == "" || used[n] {
			continue
		}
		out = append(out, n)
	}
	return out
}

func boardScheduleErrMsg(raw string) string {
	switch strings.TrimSpace(raw) {
	case "past":
		return "지난 날짜는 수정할 수 없습니다. 관리자는 잠금 해제 후 이용하세요."
	case "place":
		return "배치할 업무와 시간을 선택해 주세요."
	case "date_order":
		return "종료일은 시작일 이후여야 합니다"
	case "seg_date":
		return "구간 날짜를 바꿀 수 없습니다."
	case "done":
		return "완료된 업무는 시간을 바꿀 수 없습니다."
	default:
		return ""
	}
}

func boardPlanSavedMsg(c echo.Context) string {
	ok := strings.TrimSpace(c.QueryParam("ok"))
	if ok != "plan_saved" && ok != "task" {
		return ""
	}
	placed, _ := strconv.Atoi(strings.TrimSpace(c.QueryParam("placed")))
	split, _ := strconv.Atoi(strings.TrimSpace(c.QueryParam("split")))
	skip, _ := strconv.Atoi(strings.TrimSpace(c.QueryParam("skip")))
	if placed == 0 && split == 0 && skip == 0 {
		if ok == "task" {
			return ""
		}
		placed = 1
	}
	return fmt.Sprintf("%d일 등록, 끼워넣기 %d건, 건너뜀 %d건", placed, split, skip)
}

func boardDayLeaveNames(h *WorkboardHandler, c echo.Context, date string) []string {
	if h == nil || h.mntRepo == nil || date == "" {
		return nil
	}
	lr := h.mntRepo.Leaves()
	if lr == nil {
		return nil
	}
	items, _ := lr.ListInRange(date, date)
	leaveMap := leaveBadgesByDate(items, currentUserID(c))
	var names []string
	hit := map[string]bool{}
	for _, b := range leaveMap[date] {
		if b.Name != "" && !hit[b.Name] {
			hit[b.Name] = true
			names = append(names, b.Name)
		}
	}
	return names
}

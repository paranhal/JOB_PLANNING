package handler

import (
	"fmt"
	"html/template"
	"sort"
	"strings"

	"customer-support/internal/model"
)

const registerAssigneeColMinPx = 160

const registerUnassignedLabel = "미배정"

type assigneeColItem struct {
	task   model.WorkTask
	role   string
	extra  int
	durMin int
}

// buildRegisterAssigneeColumns 일일 보기 전용. 가로축을 담당자 열로 둔다. §7.6.2~7.6.4
// dateCol 은 그날 1열(날짜). order 는 상단 담당자 칩과 같은 이름 순서.
// filter 가 있으면 그 사람 1열만. 기존 buildRegisterDayColumns 는 바꾸지 않는다.
func buildRegisterAssigneeColumns(dateCol RegisterColumn, placed []model.WorkTask, toCard func(model.WorkTask) model.WBCard, order []string, filter string) []RegisterDayColumn {
	return buildRegisterAssigneeColumnsMembers(dateCol, placed, toCard, order, filter, nil)
}

func buildRegisterAssigneeColumnsMembers(dateCol RegisterColumn, placed []model.WorkTask, toCard func(model.WorkTask) model.WBCard, order []string, filter string, members map[string][]model.WorkTaskMember) []RegisterDayColumn {
	return buildRegisterAssigneeColumnsAlways(dateCol, placed, toCard, order, filter, members, nil)
}

func buildRegisterAssigneeColumnsAlways(dateCol RegisterColumn, placed []model.WorkTask, toCard func(model.WorkTask) model.WBCard, order []string, filter string, members map[string][]model.WorkTaskMember, always []string) []RegisterDayColumn {
	filter = strings.TrimSpace(filter)
	alwaysSet := map[string]bool{}
	for _, n := range always {
		n = strings.TrimSpace(n)
		if n != "" {
			alwaysSet[n] = true
		}
	}
	date := strings.TrimSpace(dateCol.Date)
	if date == "" {
		date = dateCol.From
	}
	byName := map[string][]assigneeColItem{}
	var unassigned []assigneeColItem
	seen := map[string]bool{}
	for _, t := range placed {
		if strings.TrimSpace(t.WorkDate) != date {
			continue
		}
		if strings.TrimSpace(t.StartTime) == "" {
			continue
		}
		items := expandTaskForAssigneeColumns(t, members)
		for _, it := range items {
			name := strings.TrimSpace(columnPerson(it))
			if name == "" {
				unassigned = append(unassigned, it)
				continue
			}
			seen[name] = true
			byName[name] = append(byName[name], it)
		}
	}

	var names []string
	if filter != "" {
		names = []string{filter}
	} else {
		used := map[string]bool{}
		for _, n := range order {
			n = strings.TrimSpace(n)
			if n == "" || used[n] {
				continue
			}
			if !seen[n] && !alwaysSet[n] {
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
		for n := range alwaysSet {
			if !used[n] {
				extra = append(extra, n)
			}
		}
		sort.Strings(extra)
		names = append(names, extra...)
	}

	out := make([]RegisterDayColumn, 0, len(names)+1)
	for _, name := range names {
		out = append(out, makeAssigneeColumn(dateCol, name, false, byName[name], toCard))
	}
	if filter == "" && len(unassigned) > 0 {
		out = append(out, makeAssigneeColumn(dateCol, registerUnassignedLabel, true, unassigned, toCard))
	}
	return out
}

func expandTaskForAssigneeColumns(t model.WorkTask, members map[string][]model.WorkTaskMember) []assigneeColItem {
	ms := members[t.TaskID]
	if len(ms) == 0 {
		dur := assigneeTaskDuration(t)
		return []assigneeColItem{{task: t, role: model.WBMemberOwner, extra: 0, durMin: dur}}
	}
	n := len(ms)
	extra := n - 1
	if extra < 0 {
		extra = 0
	}
	out := make([]assigneeColItem, 0, n)
	for _, m := range ms {
		dur := m.DurationMin
		if dur <= 0 {
			dur = assigneeTaskDuration(t)
		}
		role := m.Role
		if role == "" {
			role = model.WBMemberOwner
		}
		it := assigneeColItem{task: t, role: role, extra: extra, durMin: dur}
		it.task.Assignee = strings.TrimSpace(m.Assignee)
		out = append(out, it)
	}
	return out
}

func columnPerson(it assigneeColItem) string {
	return strings.TrimSpace(it.task.Assignee)
}

func makeAssigneeColumn(dateCol RegisterColumn, label string, unassigned bool, items []assigneeColItem, toCard func(model.WorkTask) model.WBCard) RegisterDayColumn {
	col := dateCol
	col.Label = label
	dur := 0
	for _, it := range items {
		dur += it.durMin
	}
	col.Sub = assigneeColumnSub(len(items), dur)
	day := RegisterDayColumn{
		RegisterColumn: col,
		Assignee:       "",
		Unassigned:     unassigned,
		Count:          len(items),
		DurationMin:    dur,
		DurationLabel:  model.FormatStatsDuration(dur),
	}
	if !unassigned {
		day.Assignee = label
	}
	day.Blocks = layoutAssigneeBlocksItems(items, toCard)
	return day
}

func assigneeColumnSub(n, durMin int) string {
	return fmt.Sprintf("%d건 · %s", n, model.FormatStatsDuration(durMin))
}

func assigneeTaskDuration(t model.WorkTask) int {
	if t.DurationMin > 0 {
		return t.DurationMin
	}
	sm, ok := parseHHMMToMin(t.StartTime)
	if !ok {
		return 0
	}
	em := eventEndMin(sm, t.DurationMin, t.EndTime)
	if em > sm {
		return em - sm
	}
	return 0
}

func layoutAssigneeBlocksItems(items []assigneeColItem, toCard func(model.WorkTask) model.WBCard) []RegisterBlock {
	dayStart := workdayStartHour * 60
	dayEnd := workdayEndHour*60 + 15
	var evs []layoutEvent
	for _, it := range items {
		t := it.task
		sm, ok := parseHHMMToMin(t.StartTime)
		if !ok {
			continue
		}
		em := eventEndMin(sm, t.DurationMin, t.EndTime)
		if em <= sm {
			em = sm + 15
		}
		if em <= dayStart || sm >= dayEnd {
			continue
		}
		if sm < dayStart {
			sm = dayStart
		}
		if em > dayEnd {
			em = dayEnd
		}
		card := toCard(t)
		evs = append(evs, layoutEvent{
			card: card, startMin: sm, endMin: em,
			assignee: strings.TrimSpace(card.Assignee),
			lane:     -1,
			support:  it.role == model.WBMemberSupport,
			extra:    it.extra,
		})
	}
	assignAssigneeLanes(evs)
	blocks := make([]RegisterBlock, 0, len(evs))
	for _, e := range evs {
		border, soft, text := model.AssigneeColor(e.assignee)
		topSlots := (e.startMin - dayStart) / 15
		if topSlots < 0 {
			topSlots = 0
		}
		span := (e.endMin - e.startMin + 14) / 15
		if span < 1 {
			span = 1
		}
		lanes := e.lanes
		if lanes < 1 {
			lanes = 1
		}
		lane := e.lane
		if lane < 0 {
			lane = 0
		}
		laneWidth := 100.0 / float64(lanes)
		left := laneWidth * float64(lane)
		gapPct := 0.8
		w := laneWidth - gapPct
		if w < 8 {
			w = laneWidth
			gapPct = 0
		}
		l := left + gapPct/2
		topRem := float64(topSlots) * registerSlotRem
		heightRem := float64(span) * registerSlotRem
		z := 5
		if e.support {
			z = 4
		}
		style := fmt.Sprintf(
			"position:absolute;top:%.3frem;height:%.3frem;left:%.3f%%;width:%.3f%%;border-left:3px solid %s;background-color:%s;color:%s;box-sizing:border-box;z-index:%d",
			topRem, heightRem, l, w, border, soft, text, z,
		)
		blocks = append(blocks, RegisterBlock{
			Card: e.card, Style: template.CSS(style),
			BorderColor: border, SoftBg: soft, TextColor: text,
			Lane: lane, Lanes: lanes,
			TopRem: topRem, HeightRem: heightRem, LeftPct: l, WidthPct: w,
			OverlapWarn: lanes > 1,
			IsSupport:   e.support,
			ExtraCount:  e.extra,
		})
	}
	return blocks
}

func extraAssigneesForDay(view, filter string, users []model.User, cols []RegisterDayColumn) []string {
	if view != regViewDay || strings.TrimSpace(filter) != "" {
		return nil
	}
	return extraAssigneeOptions(users, cols)
}

func extraAssigneeOptions(users []model.User, cols []RegisterDayColumn) []string {
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

func assigneeOverlapMessage(name, start, end string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = registerUnassignedLabel
	}
	start = trimHHMM(start)
	end = trimHHMM(end)
	return fmt.Sprintf("%s의 %s~%s에 다른 업무가 있습니다", name, start, end)
}

func trimHHMM(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 5 {
		return s[:5]
	}
	return s
}

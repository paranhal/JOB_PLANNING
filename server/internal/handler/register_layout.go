package handler

import (
	"fmt"
	"html/template"
	"sort"
	"strings"
	"time"

	"customer-support/internal/model"
)

const registerSlotRem = 1.55 // 15분 칸 높이(rem) — 카드 높이는 소요시간과 연동

// registerCardMaxWidthPct 일 열 안에서 카드 최대 가로(%). 한 화면에 약 5개가 나란히 보이도록.
const registerCardMaxWidthPct = 20.0

type displaySpan struct{ s, e int }

// ensureTimelineDisplayTimes 시간표 표시용으로 WorkDate·StartTime을 채운다(DB는 변경하지 않음).
// WorkDate가 비면 DueDate를 쓰고, 시각이 없으면 같은 날 기존 일정과 겹치지 않게 순차 배정한다.
func ensureTimelineDisplayTimes(tasks []model.WorkTask) []model.WorkTask {
	out := make([]model.WorkTask, len(tasks))
	copy(out, tasks)

	occupied := map[string][]displaySpan{}
	for i := range out {
		d := strings.TrimSpace(out[i].WorkDate)
		if d == "" {
			d = strings.TrimSpace(out[i].DueDate)
		}
		if d == "" {
			continue
		}
		out[i].WorkDate = d
		if strings.TrimSpace(out[i].StartTime) == "" {
			continue
		}
		sm, ok := parseHHMMToMin(out[i].StartTime)
		if !ok {
			continue
		}
		em := eventEndMin(sm, out[i].DurationMin, out[i].EndTime)
		if em <= sm {
			em = sm + 30
		}
		occupied[d] = append(occupied[d], displaySpan{sm, em})
		if strings.TrimSpace(out[i].EndTime) == "" {
			out[i].EndTime = fmt.Sprintf("%02d:%02d", em/60, em%60)
		}
	}

	dayStart := workdayStartHour * 60
	dayEnd := workdayEndHour * 60
	for i := range out {
		d := strings.TrimSpace(out[i].WorkDate)
		if d == "" || strings.TrimSpace(out[i].StartTime) != "" {
			continue
		}
		dur := out[i].DurationMin
		if dur <= 0 {
			dur = 30
		}
		start := nextFreeDisplaySlot(occupied[d], dayStart, dayEnd, dur)
		end := start + dur
		if end > dayEnd {
			end = dayEnd
		}
		out[i].StartTime = fmt.Sprintf("%02d:%02d", start/60, start%60)
		out[i].EndTime = fmt.Sprintf("%02d:%02d", end/60, end%60)
		out[i].DurationMin = dur
		occupied[d] = append(occupied[d], displaySpan{start, end})
	}
	return out
}

func nextFreeDisplaySlot(busy []displaySpan, dayStart, dayEnd, dur int) int {
	if dur <= 0 {
		dur = 30
	}
	cursor := 9 * 60
	if cursor < dayStart {
		cursor = dayStart
	}
	for cursor+dur <= dayEnd {
		overlap := false
		for _, b := range busy {
			if cursor < b.e && cursor+dur > b.s {
				overlap = true
				if b.e > cursor {
					cursor = b.e
				}
				break
			}
		}
		if !overlap {
			return cursor
		}
	}
	return cursor
}

// RegisterBlock 시간표에 절대 배치되는 업무 카드
type RegisterBlock struct {
	Card        model.WBCard
	Style       template.CSS // html/template이 calc/색을 지우지 않도록 CSS 타입
	BorderColor string
	SoftBg      string
	TextColor   string
	Lane        int
	Lanes       int
	TopRem      float64
	HeightRem   float64
	LeftPct     float64
	WidthPct    float64
	OverlapWarn bool // 같은 담당자 시간 겹침. §7.6.4
	IsSupport   bool // 지원 카드. §7.7.4
	ExtraCount  int  // 나 제외 참여 인원. 카드 하단 「외 n명」
}

// RegisterDayColumn 요일(또는 일) 열 + 배치 블록
type RegisterDayColumn struct {
	RegisterColumn
	Blocks        []RegisterBlock
	Assignee      string // 일일 담당자 열. 비면 날짜 열(주간)
	Unassigned    bool
	Count         int
	DurationMin   int
	DurationLabel string
	OnLeave       bool
}

// AssigneeLegend 상단 범례용
type AssigneeLegend struct {
	Name   string
	Border string
	Soft   string
}

// RegisterMonthDay 월간 캘린더 하루 칸
type RegisterMonthDay struct {
	Date        string
	DayNum      int
	InMonth     bool
	IsToday     bool
	IsWeekend   bool
	IsSunday    bool
	IsSaturday  bool
	HolidayName string
	HolidayKind string
	DateTitle   string
	DayClass    string
	CellClass   string
	DayStyle    string
	CellStyle   string
	Leaves      LeaveBadgeGroup
	Cards       []model.WBCard
	Visible     []model.WBCard
	MoreCount   int
	Unassigned  int
	Total       int
}

const registerMonthVisibleMax = 3

// buildRegisterMonthWeeks 일요일 시작 월간 캘린더(피그마형).
func buildRegisterMonthWeeks(anchor time.Time, today string, placed []model.WorkTask, toCard func(model.WorkTask) model.WBCard) [][]RegisterMonthDay {
	loc := anchor.Location()
	first := time.Date(anchor.Year(), anchor.Month(), 1, 0, 0, 0, 0, loc)
	last := first.AddDate(0, 1, -1)
	start := first.AddDate(0, 0, -int(first.Weekday())) // 일요일
	end := last.AddDate(0, 0, (6-int(last.Weekday())+7)%7)
	byDate := map[string][]model.WorkTask{}
	for _, t := range placed {
		d := strings.TrimSpace(t.WorkDate)
		if d == "" {
			continue
		}
		byDate[d] = append(byDate[d], t)
	}
	var weeks [][]RegisterMonthDay
	var week []RegisterMonthDay
	for cursor := start; !cursor.After(end); cursor = cursor.AddDate(0, 0, 1) {
		ds := cursor.Format(dateLayout)
		inMonth := !cursor.Before(first) && !cursor.After(last)
		day := RegisterMonthDay{
			Date: ds, DayNum: cursor.Day(), InMonth: inMonth,
			IsToday:   ds == today,
			IsWeekend: cursor.Weekday() == time.Sunday || cursor.Weekday() == time.Saturday,
		}
		if inMonth {
			tasks := append([]model.WorkTask(nil), byDate[ds]...)
			sort.SliceStable(tasks, func(i, j int) bool {
				if tasks[i].StartTime != tasks[j].StartTime {
					return tasks[i].StartTime < tasks[j].StartTime
				}
				return tasks[i].Title < tasks[j].Title
			})
			for _, t := range tasks {
				card := toCard(t)
				day.Cards = append(day.Cards, card)
				if strings.TrimSpace(card.Assignee) == "" {
					day.Unassigned++
				}
			}
			day.Total = len(day.Cards)
			if day.Total > registerMonthVisibleMax {
				day.Visible = day.Cards[:registerMonthVisibleMax]
				day.MoreCount = day.Total - registerMonthVisibleMax
			} else {
				day.Visible = day.Cards
			}
		}
		week = append(week, day)
		if len(week) == 7 {
			weeks = append(weeks, week)
			week = nil
		}
	}
	return weeks
}

type layoutEvent struct {
	card     model.WBCard
	startMin int
	endMin   int
	assignee string
	lane     int
	lanes    int
	support  bool
	extra    int
}

func registerSlotTimes() []string {
	out := make([]string, 0, ((workdayEndHour-workdayStartHour)*60)/15+1)
	for hour := workdayStartHour; hour <= workdayEndHour; hour++ {
		for m := 0; m < 60; m += 15 {
			if hour == workdayEndHour && m > 0 {
				break
			}
			out = append(out, fmt.Sprintf("%02d:%02d", hour, m))
		}
	}
	return out
}

// registerGridStyles html/template이 calc()를 제거하지 않도록 rem 수치로 고정한다.
func registerGridStyles(slotCount int) (template.CSS, []template.CSS) {
	if slotCount < 1 {
		slotCount = 1
	}
	h := template.CSS(fmt.Sprintf("height:%.3frem", float64(slotCount)*registerSlotRem))
	tops := make([]template.CSS, slotCount)
	for i := 0; i < slotCount; i++ {
		tops[i] = template.CSS(fmt.Sprintf("top:%.3frem", float64(i)*registerSlotRem))
	}
	return h, tops
}

func parseHHMMToMin(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	// HH:MM 또는 HH:MM:SS
	parts := strings.Split(s, ":")
	if len(parts) < 2 {
		return 0, false
	}
	var h, m int
	if _, err := fmt.Sscanf(parts[0], "%d", &h); err != nil {
		return 0, false
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &m); err != nil {
		return 0, false
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

func eventEndMin(startMin, durationMin int, endTime string) int {
	if em, ok := parseHHMMToMin(endTime); ok && em > startMin {
		return em
	}
	if durationMin <= 0 {
		durationMin = 30
	}
	return startMin + durationMin
}

func buildRegisterDayColumns(cols []RegisterColumn, placed []model.WorkTask, toCard func(model.WorkTask) model.WBCard) []RegisterDayColumn {
	dayStart := workdayStartHour * 60
	dayEnd := workdayEndHour*60 + 15 // 20:00 슬롯 포함
	out := make([]RegisterDayColumn, len(cols))
	for i, col := range cols {
		out[i] = RegisterDayColumn{RegisterColumn: col}
		var evs []layoutEvent
		for _, t := range placed {
			// 일/주: 해당 열 날짜에만. 월(주차 열): From~To 주 안 배정일.
			if col.From == col.To {
				if t.WorkDate != col.Date {
					continue
				}
			} else if t.WorkDate < col.From || t.WorkDate > col.To {
				continue
			}
			if strings.TrimSpace(t.StartTime) == "" {
				continue
			}
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
			// 레인 사이 미세 간격(calc 없이 — html/template CSS 필터 회피)
			gapPct := 0.4
			w := laneWidth - gapPct
			if w < 8 {
				w = laneWidth
				gapPct = 0
			}
			// 가로는 최대 ~1/5열 — 높이는 시간(span)과만 연동
			if w > registerCardMaxWidthPct {
				w = registerCardMaxWidthPct
			}
			l := left + gapPct/2
			topRem := float64(topSlots) * registerSlotRem
			heightRem := float64(span) * registerSlotRem
			style := fmt.Sprintf(
				"position:absolute;top:%.3frem;height:%.3frem;left:%.3f%%;width:%.3f%%;max-width:%.3f%%;border-left:3px solid %s;background-color:%s;color:%s;box-sizing:border-box;z-index:5",
				topRem, heightRem, l, w, registerCardMaxWidthPct, border, soft, text,
			)
			blocks = append(blocks, RegisterBlock{
				Card: e.card, Style: template.CSS(style),
				BorderColor: border, SoftBg: soft, TextColor: text,
				Lane: lane, Lanes: lanes,
				TopRem: topRem, HeightRem: heightRem, LeftPct: l, WidthPct: w,
			})
		}
		out[i].Blocks = blocks
	}
	return out
}

// assignAssigneeLanes 시간이 겹치는 카드는 담당자 동일 여부와 관계없이 가로 레인으로 나눈다.
// (같은 담당자·같은 시각에 여러 완료 건이 있으면 서로 덮어쓰지 않도록 함)
func assignAssigneeLanes(evs []layoutEvent) {
	n := len(evs)
	if n == 0 {
		return
	}
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		i, j := order[a], order[b]
		if evs[i].startMin != evs[j].startMin {
			return evs[i].startMin < evs[j].startMin
		}
		if evs[i].endMin != evs[j].endMin {
			return evs[i].endMin > evs[j].endMin
		}
		if evs[i].assignee != evs[j].assignee {
			return evs[i].assignee < evs[j].assignee
		}
		return evs[i].card.TaskID < evs[j].card.TaskID
	})

	for _, idx := range order {
		e := &evs[idx]
		used := map[int]bool{}
		for j := 0; j < n; j++ {
			if evs[j].lane < 0 || j == idx {
				continue
			}
			if timeOverlap(*e, evs[j]) {
				used[evs[j].lane] = true
			}
		}
		lane := 0
		for used[lane] {
			lane++
		}
		e.lane = lane
	}

	for i := 0; i < n; i++ {
		maxLane := evs[i].lane
		overlapAny := false
		for j := 0; j < n; j++ {
			if i == j || !timeOverlap(evs[i], evs[j]) {
				continue
			}
			overlapAny = true
			if evs[j].lane > maxLane {
				maxLane = evs[j].lane
			}
		}
		if overlapAny {
			evs[i].lanes = maxLane + 1
		} else {
			evs[i].lanes = 1
			evs[i].lane = 0
		}
	}
}

func timeOverlap(a, b layoutEvent) bool {
	return a.startMin < b.endMin && b.startMin < a.endMin
}

// buildAssigneeLegend 배정 가능 사용자 + 배치/팔레트에 등장한 담당자 전부를 색과 함께 표시.
func buildAssigneeLegend(placed []model.WorkTask, users []model.User, extraCards ...[]model.WBCard) []AssigneeLegend {
	seen := map[string]bool{}
	var names []string
	add := func(n string) {
		n = strings.TrimSpace(n)
		if n == "" || seen[n] {
			return
		}
		seen[n] = true
		names = append(names, n)
	}
	for _, u := range users {
		add(u.FullName)
	}
	for _, t := range placed {
		add(t.Assignee)
	}
	for _, cards := range extraCards {
		for _, c := range cards {
			add(c.Assignee)
		}
	}
	sort.Strings(names)
	out := make([]AssigneeLegend, 0, len(names))
	for _, n := range names {
		b, s, _ := model.AssigneeColor(n)
		out = append(out, AssigneeLegend{Name: n, Border: b, Soft: s})
	}
	return out
}

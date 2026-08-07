package handler

import (
	"fmt"
	"html/template"
	"sort"
	"strings"
	"time"

	"customer-support/internal/model"
)

const registerSlotRem = 1.55 // 15분 칸 높이(rem) — 템플릿과 동일

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
}

// RegisterDayColumn 요일(또는 일) 열 + 배치 블록
type RegisterDayColumn struct {
	RegisterColumn
	Blocks []RegisterBlock
}

// AssigneeLegend 상단 범례용
type AssigneeLegend struct {
	Name   string
	Border string
	Soft   string
}

// RegisterMonthDay 월간 캘린더 하루 칸
type RegisterMonthDay struct {
	Date       string
	DayNum     int
	InMonth    bool
	IsToday    bool
	IsWeekend  bool
	Cards      []model.WBCard
	Visible    []model.WBCard
	MoreCount  int
	Unassigned int
	Total      int
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
			width := 100.0 / float64(lanes)
			left := width * float64(lane)
			// 레인 사이 미세 간격(calc 없이 — html/template CSS 필터 회피)
			gapPct := 0.4
			w := width - gapPct
			if w < 8 {
				w = width
				gapPct = 0
			}
			l := left + gapPct/2
			topRem := float64(topSlots) * registerSlotRem
			heightRem := float64(span) * registerSlotRem
			style := fmt.Sprintf(
				"position:absolute;top:%.3frem;height:%.3frem;left:%.3f%%;width:%.3f%%;border-left:3px solid %s;background-color:%s;color:%s;box-sizing:border-box;z-index:5",
				topRem, heightRem, l, w, border, soft, text,
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

// assignAssigneeLanes 직접 시간이 겹치고 담당자가 다르면 가로 레인으로 나눈다.
// (전이적 연결로 하루 전체를 한 덩어리로 묶지 않음)
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
		return evs[i].assignee < evs[j].assignee
	})

	for _, idx := range order {
		e := &evs[idx]
		used := map[int]bool{}
		for j := 0; j < n; j++ {
			if evs[j].lane < 0 || j == idx {
				continue
			}
			if timeOverlap(*e, evs[j]) && e.assignee != evs[j].assignee {
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
		cross := false
		for j := 0; j < n; j++ {
			if i == j || !timeOverlap(evs[i], evs[j]) {
				continue
			}
			if evs[i].assignee == evs[j].assignee {
				continue
			}
			cross = true
			if evs[j].lane > maxLane {
				maxLane = evs[j].lane
			}
		}
		if !cross {
			evs[i].lanes = 1
			evs[i].lane = 0
		} else {
			evs[i].lanes = maxLane + 1
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

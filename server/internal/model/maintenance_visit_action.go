package model

import (
	"fmt"
	"strings"
)

const MaxVisitTargets = 50

// NormalizeVisitClock HH:MM. 빈 값이면 "". 잘못된 값이면 ok=false.
func NormalizeVisitClock(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", true
	}
	min := ParseHHMMMinutes(raw)
	if min < 0 {
		return "", false
	}
	return FormatHHMMMinutes(min), true
}

// VisitSiteDurationMin 시작·종료가 있으면 현장 소요(분). 둘 다 없으면 0. 종료≤시작이면 ok=false.
func VisitSiteDurationMin(start, end string) (int, bool) {
	s, okS := NormalizeVisitClock(start)
	e, okE := NormalizeVisitClock(end)
	if !okS || !okE {
		return 0, false
	}
	if s == "" && e == "" {
		return 0, true
	}
	if s == "" || e == "" {
		return 0, true
	}
	sm := ParseHHMMMinutes(s)
	em := ParseHHMMMinutes(e)
	if sm < 0 || em < 0 {
		return 0, false
	}
	if em <= sm {
		return 0, false
	}
	return em - sm, true
}

// FormatDurationLabel 분 → "1시간 30분". 0이면 "—".
func FormatDurationLabel(min int) string {
	if min <= 0 {
		return "—"
	}
	h := min / 60
	r := min % 60
	if h > 0 && r > 0 {
		return fmt.Sprintf("%d시간 %d분", h, r)
	}
	if h > 0 {
		return fmt.Sprintf("%d시간", h)
	}
	return fmt.Sprintf("%d분", r)
}

// FormatVisitClockRange 10:00~11:30. 없으면 "".
func FormatVisitClockRange(start, end string) string {
	s, _ := NormalizeVisitClock(start)
	e, _ := NormalizeVisitClock(end)
	if s != "" && e != "" {
		return s + "~" + e
	}
	if s != "" {
		return s
	}
	return e
}

// FormatVisitTargetsLine 장비 이름 나열. 3개 초과 시 「외 N대」.
func FormatVisitTargetsLine(items []MaintenanceVisitTarget) string {
	var names []string
	for _, it := range items {
		n := strings.TrimSpace(it.Name)
		if n == "" {
			continue
		}
		names = append(names, n)
	}
	if len(names) == 0 {
		return ""
	}
	if len(names) <= 3 {
		return strings.Join(names, ", ")
	}
	return strings.Join(names[:3], ", ") + fmt.Sprintf(" 외 %d대", len(names)-3)
}

func (v MaintenanceVisit) TotalVisitMinutes() int {
	d := v.DurationMin
	if d <= 0 {
		if got, ok := VisitSiteDurationMin(v.StartTime, v.EndTime); ok {
			d = got
		}
	}
	if d <= 0 {
		return 0
	}
	if v.TravelMin < 0 {
		return d
	}
	return d + v.TravelMin*2
}

func (v MaintenanceVisit) AssigneeSummary() string {
	names := make([]string, 0, len(v.People)+1)
	seen := map[string]bool{}
	add := func(n string) {
		n = strings.TrimSpace(n)
		if n == "" || seen[n] {
			return
		}
		seen[n] = true
		names = append(names, n)
	}
	for _, p := range v.People {
		add(p.AssigneeName)
	}
	add(v.Assignee)
	if len(names) == 0 {
		return "—"
	}
	if len(names) == 1 {
		return names[0]
	}
	return names[0] + fmt.Sprintf(" 외 %d명", len(names)-1)
}

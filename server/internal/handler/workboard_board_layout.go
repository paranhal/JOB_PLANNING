package handler

import "sort"

// DayLayoutEvent 일 화면 겹침 배치 입력. 시각은 분(0~24*60).
type DayLayoutEvent struct {
	StartMin int
	EndMin   int
	Col      int
	Cols     int
}

// layoutDayEvents 겹치는 일정을 가로 열로 나눈다. 종료=시작은 겹치지 않는다.
func layoutDayEvents(list []DayLayoutEvent) []DayLayoutEvent {
	ev := make([]DayLayoutEvent, 0, len(list))
	idx := make([]int, 0, len(list))
	for i, e := range list {
		s, end := e.StartMin, e.EndMin
		if end <= s {
			end = s + 30
		}
		ev = append(ev, DayLayoutEvent{StartMin: s, EndMin: end})
		idx = append(idx, i)
	}
	sort.SliceStable(idx, func(a, b int) bool {
		if ev[idx[a]].StartMin != ev[idx[b]].StartMin {
			return ev[idx[a]].StartMin < ev[idx[b]].StartMin
		}
		return ev[idx[a]].EndMin < ev[idx[b]].EndMin
	})
	ordered := make([]DayLayoutEvent, len(ev))
	for i, id := range idx {
		ordered[i] = ev[id]
	}

	group := []int{}
	gEnd := 0
	flush := func() {
		n := 0
		for _, gi := range group {
			if ordered[gi].Col+1 > n {
				n = ordered[gi].Col + 1
			}
		}
		if n < 1 {
			n = 1
		}
		for _, gi := range group {
			ordered[gi].Cols = n
		}
		group = group[:0]
	}
	for i := range ordered {
		if len(group) > 0 && ordered[i].StartMin >= gEnd {
			flush()
			gEnd = 0
		}
		used := map[int]bool{}
		for _, gi := range group {
			if ordered[gi].EndMin > ordered[i].StartMin {
				used[ordered[gi].Col] = true
			}
		}
		c := 0
		for used[c] {
			c++
		}
		ordered[i].Col = c
		group = append(group, i)
		if ordered[i].EndMin > gEnd {
			gEnd = ordered[i].EndMin
		}
	}
	flush()

	out := make([]DayLayoutEvent, len(list))
	for i, id := range idx {
		out[id] = ordered[i]
	}
	return out
}

package model

import "math"

// UniqueWorkList 업무 회의 칸반과 같은 키로 중복을 뺀다.
func UniqueWorkList(lists ...[]WorkListItem) []WorkListItem {
	seen := map[string]bool{}
	var out []WorkListItem
	for _, items := range lists {
		for _, it := range items {
			k := WorkListItemKey(it)
			if k == ":" || seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, it)
		}
	}
	return out
}

// MeetingProgressExcludingScheduled 오늘 예정과 겹치는 진행중 행을 뺀다. 회의 Show 와 동일.
func MeetingProgressExcludingScheduled(items []WorkListItem, date string) []WorkListItem {
	var out []WorkListItem
	for _, it := range items {
		if WorkItemScheduledDate(it) == date {
			continue
		}
		out = append(out, it)
	}
	return out
}

// MeetingTodayUnion 오늘 완료 ∪ 오늘 예정 ∪ (예정일이 오늘이 아닌 진행중).
func MeetingTodayUnion(done, sched, progress []WorkListItem, date string) []WorkListItem {
	return UniqueWorkList(done, sched, MeetingProgressExcludingScheduled(progress, date))
}

// ExecTodayCounts 완료 / 진행 / 미착수 / 지연. 한 건은 한 칸만. Total = 합.
type ExecTodayCounts struct {
	Total, Done, InProgress, NotStarted, Delayed int
	DonePct                                      int
}

// ClassifyExecToday 오늘 업무 집합을 4칸으로 나눈다. 완료 우선, 그다음 지연(95-80), 진행, 미착수.
func ClassifyExecToday(items []WorkListItem, date string) ExecTodayCounts {
	var c ExecTodayCounts
	seen := map[string]bool{}
	for _, it := range items {
		k := WorkListItemKey(it)
		if k == ":" || seen[k] {
			continue
		}
		seen[k] = true
		mapped := it.MappedStatus
		if mapped == "" {
			mapped = MapASStatusToWB(it.Status)
		}
		switch {
		case mapped == WBTaskComplete:
			c.Done++
		case WorkListTaskDelayState(it, date).Delayed:
			c.Delayed++
		case WBKanbanBucket(mapped) == WBTaskInProgress:
			c.InProgress++
		default:
			c.NotStarted++
		}
	}
	c.Total = c.Done + c.InProgress + c.NotStarted + c.Delayed
	if c.Total > 0 {
		c.DonePct = int(math.Round(100 * float64(c.Done) / float64(c.Total)))
	}
	return c
}

// ExecBarPct 진행 막대 너비. 0건이면 0.
func ExecBarPct(n, total int) int {
	if total <= 0 || n <= 0 {
		return 0
	}
	return int(math.Round(100 * float64(n) / float64(total)))
}

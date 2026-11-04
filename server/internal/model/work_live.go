package model

import "strings"

type CompletionRule struct {
	Kind  string // self | action | source
	Label string
}

type FitCheckResult struct {
	NeedMin        int
	AvailMin       int
	ShortMin       int
	OK             bool
	NextFixedTitle string
	Choices        []string
}

type ParentProgress struct {
	Total          int
	Done           int
	InProgress     int
	Planned        int
	Held           int
	Deferred       int
	EstMin         int
	ActualMin      int
	TravelMin      int
	OccupyMin      int
	RemainingMin   int
	ByTimePct      *int
	ByCountPct     *int
	InterruptCount int
	InterruptMin   int
}

type TaskTimeSummary struct {
	ActualMin  int
	TravelMin  int
	OccupyMin  int
	ElapsedMin int
	PausedMin  int
}

func FitCheckChoices() []string {
	return []string{
		"끼어든 업무 담당자 변경",
		"다음 고정 일정 조정",
		"끼어든 업무를 전화·원격으로 전환(이동 0)",
		"현재 업무를 중단하고 다음 조치일 지정",
		"충돌을 알고 강제 배치",
	}
}

func OccupancyLine(actual, travel int) string {
	return FormatWorkOccupyLine(actual, travel, actual+travel)
}

func FormatWorkOccupyLine(actual, travel, occupy int) string {
	return "업무 " + itoa(actual) + "분 · 이동 " + itoa(travel) + "분 · 일정 점유 " + itoa(occupy) + "분"
}

func formatHourMin(min int) string {
	if min < 0 {
		min = 0
	}
	if min%60 == 0 && min >= 60 {
		return itoa(min/60) + "h"
	}
	if min >= 60 {
		h, m := min/60, min%60
		return itoa(h) + "h " + itoa(m) + "분"
	}
	return itoa(min) + "분"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func PlanVsActualDaysBadge(planDays, actualDays int) string {
	if actualDays > planDays && planDays > 0 {
		return "계획 " + itoa(planDays) + "일 → 실제 " + itoa(actualDays) + "일"
	}
	return ""
}

func UniqueWorkDates(dates []string) int {
	seen := map[string]bool{}
	n := 0
	for _, d := range dates {
		d = strings.TrimSpace(d)
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		n++
	}
	return n
}

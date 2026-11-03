package model

import "strings"

// TaskDelay 종료일 기준 지연 판정 결과. 소비 화면은 이 값만 쓴다(95-80).
type TaskDelay struct {
	Delayed bool
	Days    int
	Due     string
}

// TaskDelayState 지연 판정 한 곳. 종료일이 있고 오늘보다 이전이면 지연.
// 종료일 없음·완료·취소는 지연이 아니다. 부분완료는 종료일이 지났으면 지연(Q-2).
func TaskDelayState(t WorkTask, today string) TaskDelay {
	today = NormalizeAppDate(today)
	st := strings.TrimSpace(t.Status)
	if st == WBTaskComplete || st == WBTaskCancelled {
		return TaskDelay{}
	}
	due := NormalizeAppDate(t.DueDate)
	if due == "" || today == "" || due >= today {
		return TaskDelay{Due: due}
	}
	d := DaysBetweenDates(due, today)
	if d < 1 {
		d = 1
	}
	return TaskDelay{Delayed: true, Days: d, Due: due}
}

func workListUsesOwnDue(it WorkListItem) bool {
	switch strings.TrimSpace(it.Prefix) {
	case WorkPrefixGeneral, WorkPrefixSales, WorkPrefixProjectWork:
		return true
	default:
		return false
	}
}

// WorkListTaskDelayState 통합 목록 행. work_tasks(행정·지원·영업·작업)는 due_date,
// AS·하부·점검은 예정일 하나가 종료일이다.
func WorkListTaskDelayState(it WorkListItem, today string) TaskDelay {
	t := WorkTask{
		Status:   it.Status,
		WorkDate: it.WorkDate,
		DueDate:  it.DueDate,
	}
	if strings.TrimSpace(t.Status) == "" {
		t.Status = it.MappedStatus
	}
	// 회신 대기(§13.6)·반복 회차 미완료는 종료일이 아니라 회신예정·회차일이다.
	useSched := !workListUsesOwnDue(it) ||
		it.SubLabel == "회신 대기" || it.SubLabel == "미완료"
	if useSched {
		t.DueDate = WorkItemScheduledDate(it)
	}
	return TaskDelayState(t, today)
}

func TruncRunes(s string, n int) string {
	s = strings.TrimSpace(s)
	if n <= 0 || s == "" {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func TaskSpansDate(workDate, dueDate, day string) bool {
	day = NormalizeAppDate(day)
	w := NormalizeAppDate(workDate)
	d := NormalizeAppDate(dueDate)
	if day == "" {
		return false
	}
	if w == "" {
		w = d
	}
	if w == "" {
		return false
	}
	if d == "" {
		return w == day
	}
	return w <= day && day <= d
}

func TaskIsSpan(workDate, dueDate string) bool {
	w := NormalizeAppDate(workDate)
	d := NormalizeAppDate(dueDate)
	return w != "" && d != "" && w < d
}

func DelayDueForStats(origDue, due string) string {
	if o := NormalizeAppDate(origDue); o != "" {
		return o
	}
	return NormalizeAppDate(due)
}

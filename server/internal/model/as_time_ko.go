package model

import (
	"fmt"
	"strconv"
	"strings"
)

// FormatKoreanTime HH:MM(24시간)을 「오전/오후 hh:mm」으로 바꾼다. 95-85 §2-1A.
// 오전 12:00=자정, 오후 12:00=정오.
func FormatKoreanTime(hhmm string) string {
	h, m, ok := parseClockHHMM(hhmm)
	if !ok {
		return strings.TrimSpace(hhmm)
	}
	period := "오전"
	h12 := h
	switch {
	case h == 0:
		h12 = 12
	case h == 12:
		period = "오후"
		h12 = 12
	case h > 12:
		period = "오후"
		h12 = h - 12
	}
	return fmt.Sprintf("%s %02d:%02d", period, h12, m)
}

func parseClockHHMM(s string) (hour, min int, ok bool) {
	s = strings.TrimSpace(s)
	if len(s) < 4 {
		return 0, 0, false
	}
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	h, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	m, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, false
	}
	return h, m, true
}

// ClockHHMMStrict 시각을 HH:MM 으로 맞춘다. 비어 있으면 빈 문자열.
func ClockHHMMStrict(s string) string {
	h, m, ok := parseClockHHMM(s)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%02d:%02d", h, m)
}

// IsTenMinuteClock 10분 단위 시각인지.
func IsTenMinuteClock(s string) bool {
	_, m, ok := parseClockHHMM(s)
	return ok && m%10 == 0
}

// AddMinutesClock 시작+분에 완료 시각. 자정을 넘으면 ok=false.
func AddMinutesClock(startHHMM string, durationMin int) (end string, ok bool) {
	h, m, parsed := parseClockHHMM(startHHMM)
	if !parsed || durationMin < 0 {
		return "", false
	}
	total := h*60 + m + durationMin
	if total >= 24*60 {
		return "", false
	}
	return fmt.Sprintf("%02d:%02d", total/60, total%60), true
}

// FormatDurationMin 작업 시간 드롭다운 표기. 예: 1시간 30분.
func FormatDurationMin(min int) string {
	if min < 0 {
		min = 0
	}
	h := min / 60
	m := min % 60
	switch {
	case h == 0:
		return fmt.Sprintf("%d분", m)
	case m == 0:
		return fmt.Sprintf("%d시간", h)
	default:
		return fmt.Sprintf("%d시간 %d분", h, m)
	}
}

// TimeSlotOptions 00:00~23:50 10분 단위. Value는 24시간, Label은 오전/오후.
func TimeSlotOptions() []ActionResultOption {
	out := make([]ActionResultOption, 0, 144)
	for h := 0; h < 24; h++ {
		for m := 0; m < 60; m += 10 {
			v := fmt.Sprintf("%02d:%02d", h, m)
			out = append(out, ActionResultOption{Value: v, Label: FormatKoreanTime(v)})
		}
	}
	return out
}

// DurationMinOptions 10분~12시간(720분) 10분 단위.
func DurationMinOptions() []ActionResultOption {
	out := make([]ActionResultOption, 0, 72)
	for min := 10; min <= 12*60; min += 10 {
		out = append(out, ActionResultOption{Value: strconv.Itoa(min), Label: FormatDurationMin(min)})
	}
	return out
}

// ClockAfterOrEqual a >= b (같은 날 HH:MM).
func ClockAfterOrEqual(a, b string) bool {
	ah, am, aok := parseClockHHMM(a)
	bh, bm, bok := parseClockHHMM(b)
	if !aok || !bok {
		return false
	}
	return ah*60+am >= bh*60+bm
}

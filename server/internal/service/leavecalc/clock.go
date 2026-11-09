package leavecalc

import "fmt"

// 근무·점심. 반차 4시간은 점심을 건너뛴다.
const (
	WorkStart  = 9 * 60
	WorkEnd    = 18 * 60
	LunchStart = 12 * 60
	LunchEnd   = 13 * 60
	HalfLatest = 14 * 60
)

func FormatHHMM(min int) string {
	if min < 0 {
		min = 0
	}
	return fmt.Sprintf("%02d:%02d", min/60, min%60)
}

func overlapMin(a0, a1, b0, b1 int) int {
	lo, hi := a0, a1
	if b0 > lo {
		lo = b0
	}
	if b1 < hi {
		hi = b1
	}
	if hi <= lo {
		return 0
	}
	return hi - lo
}

// ClockWorkMinutes is on-the-clock minutes minus 12:00~13:00 overlap.
func ClockWorkMinutes(startHHMM, endHHMM string) (int, error) {
	s, err := parseHHMM(startHHMM)
	if err != nil {
		return 0, err
	}
	e, err := parseHHMM(endHHMM)
	if err != nil {
		return 0, err
	}
	if e <= s {
		return 0, fmt.Errorf("end time must be after start")
	}
	return (e - s) - overlapMin(s, e, LunchStart, LunchEnd), nil
}

func addWorkMinutes(start, need int) int {
	t := start
	left := need
	for left > 0 {
		if t >= LunchStart && t < LunchEnd {
			t = LunchEnd
			continue
		}
		t++
		left--
	}
	return t
}

// ResolveHalf 시작 시각 → 보정된 시작·종료·오전/오후. end_time·half_part는 서버가 확정한다.
func ResolveHalf(startHHMM string) (start, end, part string, err error) {
	s, err := parseHHMM(startHHMM)
	if err != nil {
		return "", "", "", fmt.Errorf("반차 시작 시각을 입력하세요")
	}
	if s >= LunchStart && s < LunchEnd {
		s = LunchEnd
	}
	if s < WorkStart || s > HalfLatest {
		return "", "", "", fmt.Errorf("반차 시작은 09:00~14:00 입니다")
	}
	endMin := addWorkMinutes(s, DefaultHalfDayMinutes)
	if endMin > WorkEnd {
		return "", "", "", fmt.Errorf("반차 종료가 근무 시간을 넘습니다")
	}
	part = "am"
	if s >= LunchEnd {
		part = "pm"
	}
	return FormatHHMM(s), FormatHHMM(endMin), part, nil
}

package leavecalc

import "fmt"

// FormatDays 화면용 일수. 분모 0이 아니라 호출 쪽에서 미등록은 "—"를 쓴다.
func FormatDays(minutes int) string {
	if minutes == 0 {
		return "0일"
	}
	sign := ""
	if minutes < 0 {
		sign = "-"
		minutes = -minutes
	}
	day := DefaultWorkMinutesPerDay
	if day <= 0 {
		day = 480
	}
	if minutes%day == 0 {
		return fmt.Sprintf("%s%d일", sign, minutes/day)
	}
	return fmt.Sprintf("%s%.1f일", sign, float64(minutes)/float64(day))
}

func DaysFloat(minutes int) float64 {
	day := DefaultWorkMinutesPerDay
	if day <= 0 {
		day = 480
	}
	return float64(minutes) / float64(day)
}

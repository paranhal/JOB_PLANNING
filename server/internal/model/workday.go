package model

import (
	"strings"
	"time"
)

// DateIsWorkingDay 토·일이 아니고 휴일 표에 없으면 영업일. §4.14.2 · §23.13.7
// service.Calendar.IsWorkingDay 와 같은 판정이다. 새 규칙을 만들지 않는다.
func DateIsWorkingDay(date string, holidayHit bool) bool {
	t, _, ok := ParseYMD(date)
	if !ok {
		return false
	}
	if t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
		return false
	}
	return !holidayHit
}

// ParseYMD YYYY-MM-DD. 앞 10자만 본다.
func ParseYMD(date string) (time.Time, string, bool) {
	date = strings.TrimSpace(date)
	if len(date) >= 10 {
		date = date[:10]
	}
	t, err := time.ParseInLocation("2006-01-02", date, time.Local)
	if err != nil {
		return time.Time{}, "", false
	}
	return t, date, true
}

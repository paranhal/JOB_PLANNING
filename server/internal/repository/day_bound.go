package repository

import (
	"strings"
	"time"
)

// dayTimeStart 날짜·일시를 그날 00:00:00 으로 맞춘다. 열을 date() 로 감싸지 않고 이 값을 넘긴다. §44.7
func dayTimeStart(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 10 {
		return s[:10] + " 00:00:00"
	}
	return s
}

// dayTimeNext 다음날 00:00:00. 포함 끝날을 < 이 값으로 둔다.
func dayTimeNext(s string) string {
	s = strings.TrimSpace(s)
	if len(s) < 10 {
		return s
	}
	t, err := time.ParseInLocation("2006-01-02", s[:10], time.Local)
	if err != nil {
		return dayTimeStart(s)
	}
	return t.AddDate(0, 0, 1).Format("2006-01-02") + " 00:00:00"
}

func todayTimeStart() string {
	return time.Now().In(time.Local).Format("2006-01-02") + " 00:00:00"
}

func tomorrowTimeStart() string {
	return time.Now().In(time.Local).AddDate(0, 0, 1).Format("2006-01-02") + " 00:00:00"
}

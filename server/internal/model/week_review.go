package model

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// WeekReview 월요일에 보여주는 지난주 한 줄. §42.7.3
type WeekReview struct {
	Completed int
	AvgLead   float64
	AvgOK     bool
	OnTime    int
	Carried   int
}

func (w WeekReview) Line() string {
	avg := "—"
	if w.AvgOK {
		avg = strconv.FormatFloat(w.AvgLead, 'f', 1, 64)
	}
	return fmt.Sprintf("지난주  완료 %d건 · 평균 %s영업일 · 기한 내 %d건 · 밀린 채 넘어온 것 %d건",
		w.Completed, avg, w.OnTime, w.Carried)
}

// CalendarMonday 그 주의 월요일(로컬 날짜).
func CalendarMonday(now time.Time) time.Time {
	loc := now.Location()
	d := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	wd := int(d.Weekday())
	if wd == 0 {
		wd = 7
	}
	return d.AddDate(0, 0, 1-wd)
}

// LastISOWeekRange 직전 월요일~일요일.
func LastISOWeekRange(now time.Time) (from, to string) {
	mon := CalendarMonday(now)
	return mon.AddDate(0, 0, -7).Format("2006-01-02"), mon.AddDate(0, 0, -1).Format("2006-01-02")
}

func IsMonday(now time.Time) bool {
	return now.Weekday() == time.Monday
}

const (
	BlockedReasonParts     = "부품 대기"
	BlockedReasonNoContact = "고객 연락 두절"
)

func NormalizeBlockedReason(s string) string {
	s = strings.TrimSpace(s)
	switch s {
	case BlockedReasonParts, "parts", "부품":
		return BlockedReasonParts
	case BlockedReasonNoContact, "no_contact", "연락두절":
		return BlockedReasonNoContact
	default:
		return s
	}
}

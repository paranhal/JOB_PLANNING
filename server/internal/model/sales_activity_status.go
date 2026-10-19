package model

import (
	"fmt"
	"strings"
	"time"
)

const (
	ActStatusPlanned  = "planned"
	ActStatusDone     = "done"
	ActStatusFollowup = "followup"
)

func ParseAppDay(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	t, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func LatestActivityIDBySales(acts []SalesActivity) map[string]string {
	best := map[string]SalesActivity{}
	for _, a := range acts {
		sid := strings.TrimSpace(a.SalesID)
		if sid == "" {
			continue
		}
		cur, ok := best[sid]
		if !ok || salesActivityNewer(a, cur) {
			best[sid] = a
		}
	}
	out := make(map[string]string, len(best))
	for sid, a := range best {
		out[sid] = a.ActivityID
	}
	return out
}

func salesActivityNewer(a, b SalesActivity) bool {
	if a.ActivityDate != b.ActivityDate {
		return a.ActivityDate > b.ActivityDate
	}
	ta, tb := strings.TrimSpace(a.StartTime), strings.TrimSpace(b.StartTime)
	if ta == "" {
		ta = "00:00"
	}
	if tb == "" {
		tb = "00:00"
	}
	if ta != tb {
		return ta > tb
	}
	return a.ActivityID > b.ActivityID
}

func DeriveSalesActivityStatus(a SalesActivity, latestBySales map[string]string, today string) string {
	today = strings.TrimSpace(today)
	if a.ActivityDate > today {
		return ActStatusPlanned
	}
	if latestBySales[a.SalesID] == a.ActivityID {
		nd := strings.TrimSpace(a.NextActionDate)
		if nd != "" && nd < today {
			return ActStatusFollowup
		}
	}
	return ActStatusDone
}

func SalesActivityStatusLabel(code string) string {
	switch code {
	case ActStatusPlanned:
		return "예정"
	case ActStatusFollowup:
		return "후속 지연"
	default:
		return "완료"
	}
}

type SalesActivityKPI struct {
	Total       int
	MonthNew    int
	WeekPlanned int
	Done        int
	Followup    int
	DoneRate    string
}

func BuildSalesActivityKPI(acts []SalesActivity, latest map[string]string, today string) SalesActivityKPI {
	k := SalesActivityKPI{DoneRate: "—"}
	if today == "" {
		today = time.Now().Format("2006-01-02")
	}
	month := today[:7]
	weekEnd := weekSunday(today)
	for _, a := range acts {
		k.Total++
		if strings.HasPrefix(a.CreatedAt, month) || (a.CreatedAt == "" && strings.HasPrefix(a.ActivityDate, month)) {
			k.MonthNew++
		}
		st := DeriveSalesActivityStatus(a, latest, today)
		if st == ActStatusPlanned && a.ActivityDate >= today && a.ActivityDate <= weekEnd {
			k.WeekPlanned++
		}
		if st != ActStatusPlanned {
			k.Done++
		}
		if st == ActStatusFollowup {
			k.Followup++
		}
	}
	if k.Total > 0 {
		k.DoneRate = fmt.Sprintf("%d%%", k.Done*100/k.Total)
	}
	return k
}

func weekSunday(today string) string {
	t, ok := ParseAppDay(today)
	if !ok {
		return today
	}
	// Sunday = 0. Days until Sunday.
	wd := int(t.Weekday())
	add := (7 - wd) % 7
	return t.AddDate(0, 0, add).Format("2006-01-02")
}

func PeriodRange(period, today string) (from, to string) {
	today = strings.TrimSpace(today)
	switch strings.TrimSpace(period) {
	case "all", "":
		if period == "all" {
			return "", ""
		}
		if len(today) >= 7 {
			return today[:7] + "-01", monthLastDay(today)
		}
		return "", ""
	case "month":
		if len(today) >= 7 {
			return today[:7] + "-01", monthLastDay(today)
		}
	case "30":
		t, ok := ParseAppDay(today)
		if !ok {
			return "", today
		}
		return t.AddDate(0, 0, -29).Format("2006-01-02"), today
	}
	return "", ""
}

func monthLastDay(today string) string {
	t, ok := ParseAppDay(today[:7] + "-01")
	if !ok {
		return today
	}
	return t.AddDate(0, 1, -1).Format("2006-01-02")
}

func SalesActivityTypeKO(code, fallback string) string {
	switch strings.TrimSpace(code) {
	case "call":
		return "전화"
	case "visit":
		return "방문"
	case "mail":
		return "이메일"
	case "research":
		return "조사"
	case "internal":
		return "내부"
	case "bid":
		return "입찰"
	}
	if strings.TrimSpace(fallback) != "" {
		return fallback
	}
	return code
}

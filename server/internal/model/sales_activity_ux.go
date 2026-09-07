package model

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// FormatSalesDuration 요약용. 150분 → "2시간 30분" (§32.8.3).
func FormatSalesDuration(min int) string {
	if min <= 0 {
		return "0분"
	}
	h, m := min/60, min%60
	switch {
	case h > 0 && m > 0:
		return fmt.Sprintf("%d시간 %d분", h, m)
	case h > 0:
		return fmt.Sprintf("%d시간", h)
	default:
		return fmt.Sprintf("%d분", min)
	}
}

// FormatMinutesAsCard 카드 우측. 90분 그대로.
func FormatMinutesAsCard(min int) string {
	if min <= 0 {
		return "0분"
	}
	return fmt.Sprintf("%d분", min)
}

// SalesNextActionChip 다음 행동 칩. today 가 비면 오늘(로컬).
func SalesNextActionChip(next, date, today string) (label, class string) {
	next = strings.TrimSpace(next)
	date = strings.TrimSpace(date)
	if today == "" {
		today = time.Now().Format("2006-01-02")
	}
	if next == "" && date == "" {
		return "다음 행동 없음", "text-gray-500 bg-gray-50 border-gray-200"
	}
	label = "다음: " + next
	if date != "" {
		if next == "" {
			label = "다음: " + date
		} else {
			label += " · " + date
		}
	}
	class = "text-orange-800 bg-orange-50 border-orange-200"
	if date != "" && date < today {
		class = "text-red-700 bg-red-50 border-red-200"
	}
	if date == today {
		class += " font-semibold"
	}
	return label, class
}

func SalesActivityDayHeading(date string, n int) string {
	t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(date), time.Local)
	if err != nil {
		return fmt.Sprintf("%s · %d건", date, n)
	}
	return fmt.Sprintf("%s (%s) · %d건", t.Format("2006-01-02"), WeekdayKR(t), n)
}

func SalesActivityTypeBadgeClass(code string) string {
	switch strings.TrimSpace(code) {
	case SalesActTypeVisit:
		return "bg-sky-100 text-sky-800"
	case SalesActTypeCall:
		return "bg-emerald-100 text-emerald-800"
	case SalesActTypeMail:
		return "bg-violet-100 text-violet-800"
	case SalesActTypeOnline:
		return "bg-indigo-100 text-indigo-800"
	case SalesActTypeInternal:
		return "bg-slate-100 text-slate-800"
	case SalesActTypeResearch:
		return "bg-amber-100 text-amber-900"
	case SalesActTypeQuote, SalesActTypeProposal, SalesActTypeRFP, SalesActTypeBid:
		return "bg-orange-100 text-orange-800"
	default:
		return "bg-gray-100 text-gray-700"
	}
}

type SalesActivitySummary struct {
	Count          int
	Minutes        int
	DurationLabel  string
	TopType        string
	TopLabel       string
	TopCount       int
}

func BuildSalesActivitySummary(acts []SalesActivity, types []Code) SalesActivitySummary {
	s := SalesActivitySummary{DurationLabel: FormatSalesDuration(0)}
	counts := map[string]int{}
	for _, a := range acts {
		s.Count++
		s.Minutes += a.DurationMin
		t := strings.TrimSpace(a.ActivityType)
		if t != "" {
			counts[t]++
		}
	}
	s.DurationLabel = FormatSalesDuration(s.Minutes)
	best, bestN := "", 0
	for _, c := range types {
		n := counts[c.CodeValue]
		if n > bestN {
			best, bestN = c.CodeValue, n
		}
	}
	if bestN == 0 {
		for code, n := range counts {
			if n > bestN {
				best, bestN = code, n
			}
		}
	}
	s.TopType = best
	s.TopCount = bestN
	for _, c := range types {
		if c.CodeValue == best {
			s.TopLabel = c.CodeName
			break
		}
	}
	if s.TopLabel == "" && best != "" {
		s.TopLabel = best
	}
	return s
}

type SalesActivityDayGroup struct {
	Date     string
	Heading  string
	Count    int
	DayStyle string
	DayClass string
	Items    []SalesActivity
}

func GroupSalesActivitiesByDate(acts []SalesActivity) []SalesActivityDayGroup {
	order := []string{}
	bag := map[string][]SalesActivity{}
	for _, a := range acts {
		d := strings.TrimSpace(a.ActivityDate)
		if d == "" {
			d = "미정"
		}
		if _, ok := bag[d]; !ok {
			order = append(order, d)
		}
		bag[d] = append(bag[d], a)
	}
	sort.Slice(order, func(i, j int) bool { return order[i] > order[j] })
	out := make([]SalesActivityDayGroup, 0, len(order))
	for _, d := range order {
		items := bag[d]
		out = append(out, SalesActivityDayGroup{
			Date:    d,
			Heading: SalesActivityDayHeading(d, len(items)),
			Count:   len(items),
			Items:   items,
		})
	}
	return out
}

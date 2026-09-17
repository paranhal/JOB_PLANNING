package model

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	LookbackUnitDay    = "d"
	LookbackUnitWeek   = "w"
	LookbackUnitMonth  = "m"
	LookbackUnitCustom = "custom"

	LookbackMaxDays   = 365
	LookbackMaxWeeks  = 104
	LookbackMaxMonths = 60
)

// StatsLookback §15.7 기간(range/from·to)과 그래프 쪼개기(view).
type StatsLookback struct {
	N              int
	Unit           string
	RangeParam     string
	Custom         bool
	RequestFrom    string
	RequestTo      string
	From           string
	To             string
	ToExclusive    string
	Headline       string
	ClippedByBase  bool
	BaseDate       string
	View           string
	ViewRequested  string
	ViewNarrowed   bool
	ViewNarrowNote string
}

func ParseStatsLookback(rangeQ, fromQ, toQ, viewQ, unitQ, baseDate string, now time.Time) StatsLookback {
	loc := now.Location()
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, loc)
	todayS := today.Format("2006-01-02")
	baseDate = NormalizeAppDate(baseDate)

	rangeQ = strings.TrimSpace(rangeQ)
	fromQ = NormalizeAppDate(fromQ)
	toQ = NormalizeAppDate(toQ)
	unitQ = strings.TrimSpace(unitQ)
	viewQ = strings.TrimSpace(viewQ)

	lb := StatsLookback{BaseDate: baseDate, ViewRequested: viewQ}
	if viewQ == StatsViewRange {
		lb.ViewRequested = ""
		viewQ = ""
		if fromQ != "" || toQ != "" {
			unitQ = LookbackUnitCustom
		}
	}

	custom := unitQ == LookbackUnitCustom || (rangeQ == "" && fromQ != "" && toQ != "")
	if custom {
		lb.Custom = true
		lb.Unit = LookbackUnitCustom
		lb.N = 1
		fromIncl, toIncl := parseCustomBounds(fromQ, toQ, today)
		lb.RequestFrom = fromIncl.Format("2006-01-02")
		lb.RequestTo = toIncl.Format("2006-01-02")
		lb.From, lb.To, lb.ClippedByBase = clipLookbackToBase(fromIncl, toIncl, baseDate, loc)
	} else {
		n, unit := parseRangeToken(rangeQ)
		if unit == "" {
			unit = LookbackUnitMonth
			n = 1
		}
		n, unit = clampLookback(n, unit)
		lb.N, lb.Unit = n, unit
		lb.RangeParam = fmt.Sprintf("%d%s", n, unit)
		fromIncl := lookbackStart(today, n, unit)
		if fromIncl.After(today) {
			fromIncl = today
		}
		lb.RequestFrom = fromIncl.Format("2006-01-02")
		lb.RequestTo = todayS
		lb.From, lb.To, lb.ClippedByBase = clipLookbackToBase(fromIncl, today, baseDate, loc)
	}

	if t, err := time.ParseInLocation("2006-01-02", lb.To, loc); err == nil {
		lb.ToExclusive = t.AddDate(0, 0, 1).Format("2006-01-02")
	} else {
		lb.ToExclusive = today.AddDate(0, 0, 1).Format("2006-01-02")
	}

	days := lookbackDays(lb.From, lb.To)
	fitted, note, narrowed := FitStatsView(viewQ, days)
	lb.View = fitted
	lb.ViewNarrowed = narrowed
	lb.ViewNarrowNote = note
	lb.Headline = lookbackHeadline(lb)
	return lb
}

func parseRangeToken(s string) (int, string) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return 0, ""
	}
	if len(s) < 2 {
		return 0, ""
	}
	unit := s[len(s)-1:]
	if unit != LookbackUnitDay && unit != LookbackUnitWeek && unit != LookbackUnitMonth {
		return 0, ""
	}
	n, err := strconv.Atoi(s[:len(s)-1])
	if err != nil {
		return 0, ""
	}
	return n, unit
}

func clampLookback(n int, unit string) (int, string) {
	if n < 1 {
		n = 1
	}
	switch unit {
	case LookbackUnitDay:
		if n > LookbackMaxDays {
			n = LookbackMaxDays
		}
	case LookbackUnitWeek:
		if n > LookbackMaxWeeks {
			n = LookbackMaxWeeks
		}
	default:
		unit = LookbackUnitMonth
		if n > LookbackMaxMonths {
			n = LookbackMaxMonths
		}
	}
	return n, unit
}

func lookbackStart(today time.Time, n int, unit string) time.Time {
	switch unit {
	case LookbackUnitDay:
		return today.AddDate(0, 0, 1-n)
	case LookbackUnitWeek:
		return today.AddDate(0, 0, 1-n*7)
	default:
		return today.AddDate(0, -n, 1)
	}
}

func parseCustomBounds(fromQ, toQ string, today time.Time) (fromIncl, toIncl time.Time) {
	fromIncl, toIncl = today, today
	if t, err := time.ParseInLocation("2006-01-02", fromQ, today.Location()); err == nil {
		fromIncl = t
	}
	if t, err := time.ParseInLocation("2006-01-02", toQ, today.Location()); err == nil {
		toIncl = t
	}
	if toIncl.Before(fromIncl) {
		fromIncl, toIncl = toIncl, fromIncl
	}
	max := fromIncl.AddDate(0, LookbackMaxMonths, 0)
	if toIncl.After(max) {
		toIncl = max
	}
	return fromIncl, toIncl
}

func clipLookbackToBase(fromIncl, toIncl time.Time, baseDate string, loc *time.Location) (fromS, toS string, clipped bool) {
	if baseDate != "" {
		if b, err := time.ParseInLocation("2006-01-02", baseDate, loc); err == nil {
			if fromIncl.Before(b) {
				fromIncl = b
				clipped = true
			}
			if toIncl.Before(b) {
				toIncl = b
				clipped = true
			}
		}
	}
	return fromIncl.Format("2006-01-02"), toIncl.Format("2006-01-02"), clipped
}

func lookbackDays(fromS, toS string) int {
	from, err1 := time.Parse("2006-01-02", fromS)
	to, err2 := time.Parse("2006-01-02", toS)
	if err1 != nil || err2 != nil {
		return 1
	}
	d := int(to.Sub(from).Hours()/24) + 1
	if d < 1 {
		return 1
	}
	return d
}

// FitStatsView range가 짧은데 view가 크면 좁힌다. §15.7.2
func FitStatsView(view string, days int) (fitted, note string, narrowed bool) {
	switch view {
	case StatsViewWeek, StatsViewMonth, StatsViewDay:
	default:
		view = StatsViewDay
	}
	fitted = view
	if days < 8 && (view == StatsViewWeek || view == StatsViewMonth) {
		fitted = StatsViewDay
	} else if days < 32 && view == StatsViewMonth {
		fitted = StatsViewWeek
	}
	if fitted != view {
		narrowed = true
		note = fmt.Sprintf("기간이 짧아 %s 대신 %s로 그립니다", viewLabelKo(view), viewLabelKo(fitted))
	}
	return fitted, note, narrowed
}

// AutoStatsView 대시보드 그래프 단위. 기간 일수로 정한다 (§4.11).
func AutoStatsView(days int) string {
	if days <= 31 {
		return StatsViewDay
	}
	if days <= 180 {
		return StatsViewWeek
	}
	return StatsViewMonth
}

// ChartBucketNote 그래프 축 안내. 예: 「주별로 묶어 표시」
func ChartBucketNote(view string) string {
	switch view {
	case StatsViewWeek:
		return "주별로 묶어 표시"
	case StatsViewMonth:
		return "월별로 묶어 표시"
	default:
		return "일별로 묶어 표시"
	}
}

// ApplyDashboardChartView 대시보드는 기간에서 단위를 고른다. 수동 view 쿼리는 무시한다 (§4.11).
func ApplyDashboardChartView(lb *StatsLookback) {
	if lb == nil {
		return
	}
	lb.View = AutoStatsView(lookbackDays(lb.From, lb.To))
	lb.ViewRequested = ""
	lb.ViewNarrowed = false
	lb.ViewNarrowNote = ""
}

func viewLabelKo(view string) string {
	switch view {
	case StatsViewWeek:
		return "주별"
	case StatsViewMonth:
		return "월별"
	default:
		return "일별"
	}
}

func lookbackHeadline(lb StatsLookback) string {
	span := lb.From + " ~ " + lb.To
	if lb.Custom {
		return "사용자 지정 (" + span + ")"
	}
	return fmt.Sprintf("최근 %d%s (%s)", lb.N, unitLabelKo(lb.Unit), span)
}

func unitLabelKo(unit string) string {
	switch unit {
	case LookbackUnitDay:
		return "일"
	case LookbackUnitWeek:
		return "주"
	default:
		return "개월"
	}
}

func (lb StatsLookback) PeriodQueryValues() string {
	q := ""
	if lb.Custom {
		q = "from=" + lb.RequestFrom + "&to=" + lb.To
		if lb.RequestTo != "" {
			q = "from=" + lb.RequestFrom + "&to=" + lb.RequestTo
		}
	} else if lb.RangeParam != "" {
		q = "range=" + lb.RangeParam
	}
	return q
}

func (lb StatsLookback) QueryValues() string {
	q := lb.PeriodQueryValues()
	if lb.ViewRequested != "" && lb.ViewRequested != StatsViewRange {
		if q != "" {
			q += "&"
		}
		q += "view=" + lb.ViewRequested
	} else if lb.View != "" {
		if q != "" {
			q += "&"
		}
		q += "view=" + lb.View
	}
	return q
}

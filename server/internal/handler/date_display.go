package handler

import (
	"fmt"
	"strings"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/repository"
	"customer-support/internal/service"
)

const (
	dateColorHoliday = "#DC2626"
	dateColorSat     = "#2563EB"
	dateBgPublic     = "#FEF2F2"
	dateBgCompany    = "#F3F4F6"
	leaveBadgeAnnual = "#F97316"
	leaveBadgeHalf   = "#FDBA74"
)

// DateTone 한국 달력 날짜 색. 오늘은 휴일 색보다 우선. §23.13.4
type DateTone struct {
	Date        string
	Title       string
	HolidayName string
	HolidayKind string
	IsToday     bool
	IsSunday    bool
	IsSaturday  bool
	DayClass    string
	CellClass   string
	DayStyle    string
	CellStyle   string
}

// LeaveBadge 날짜 칸 아래 연차 뱃지. §23.13.5
type LeaveBadge struct {
	UserID string
	Name   string
	Kind   string
	Text   string
	Mine   bool
	Color  string
	Bold   bool
}

type LeaveBadgeGroup struct {
	Shown  []LeaveBadge
	Hidden []LeaveBadge
	More   int
}

func toneForDate(date, today string) DateTone {
	t := DateTone{Date: date, DayClass: "text-slate-700"}
	parsed, key, ok := parseToneDate(date)
	if !ok {
		return t
	}
	t.Date = key
	wd := parsed.Weekday()
	t.IsSunday = wd == time.Sunday
	t.IsSaturday = wd == time.Saturday
	t.IsToday = key == today

	hit, name := service.IsHoliday(key)
	if hit {
		t.HolidayName = name
		t.Title = name
		// kind from table via calendar names only; classify by... we need kind.
		// IsHoliday doesn't return kind. Look up via extra? Keep name; kind from Get is heavy.
		// Use calendar year cache? Simpler: Get from holiday repo is per-page.
		// For color, public vs company: if we only have name, company holidays are still red+gray.
		// We'll set kind in decorate functions that have the holiday map.
	}

	if t.IsToday {
		t.DayClass = "text-blue-700"
		t.CellClass = "bg-blue-50/40"
		t.DayStyle = "color:#1D4ED8"
		t.CellStyle = "background-color:#EFF6FF"
		if t.Title == "" {
			t.Title = "오늘"
		}
		return t
	}
	if hit {
		t.DayClass = "text-red-600 font-semibold"
		t.CellClass = "bg-red-50"
		t.DayStyle = "color:" + dateColorHoliday
		t.CellStyle = "background-color:" + dateBgPublic
		return t
	}
	if t.IsSunday {
		t.DayClass = "text-red-600"
		t.DayStyle = "color:" + dateColorHoliday
		t.Title = "일요일"
		return t
	}
	if t.IsSaturday {
		t.DayClass = "text-blue-600"
		t.DayStyle = "color:" + dateColorSat
		t.Title = "토요일"
		return t
	}
	return t
}

func applyHolidayKind(t *DateTone, kind string) {
	if t == nil || t.IsToday || strings.TrimSpace(t.HolidayName) == "" {
		return
	}
	t.HolidayKind = strings.TrimSpace(kind)
	if t.HolidayKind == model.HolidayKindCompany {
		t.CellClass = "bg-gray-100"
		t.CellStyle = "background-color:" + dateBgCompany
	} else {
		t.CellClass = "bg-red-50"
		t.CellStyle = "background-color:" + dateBgPublic
	}
	t.DayClass = "text-red-600 font-semibold"
	t.DayStyle = "color:" + dateColorHoliday
}

func parseToneDate(date string) (time.Time, string, bool) {
	date = strings.TrimSpace(date)
	if len(date) >= 10 {
		date = date[:10]
	}
	t, err := time.ParseInLocation(dateLayout, date, time.Local)
	if err != nil {
		return time.Time{}, "", false
	}
	return t, date, true
}

func holidayMapInRange(repo interface {
	DatesInRange(from, toInclusive string) (map[string]bool, error)
}, from, to string) map[string]bool {
	if repo == nil {
		return map[string]bool{}
	}
	m, err := repo.DatesInRange(from, to)
	if err != nil || m == nil {
		return map[string]bool{}
	}
	return m
}

func decorateRegisterColumns(cols []RegisterColumn, today string, byDate map[string]model.Holiday) {
	for i := range cols {
		tn := toneForDate(cols[i].Date, today)
		if h, ok := byDate[cols[i].Date]; ok {
			tn.HolidayName = h.Name
			tn.Title = h.Name
			applyHolidayKind(&tn, h.Kind)
		}
		cols[i].IsToday = tn.IsToday
		cols[i].HolidayName = tn.HolidayName
		cols[i].HolidayKind = tn.HolidayKind
		cols[i].IsSunday = tn.IsSunday
		cols[i].IsSaturday = tn.IsSaturday
		cols[i].DateTitle = tn.Title
		cols[i].DayClass = tn.DayClass
		cols[i].CellClass = tn.CellClass
		cols[i].DayStyle = tn.DayStyle
		cols[i].CellStyle = tn.CellStyle
		cols[i].IsOffDay = !service.IsWorkingDay(cols[i].Date)
	}
}

func decorateRegisterDayColumns(cols []RegisterDayColumn, today string, byDate map[string]model.Holiday) {
	for i := range cols {
		one := []RegisterColumn{cols[i].RegisterColumn}
		decorateRegisterColumns(one, today, byDate)
		cols[i].RegisterColumn = one[0]
	}
}

func decorateMonthWeeks(weeks [][]RegisterMonthDay, today string, byDate map[string]model.Holiday, leaves map[string][]LeaveBadge) {
	for wi := range weeks {
		for di := range weeks[wi] {
			d := &weeks[wi][di]
			if d.Date == "" {
				continue
			}
			tn := toneForDate(d.Date, today)
			if h, ok := byDate[d.Date]; ok {
				tn.HolidayName = h.Name
				tn.Title = h.Name
				applyHolidayKind(&tn, h.Kind)
			}
			d.IsToday = tn.IsToday
			d.IsSunday = tn.IsSunday
			d.IsSaturday = tn.IsSaturday
			d.HolidayName = tn.HolidayName
			d.HolidayKind = tn.HolidayKind
			d.DateTitle = tn.Title
			d.DayClass = tn.DayClass
			d.CellClass = tn.CellClass
			d.DayStyle = tn.DayStyle
			d.CellStyle = tn.CellStyle
			if tn.HolidayName == "" {
				d.Leaves = groupLeaveBadges(leaves[d.Date])
			}
			// 실제 주말만. 공휴일을 주말로 위장하지 않는다.
			d.IsWeekend = tn.IsSunday || tn.IsSaturday
		}
	}
}

func loadHolidays(repo *repository.HolidayRepo, from, to string) map[string]model.Holiday {
	out := map[string]model.Holiday{}
	if repo == nil {
		return out
	}
	y1 := yearOfDate(from)
	y2 := yearOfDate(to)
	if y1 <= 0 {
		y1 = y2
	}
	if y2 <= 0 {
		y2 = y1
	}
	if y1 > y2 {
		y1, y2 = y2, y1
	}
	for y := y1; y <= y2; y++ {
		items, err := repo.ListByYear(y)
		if err != nil {
			continue
		}
		for _, h := range items {
			out[h.Date] = h
		}
	}
	return out
}

func yearOfDate(date string) int {
	t, _, ok := parseToneDate(date)
	if !ok {
		return 0
	}
	return t.Year()
}

func holidayNameMap(repo *repository.HolidayRepo, year int) map[string]string {
	out := map[string]string{}
	if repo == nil || year <= 0 {
		return out
	}
	items, err := repo.ListByYear(year)
	if err != nil {
		return out
	}
	for _, h := range items {
		out[h.Date] = h.Name
	}
	return out
}

func groupLeaveBadges(items []LeaveBadge) LeaveBadgeGroup {
	g := LeaveBadgeGroup{}
	if len(items) <= 3 {
		g.Shown = items
		return g
	}
	g.Shown = items[:3]
	g.Hidden = items[3:]
	g.More = len(items) - 3
	return g
}

func leaveBadgesByDate(items []model.StaffLeave, currentUserID string) map[string][]LeaveBadge {
	out := map[string][]LeaveBadge{}
	for _, it := range items {
		hit, _ := service.IsHoliday(it.Date)
		if hit {
			continue
		}
		color := leaveBadgeAnnual
		if it.Kind != model.LeaveKindAnnual {
			color = leaveBadgeHalf
		}
		mine := it.UserID != "" && it.UserID == currentUserID
		out[it.Date] = append(out[it.Date], LeaveBadge{
			UserID: it.UserID,
			Name:   it.UserName,
			Kind:   it.Kind,
			Text:   model.LeaveBadgeText(it.UserName, it.Kind),
			Mine:   mine,
			Bold:   mine,
			Color:  color,
		})
	}
	return out
}

func decorateMntCalendar(weeks [][]MntCalDay, today string, byDate map[string]model.Holiday, leaves map[string][]LeaveBadge) {
	for wi := range weeks {
		for di := range weeks[wi] {
			d := &weeks[wi][di]
			if d.Date == "" {
				continue
			}
			tn := toneForDate(d.Date, today)
			if h, ok := byDate[d.Date]; ok {
				tn.HolidayName = h.Name
				tn.Title = h.Name
				applyHolidayKind(&tn, h.Kind)
			}
			d.Today = tn.IsToday
			d.IsSunday = tn.IsSunday
			d.IsSaturday = tn.IsSaturday
			d.Weekend = tn.IsSunday || tn.IsSaturday
			d.HolidayName = tn.HolidayName
			d.HolidayKind = tn.HolidayKind
			d.DateTitle = tn.Title
			d.DayClass = tn.DayClass
			d.CellClass = tn.CellClass
			d.DayStyle = tn.DayStyle
			d.CellStyle = tn.CellStyle
			if tn.HolidayName == "" {
				d.Leaves = groupLeaveBadges(leaves[d.Date])
			}
		}
	}
}

func attachColumnLeaves(cols []RegisterColumn, leaves map[string][]LeaveBadge) {
	for i := range cols {
		if cols[i].HolidayName == "" {
			cols[i].Leaves = groupLeaveBadges(leaves[cols[i].Date])
		}
	}
}

func decorateMntWeeks(weeks [][]MntCalDay, today string, holidays *repository.HolidayRepo, leaves *repository.StaffLeaveRepo, uid string) [][]MntCalDay {
	if len(weeks) == 0 {
		return weeks
	}
	from, to := "", ""
	for _, week := range weeks {
		for _, d := range week {
			if d.Date == "" {
				continue
			}
			if from == "" || d.Date < from {
				from = d.Date
			}
			if to == "" || d.Date > to {
				to = d.Date
			}
		}
	}
	holi := loadHolidays(holidays, from, to)
	var items []model.StaffLeave
	if leaves != nil {
		items, _ = leaves.ListInRange(from, to)
	}
	decorateMntCalendar(weeks, today, holi, leaveBadgesByDate(items, uid))
	return weeks
}

func meetingHolidayBanner(date string) string {
	hit, name := service.IsHoliday(date)
	if hit {
		if name == "" {
			name = "휴일"
		}
		return fmt.Sprintf("오늘은 %s입니다", name)
	}
	t, _, ok := parseToneDate(date)
	if !ok {
		return ""
	}
	switch t.Weekday() {
	case time.Sunday:
		return "오늘은 일요일입니다"
	case time.Saturday:
		return "오늘은 토요일입니다"
	default:
		return ""
	}
}

package service

import (
	"strings"
	"sync"
	"time"

	"customer-support/internal/repository"
)

// Calendar holidays 테이블만 보는 휴일 달력. 연도 단위 캐시. §23.13.7
type Calendar struct {
	repo  *repository.HolidayRepo
	mu    sync.RWMutex
	cache map[int]yearCache
}

type yearCache struct {
	epoch int64
	names map[string]string // YYYY-MM-DD → 이름
}

var defaultCal *Calendar

// SetHolidayCalendar 화면·패키지 함수가 쓸 기본 달력. 테이블이 바뀌면 epoch 로 캐시가 비워진다.
func SetHolidayCalendar(c *Calendar) {
	defaultCal = c
}

func DefaultCalendar() *Calendar {
	return defaultCal
}

func HasHolidayCalendar() bool {
	return defaultCal != nil
}

func NewCalendar(repo *repository.HolidayRepo) *Calendar {
	return &Calendar{repo: repo, cache: map[int]yearCache{}}
}

func parseHolidayDate(date string) (time.Time, string, bool) {
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

func (c *Calendar) namesForYear(year int) map[string]string {
	if c == nil {
		return map[string]string{}
	}
	ep := repository.HolidayDataEpoch()
	c.mu.RLock()
	if e, ok := c.cache[year]; ok && e.epoch == ep {
		names := e.names
		c.mu.RUnlock()
		return names
	}
	c.mu.RUnlock()

	names := map[string]string{}
	if c.repo != nil {
		items, err := c.repo.ListByYear(year)
		if err == nil {
			for _, h := range items {
				if h.Date != "" {
					names[h.Date] = h.Name
				}
			}
		}
	}
	c.mu.Lock()
	if c.cache == nil {
		c.cache = map[int]yearCache{}
	}
	c.cache[year] = yearCache{epoch: ep, names: names}
	c.mu.Unlock()
	return names
}

// Invalidate 해당 연도 캐시를 버린다. year<=0 이면 전부.
func (c *Calendar) Invalidate(year int) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if year <= 0 {
		c.cache = map[int]yearCache{}
		return
	}
	delete(c.cache, year)
}

// IsHoliday 공휴일·대체공휴일·회사 휴무일이면 true와 이름. 주말만으로는 true가 아니다. §23.13.7
func (c *Calendar) IsHoliday(date string) (bool, string) {
	t, key, ok := parseHolidayDate(date)
	if !ok {
		return false, ""
	}
	name, hit := c.namesForYear(t.Year())[key]
	return hit, name
}

// IsWorkingDay 평일이고 휴일이 아니면 true. §23.13.7
func (c *Calendar) IsWorkingDay(date string) bool {
	t, key, ok := parseHolidayDate(date)
	if !ok {
		return false
	}
	if t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
		return false
	}
	if _, hit := c.namesForYear(t.Year())[key]; hit {
		return false
	}
	return true
}

// WorkingDaysBetween from~to 포함 근무일 수. §23.13.7
func (c *Calendar) WorkingDaysBetween(from, to string) int {
	start, _, ok := parseHolidayDate(from)
	if !ok {
		return 0
	}
	end, _, ok := parseHolidayDate(to)
	if !ok {
		return 0
	}
	if end.Before(start) {
		return 0
	}
	n := 0
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		if c.IsWorkingDay(d.Format("2006-01-02")) {
			n++
		}
	}
	return n
}

// NextWorkingDay date 가 근무일이면 그대로, 아니면 이후 첫 근무일. 판정은 IsWorkingDay 만 쓴다.
func (c *Calendar) NextWorkingDay(date string) string {
	if c == nil {
		c = &Calendar{}
	}
	t, _, ok := parseHolidayDate(date)
	if !ok {
		return ""
	}
	for i := 0; i < 366; i++ {
		ds := t.Format("2006-01-02")
		if c.IsWorkingDay(ds) {
			return ds
		}
		t = t.AddDate(0, 0, 1)
	}
	return ""
}

// PrevWorkingDay date 가 근무일이면 그대로, 아니면 이전 첫 근무일. 판정은 IsWorkingDay 만 쓴다.
func (c *Calendar) PrevWorkingDay(date string) string {
	if c == nil {
		c = &Calendar{}
	}
	t, _, ok := parseHolidayDate(date)
	if !ok {
		return ""
	}
	for i := 0; i < 366; i++ {
		ds := t.Format("2006-01-02")
		if c.IsWorkingDay(ds) {
			return ds
		}
		t = t.AddDate(0, 0, -1)
	}
	return ""
}

// LastWorkingDayOfMonth 해당 달의 마지막 근무일. 판정은 IsWorkingDay 만 쓴다. §33.4
func (c *Calendar) LastWorkingDayOfMonth(year int, month time.Month) string {
	if c == nil {
		c = &Calendar{}
	}
	if month < 1 {
		month = 1
	}
	last := time.Date(year, month+1, 0, 0, 0, 0, 0, time.Local)
	for i := 0; i < 31; i++ {
		ds := last.Format("2006-01-02")
		if c.IsWorkingDay(ds) {
			return ds
		}
		last = last.AddDate(0, 0, -1)
	}
	return ""
}

func IsHoliday(date string) (bool, string) {
	return defaultCal.IsHoliday(date)
}

func IsWorkingDay(date string) bool {
	return defaultCal.IsWorkingDay(date)
}

func WorkingDaysBetween(from, to string) int {
	return defaultCal.WorkingDaysBetween(from, to)
}

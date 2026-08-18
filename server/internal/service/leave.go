package service

import (
	"fmt"
	"strings"
)

// ExpandLeaveWorkingDates from~to 포함 근무일만. 주말·공휴일은 건너뛴다. §23.13.3
func ExpandLeaveWorkingDates(from, to string) ([]string, error) {
	start, _, ok := parseHolidayDate(from)
	if !ok {
		return nil, fmt.Errorf("시작일을 입력하세요")
	}
	end := start
	if strings.TrimSpace(to) != "" {
		var okTo bool
		end, _, okTo = parseHolidayDate(to)
		if !okTo {
			return nil, fmt.Errorf("종료일이 올바르지 않습니다")
		}
	}
	if end.Before(start) {
		return nil, fmt.Errorf("기간이 올바르지 않습니다")
	}
	cal := DefaultCalendar()
	if cal == nil {
		cal = NewCalendar(nil)
	}
	var out []string
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		ds := d.Format("2006-01-02")
		if !cal.IsWorkingDay(ds) {
			continue
		}
		out = append(out, ds)
	}
	return out, nil
}

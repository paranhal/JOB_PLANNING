package leavecalc

import (
	"fmt"
	"strconv"
	"strings"
)

// DeductMinutes is the ledger minutes for one request (0 if type does not deduct).
// day types: businessDays * work_minutes_per_day
// half: HalfDayMinutes (240 when work day is 480)
// early_leave/outing: clockMinutes (조퇴 3h = 180)
func DeductMinutes(typeKey string, businessDays, clockMinutes int, policy Policy) (int, error) {
	policy = policy.normalized()
	td, ok := TypeByKey(typeKey)
	if !ok {
		return 0, fmt.Errorf("unknown leave type %q", typeKey)
	}
	if !td.DeductsBalance {
		return 0, nil
	}
	switch typeKey {
	case TypeHalf:
		return policy.HalfDayMinutes(), nil
	case TypeEarlyLeave, TypeOuting:
		if clockMinutes < 0 {
			return 0, fmt.Errorf("clock minutes must be >= 0")
		}
		return clockMinutes, nil
	default:
		if businessDays < 0 {
			return 0, fmt.Errorf("business days must be >= 0")
		}
		return businessDays * policy.WorkMinutesPerDay, nil
	}
}

// HoursToMinutes converts whole or fractional hours to minutes (3h → 180).
func HoursToMinutes(hours float64) int {
	return int(hours*60 + 0.5)
}

// ClockRangeMinutes is minutes from HH:MM to HH:MM on the same day.
func ClockRangeMinutes(startHHMM, endHHMM string) (int, error) {
	s, err := parseHHMM(startHHMM)
	if err != nil {
		return 0, err
	}
	e, err := parseHHMM(endHHMM)
	if err != nil {
		return 0, err
	}
	if e <= s {
		return 0, fmt.Errorf("end time must be after start")
	}
	return e - s, nil
}

func parseHHMM(s string) (int, error) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid time %q", s)
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil || h < 0 || h > 23 {
		return 0, fmt.Errorf("invalid time %q", s)
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil || m < 0 || m > 59 {
		return 0, fmt.Errorf("invalid time %q", s)
	}
	return h*60 + m, nil
}

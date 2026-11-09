package leaveapp

import (
	"fmt"
	"strings"

	"customer-support/internal/hwpx"
	"customer-support/internal/service/leavecalc"
)

func PeriodModeFor(typeKey string) string {
	switch typeKey {
	case leavecalc.TypeHalf, leavecalc.TypeEarlyLeave, leavecalc.TypeOuting, leavecalc.TypeOther:
		return hwpx.LeavePeriodHours
	default:
		return hwpx.LeavePeriodDays
	}
}

func Normalize(in Input) (Input, error) {
	in.TypeKey = strings.TrimSpace(in.TypeKey)
	in.PeriodMode = PeriodModeFor(in.TypeKey)
	in.StartDate = strings.TrimSpace(in.StartDate)
	in.EndDate = strings.TrimSpace(in.EndDate)
	in.StartTime = strings.TrimSpace(in.StartTime)
	in.EndTime = strings.TrimSpace(in.EndTime)
	in.HalfPart = strings.TrimSpace(in.HalfPart)
	if in.StartDate == "" {
		return in, fmt.Errorf("기간을 입력하세요")
	}
	if in.EndDate == "" {
		in.EndDate = in.StartDate
	}
	switch in.PeriodMode {
	case hwpx.LeavePeriodHours:
		in.SwapDate, in.SwapState = "", ""
		if in.TypeKey == leavecalc.TypeHalf {
			st, en, part, err := leavecalc.ResolveHalf(in.StartTime)
			if err != nil {
				return in, err
			}
			in.StartTime, in.EndTime, in.HalfPart = st, en, part
		} else if in.TypeKey == leavecalc.TypeEarlyLeave || in.TypeKey == leavecalc.TypeOuting {
			in.HalfPart = ""
			if in.StartTime == "" || in.EndTime == "" {
				return in, fmt.Errorf("시작·종료 시각을 입력하세요")
			}
			if _, err := leavecalc.ClockWorkMinutes(in.StartTime, in.EndTime); err != nil {
				return in, err
			}
		}
	case hwpx.LeavePeriodDays:
		in.StartTime, in.EndTime, in.HalfPart = "", "", ""
		if in.StartDate > in.EndDate {
			return in, fmt.Errorf("기간이 올바르지 않습니다")
		}
	}
	return in, nil
}

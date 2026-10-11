// Package leavecalc implements hire-date annual-leave grants, FIFO ledger
// math, type deduction policy, document numbers, and business-day counts.
// docs/leave-and-approval-spec.md v0.4 §3–§4, §7.1–§7.2.
//
// No database, HTTP, or screens. Fiscal-year grants are out of scope.
package leavecalc

// Statutory / company policy constants (adjustable; not hardcoded in formulas).
const (
	DefaultWorkMinutesPerDay   = 480
	DefaultHalfDayMinutes      = 240 // D-18: 1일=8시간, 반차=4시간
	DefaultEarlyLeaveMinutes   = 180 // 조퇴 3시간 예시(Q-07). 시각이 있으면 그 분을 쓴다.

	MonthlyGrantDays        = 1
	MonthlyMaxOccurrences   = 11
	MonthlyExpireAfterYears = 1 // 월 발생분 만료 = H + 이 연수 (정책 상수)
	YearlyBaseDays          = 15
	YearlyExtraEveryYears   = 2 // floor((y-1)/2)
	YearlyMaxDays           = 25
	YearlyExpireAfterYears  = 1 // 연 발생분 만료 = 부여일 + 이 연수
	DefaultDocPrefix        = "VI"
	DocKindAttendance       = "근태"
	GrantKeyMonthlyPrefix   = "M:"
	GrantKeyYearlyPrefix    = "Y:"
	SourceAutoMonthly       = "auto_monthly"
	SourceAutoYearly        = "auto_yearly"
	KindUse                 = "use"
	KindRestore             = "restore"
	KindExpire              = "expire"
	UnitDay                 = "day"
	UnitTime                = "time"
)

// Type keys (§3). 대체근무는 스펙 표에 type_key가 없어 substitute 로 둔다.
const (
	TypeAnnual     = "annual"
	TypeHalf       = "half"
	TypeMonthly    = "monthly"
	TypeVacation   = "vacation"
	TypeEarlyLeave = "early_leave"
	TypeOuting     = "outing"
	TypeAbsence    = "absence"
	TypeOther      = "other"
	TypeSubstitute = "substitute"
)

// TypeDef is in-memory type policy. Not a DB table.
type TypeDef struct {
	Key            string
	Label          string
	Unit           string
	DeductsBalance bool
}

// Types returns §3 type policy in sort order.
func Types() []TypeDef {
	return []TypeDef{
		{TypeAnnual, "연차", UnitDay, true},
		{TypeHalf, "반차", UnitTime, true},
		{TypeMonthly, "월차", UnitDay, true},
		{TypeVacation, "휴가", UnitDay, true},
		{TypeEarlyLeave, "조퇴", UnitTime, true},
		{TypeOuting, "외출", UnitTime, true},
		{TypeAbsence, "결근", UnitDay, false},
		{TypeOther, "기타", UnitTime, false},
		{TypeSubstitute, "대체근무", UnitDay, false},
	}
}

// TypeByKey looks up §3 policy. ok=false if unknown.
func TypeByKey(key string) (TypeDef, bool) {
	for _, t := range Types() {
		if t.Key == key {
			return t, true
		}
	}
	return TypeDef{}, false
}

// DeductsBalance reports whether type_key reduces the annual ledger.
func DeductsBalance(typeKey string) bool {
	t, ok := TypeByKey(typeKey)
	return ok && t.DeductsBalance
}

// Policy is calculation input. Zero fields fall back to named constants.
type Policy struct {
	WorkMinutesPerDay       int
	LedgerStartDate         string // YYYY-MM-DD; empty = no filter
	MonthlyMaxOccurrences   int
	MonthlyExpireAfterYears int
	YearlyBaseDays          int
	YearlyExtraEveryYears   int
	YearlyMaxDays           int
	YearlyExpireAfterYears  int
	DocPrefix               string
}

// DefaultPolicy returns hire-date policy with statutory named defaults.
func DefaultPolicy() Policy {
	return Policy{
		WorkMinutesPerDay:       DefaultWorkMinutesPerDay,
		MonthlyMaxOccurrences:   MonthlyMaxOccurrences,
		MonthlyExpireAfterYears: MonthlyExpireAfterYears,
		YearlyBaseDays:          YearlyBaseDays,
		YearlyExtraEveryYears:   YearlyExtraEveryYears,
		YearlyMaxDays:           YearlyMaxDays,
		YearlyExpireAfterYears:  YearlyExpireAfterYears,
		DocPrefix:               DefaultDocPrefix,
	}
}

func (p Policy) normalized() Policy {
	d := DefaultPolicy()
	if p.WorkMinutesPerDay <= 0 {
		p.WorkMinutesPerDay = d.WorkMinutesPerDay
	}
	if p.MonthlyMaxOccurrences <= 0 {
		p.MonthlyMaxOccurrences = d.MonthlyMaxOccurrences
	}
	if p.MonthlyExpireAfterYears <= 0 {
		p.MonthlyExpireAfterYears = d.MonthlyExpireAfterYears
	}
	if p.YearlyBaseDays <= 0 {
		p.YearlyBaseDays = d.YearlyBaseDays
	}
	if p.YearlyExtraEveryYears <= 0 {
		p.YearlyExtraEveryYears = d.YearlyExtraEveryYears
	}
	if p.YearlyMaxDays <= 0 {
		p.YearlyMaxDays = d.YearlyMaxDays
	}
	if p.YearlyExpireAfterYears <= 0 {
		p.YearlyExpireAfterYears = d.YearlyExpireAfterYears
	}
	if p.DocPrefix == "" {
		p.DocPrefix = d.DocPrefix
	}
	return p
}

// HalfDayMinutes is 반차 차감 분. 기본 480/2=240.
func (p Policy) HalfDayMinutes() int {
	p = p.normalized()
	return p.WorkMinutesPerDay / 2
}

// YearlyGrantDays is min(25, 15 + floor((y-1)/2)) for completed tenure y≥1.
func YearlyGrantDays(completedTenureYears int) int {
	return yearlyGrantDays(completedTenureYears, DefaultPolicy())
}

func yearlyGrantDays(y int, p Policy) int {
	p = p.normalized()
	if y < 1 {
		return 0
	}
	days := p.YearlyBaseDays + (y-1)/p.YearlyExtraEveryYears
	if days > p.YearlyMaxDays {
		return p.YearlyMaxDays
	}
	return days
}

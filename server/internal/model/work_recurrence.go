package model

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	RecurrenceRoleParent     = "parent"
	RecurrenceRoleOccurrence = "occurrence"

	RecurrenceDaily       = "daily"
	RecurrenceWeeklyDOW   = "weekly_dow"
	RecurrenceEveryNDays  = "every_n_days"
	RecurrenceEveryNWeeks = "every_n_weeks"
	RecurrenceManual      = "manual"
	RecurrenceNone        = "none"

	HolidayPolicyAsIs        = "as_is"
	HolidayPolicyNextWorkday = "next_workday"
	HolidayPolicyPrevWorkday = "prev_workday"
	HolidayPolicySkip        = "skip"

	CompletePolicyAuto          = "auto"
	CompletePolicyManual        = "manual"
	CompletePolicyRequireResult = "require_result"

	OccurrenceScheduled  = "scheduled"
	OccurrenceInProgress = "in_progress"
	OccurrenceComplete   = "complete"
	OccurrenceOverdue    = "overdue"
	OccurrenceDeferred   = "deferred"
	OccurrenceSkipped    = "skipped"

	RecurrenceWarnLimit   = 200
	RecurrenceRejectLimit = 500
	RecurrenceDailyYearN  = 365
)

// WorkRecurrence 상위 업무 1건의 기간 내 반복 규칙 (§13.15.4)
type WorkRecurrence struct {
	TaskID                 string
	StartDate              string
	EndDate                string
	RuleType               string
	IntervalN              int
	Weekdays               string
	HolidayPolicy          string
	CompletePolicy         string
	ProgressIncludeFuture  bool
	FinalResult            string
	ManualDates            []string // 폼 전용. DB에 안 넣는다.
}

type OccurrencePreviewItem struct {
	Date         string
	Label        string
	RawDate      string
	HolidayName  string
	Moved        bool
	Skipped      bool
	Note         string
}

type OccurrencePreview struct {
	Items          []OccurrencePreviewItem
	Dates          []string
	Count          int
	Notes          []string
	HolidayBanners []string
	Warn200        bool
	Reject500      bool
	RejectYear     bool
	Error          string
}

// OccurrenceGenerateResult 생성·재생성 건수 보고. §13.15.3
type OccurrenceGenerateResult struct {
	Created        int
	KeptComplete   int
	Deleted        int
	ParentMismatch int
}

func (r OccurrenceGenerateResult) Report() string {
	return fmt.Sprintf("생성 %d건 · 완료 유지 %d건 · 미완료 삭제 %d건 · 부모연결 불일치 %d건",
		r.Created, r.KeptComplete, r.Deleted, r.ParentMismatch)
}

func NormalizeRecurrenceRuleType(s string) string {
	switch strings.TrimSpace(s) {
	case RecurrenceDaily, RecurrenceWeeklyDOW, RecurrenceEveryNDays, RecurrenceEveryNWeeks, RecurrenceManual:
		return s
	default:
		return RecurrenceNone
	}
}

func NormalizeHolidayPolicy(s string) string {
	switch strings.TrimSpace(s) {
	case HolidayPolicyNextWorkday, HolidayPolicyPrevWorkday, HolidayPolicySkip:
		return s
	default:
		return HolidayPolicyAsIs
	}
}

func NormalizeCompletePolicy(s string) string {
	switch strings.TrimSpace(s) {
	case CompletePolicyAuto, CompletePolicyRequireResult:
		return s
	default:
		return CompletePolicyManual
	}
}

func RecurrenceRuleLabel(s string) string {
	switch NormalizeRecurrenceRuleType(s) {
	case RecurrenceDaily:
		return "매일"
	case RecurrenceWeeklyDOW:
		return "매주 요일"
	case RecurrenceEveryNDays:
		return "N일마다"
	case RecurrenceEveryNWeeks:
		return "N주마다"
	case RecurrenceManual:
		return "날짜 지정"
	default:
		return "없음"
	}
}

func HolidayPolicyLabel(s string) string {
	switch NormalizeHolidayPolicy(s) {
	case HolidayPolicyNextWorkday:
		return "다음 근무일"
	case HolidayPolicyPrevWorkday:
		return "이전 근무일"
	case HolidayPolicySkip:
		return "제외"
	default:
		return "그대로 실행"
	}
}

func OccurrenceStatusLabel(s string) string {
	switch strings.TrimSpace(s) {
	case OccurrenceInProgress:
		return "진행중"
	case OccurrenceComplete:
		return "완료"
	case OccurrenceOverdue:
		return "미완료"
	case OccurrenceDeferred:
		return "연기"
	case OccurrenceSkipped:
		return "제외"
	default:
		return "예정"
	}
}

func ParseWeekdays(s string) map[int]bool {
	out := map[int]bool{}
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 7 {
			continue
		}
		out[n] = true
	}
	return out
}

func JoinWeekdays(values []string) string {
	seen := map[int]bool{}
	var nums []int
	for _, v := range values {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || n < 1 || n > 7 || seen[n] {
			continue
		}
		seen[n] = true
		nums = append(nums, n)
	}
	sort.Ints(nums)
	parts := make([]string, len(nums))
	for i, n := range nums {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ",")
}

func ISOWeekday(t time.Time) int {
	d := int(t.Weekday())
	if d == 0 {
		return 7
	}
	return d
}

func FormatOccurrenceLabel(date string) string {
	t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(date), time.Local)
	if err != nil {
		return date
	}
	names := []string{"일", "월", "화", "수", "목", "금", "토"}
	return fmt.Sprintf("%d/%d(%s)", int(t.Month()), t.Day(), names[int(t.Weekday())])
}

func ParseDateList(raw string) []string {
	raw = strings.ReplaceAll(raw, ",", "\n")
	var out []string
	seen := map[string]bool{}
	for _, line := range strings.Split(raw, "\n") {
		d := strings.TrimSpace(line)
		if len(d) >= 10 {
			d = d[:10]
		}
		if _, err := time.ParseInLocation("2006-01-02", d, time.Local); err != nil {
			continue
		}
		if seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, d)
	}
	return out
}

func (p OccurrencePreview) SummaryLine() string {
	if p.Count <= 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "실행 예정일 %d건이 생성됩니다.", p.Count)
	max := 12
	for i, d := range p.Dates {
		if i == 0 {
			b.WriteByte('\n')
		} else {
			b.WriteByte(' ')
		}
		b.WriteString(FormatOccurrenceLabel(d))
		if i+1 >= max && len(p.Dates) > max {
			b.WriteString(" …")
			break
		}
	}
	return b.String()
}

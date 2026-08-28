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
	RecurrenceMonthly     = "monthly"
	RecurrenceQuarterly   = "quarterly"
	RecurrenceYearly      = "yearly"
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

	RecurrenceChangeFuture  = "future"   // 미완료 미래만 다시 생성 (기본)
	RecurrenceChangeKeepAdd = "keep_add" // 기존 유지 + 새 일정만 추가
	RecurrenceChangeReplace = "replace"  // 모든 미완료 삭제 후 재생성

	RecurrenceWarnLimit    = 200
	RecurrenceRejectLimit  = 500
	RecurrenceDailyYearN   = 365
	RecurrenceUpcomingDays = 3 // 대시보드 「다가오는 실행」 미리보기 일수. 당일은 제외(오늘 예정).
)

// WorkRecurrence 상위 업무 1건의 기간 내 반복 규칙 (§13.15.4)
type WorkRecurrence struct {
	TaskID                string
	StartDate             string
	EndDate               string
	RuleType              string
	IntervalN             int
	Weekdays              string
	HolidayPolicy         string
	CompletePolicy        string
	ProgressIncludeFuture bool
	FinalResult           string
	Archived              bool     // 보관. 완료 회차를 지우지 않고 목록에서만 내린다.
	MonthDay              int      // 매월 N일 · 매년 일 · 분기 일 (1–31)
	MonthN                int      // 매년 월 · 분기 시작월 (1–12)
	LastWorkday           bool     // 매월 마지막 영업일
	ManualDates           []string // 지정일자. work_recurrence.manual_dates
}

type OccurrencePreviewItem struct {
	Date        string
	Label       string
	RawDate     string
	HolidayName string
	Moved       bool
	Skipped     bool
	Note        string
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
	ChangeMode     string
	Duplicates     []string
	ChangeNotes    []string
	WillCreate     int
	WillDelete     int
	KeptComplete   int
	KeptOpenPast   int
}

// OccurrenceGenerateResult 생성·재생성 건수 보고. §13.15.3
type OccurrenceGenerateResult struct {
	Created        int
	KeptComplete   int
	KeptPast       int
	Deleted        int
	ParentMismatch int
}

func (r OccurrenceGenerateResult) Report() string {
	return fmt.Sprintf("생성 %d건 · 완료 유지 %d건 · 지난 일정 유지 %d건 · 미완료 삭제 %d건 · 부모연결 불일치 %d건",
		r.Created, r.KeptComplete, r.KeptPast, r.Deleted, r.ParentMismatch)
}

func NormalizeRecurrenceChangeMode(s string) string {
	switch strings.TrimSpace(s) {
	case RecurrenceChangeKeepAdd, RecurrenceChangeReplace:
		return s
	default:
		return RecurrenceChangeFuture
	}
}

func RecurrenceChangeModeLabel(s string) string {
	switch NormalizeRecurrenceChangeMode(s) {
	case RecurrenceChangeKeepAdd:
		return "기존 유지 + 새 일정만 추가"
	case RecurrenceChangeReplace:
		return "모든 미완료 일정 삭제 후 재생성"
	default:
		return "미완료 미래 일정만 다시 생성"
	}
}

func OccurrenceProtected(t WorkTask) bool {
	return OccurrenceDone(t) || OccurrenceSkippedStatus(t)
}

func occurrenceDate(t WorkTask) string {
	d := strings.TrimSpace(t.WorkDate)
	if d == "" {
		d = strings.TrimSpace(t.DueDate)
	}
	return d
}

func KeepOccurrenceOnChange(t WorkTask, mode, today string) bool {
	if t.RecurrenceRole != RecurrenceRoleOccurrence {
		return true
	}
	if OccurrenceProtected(t) {
		return true
	}
	switch NormalizeRecurrenceChangeMode(mode) {
	case RecurrenceChangeKeepAdd:
		return true
	case RecurrenceChangeFuture:
		d := occurrenceDate(t)
		return d != "" && today != "" && d <= today
	default:
		return false
	}
}

// AnnotateOccurrenceChange 일정 변경 미리보기. 쓰기는 하지 않는다. §13.15.8
func AnnotateOccurrenceChange(p *OccurrencePreview, children []WorkTask, mode, today string) {
	if p == nil {
		return
	}
	mode = NormalizeRecurrenceChangeMode(mode)
	today = strings.TrimSpace(today)
	p.ChangeMode = mode
	remain := map[string]bool{}
	exist := map[string]bool{}
	p.WillDelete, p.KeptComplete, p.KeptOpenPast = 0, 0, 0
	for _, ch := range children {
		if ch.RecurrenceRole != RecurrenceRoleOccurrence {
			continue
		}
		d := occurrenceDate(ch)
		if d != "" {
			exist[d] = true
		}
		if KeepOccurrenceOnChange(ch, mode, today) {
			if d != "" {
				remain[d] = true
			}
			if OccurrenceDone(ch) {
				p.KeptComplete++
			} else if !OccurrenceSkippedStatus(ch) {
				p.KeptOpenPast++
			}
			continue
		}
		p.WillDelete++
	}
	var dups, create []string
	for _, d := range p.Dates {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		if exist[d] {
			dups = append(dups, d)
		}
		if !remain[d] {
			create = append(create, d)
		}
	}
	p.Duplicates = dups
	p.WillCreate = len(create)
	var notes []string
	switch mode {
	case RecurrenceChangeKeepAdd:
		notes = append(notes, fmt.Sprintf("기존 일정은 그대로 두고 없는 날짜만 추가합니다. 추가 %d건.", p.WillCreate))
		if len(dups) > 0 {
			notes = append(notes, "중복 "+strconv.Itoa(len(dups))+"건은 이미 있어 추가하지 않습니다. "+joinOccurrenceLabels(dups, 8))
		}
	case RecurrenceChangeReplace:
		notes = append(notes, fmt.Sprintf("완료·제외 %d건은 그대로 둡니다. 미완료 %d건을 지우고 다시 만듭니다(추가 %d건).",
			p.KeptComplete, p.WillDelete, p.WillCreate))
	default:
		notes = append(notes, fmt.Sprintf("지난 일정 %d건·완료 %d건은 그대로 둡니다. 미완료 미래 %d건을 지우고 다시 만듭니다(추가 %d건).",
			p.KeptOpenPast, p.KeptComplete, p.WillDelete, p.WillCreate))
	}
	p.ChangeNotes = notes
}

func joinOccurrenceLabels(dates []string, max int) string {
	var b strings.Builder
	for i, d := range dates {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(FormatOccurrenceLabel(d))
		if i+1 >= max && len(dates) > max {
			b.WriteString(" …")
			break
		}
	}
	return b.String()
}

func NormalizeRecurrenceRuleType(s string) string {
	switch strings.TrimSpace(s) {
	case RecurrenceDaily, RecurrenceWeeklyDOW, RecurrenceEveryNDays, RecurrenceEveryNWeeks,
		RecurrenceMonthly, RecurrenceQuarterly, RecurrenceYearly, RecurrenceManual:
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
	case RecurrenceMonthly:
		return "매월"
	case RecurrenceQuarterly:
		return "분기"
	case RecurrenceYearly:
		return "매년"
	case RecurrenceManual:
		return "지정일자"
	default:
		return "없음"
	}
}

func RecurrenceCycleLabel(w WorkRecurrence) string {
	n := w.IntervalN
	if n < 1 {
		n = 1
	}
	switch NormalizeRecurrenceRuleType(w.RuleType) {
	case RecurrenceDaily:
		return "매일"
	case RecurrenceWeeklyDOW:
		return "매주 요일"
	case RecurrenceEveryNDays:
		return fmt.Sprintf("%d일마다", n)
	case RecurrenceEveryNWeeks:
		return fmt.Sprintf("%d주마다", n)
	case RecurrenceMonthly:
		if w.LastWorkday {
			return "매월 마지막 영업일"
		}
		d := w.MonthDay
		if d < 1 {
			d = n
		}
		if d < 1 {
			d = 1
		}
		return fmt.Sprintf("매월 %d일", d)
	case RecurrenceQuarterly:
		m := w.MonthN
		if m < 1 || m > 12 {
			m = 1
		}
		return fmt.Sprintf("분기(%d월 시작)", m)
	case RecurrenceYearly:
		m, d := w.MonthN, w.MonthDay
		if m < 1 || m > 12 {
			m = 1
		}
		if d < 1 {
			d = 1
		}
		return fmt.Sprintf("매년 %d월 %d일", m, d)
	case RecurrenceManual:
		return "지정일자"
	default:
		return RecurrenceRuleLabel(w.RuleType)
	}
}

// FormatRecurrenceProgress 주간보고 완료 부기. 「정기 데이터 확인 · 5회 중 4회」 §13.15.9
func FormatRecurrenceProgress(title string, total, done int) string {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "반복 업무"
	}
	if total <= 0 {
		return title
	}
	return fmt.Sprintf("%s · %d회 중 %d회", title, total, done)
}

// FormatRecurrenceTimes 전사 주간보고 F열. 「·정기 데이터 확인 5회」 §13.15.9 · §16.6
func FormatRecurrenceTimes(title string, n int) string {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "반복 업무"
	}
	if n <= 1 {
		return "·" + title
	}
	return fmt.Sprintf("·%s %d회", title, n)
}

// DaysUntil today 기준 date까지 남은 일수. 당일 0, 지난 날은 음수.
func DaysUntil(date, today string) (int, bool) {
	d, err1 := time.ParseInLocation("2006-01-02", strings.TrimSpace(date), time.Local)
	t, err2 := time.ParseInLocation("2006-01-02", strings.TrimSpace(today), time.Local)
	if err1 != nil || err2 != nil {
		return 0, false
	}
	return int(d.Sub(t).Hours() / 24), true
}

// DueUrgency 마감 D-n. 3일 이내 soon, 경과 over. §13.15.12
func DueUrgency(days int, ok bool) string {
	if !ok {
		return ""
	}
	if days < 0 {
		return "over"
	}
	if days <= RecurrenceUpcomingDays {
		return "soon"
	}
	return ""
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

func JoinDateList(dates []string) string {
	return strings.Join(ParseDateList(strings.Join(dates, "\n")), "\n")
}

func ClampMonthDay(year int, month time.Month, day int) time.Time {
	if month < 1 {
		month = 1
	}
	if day < 1 {
		day = 1
	}
	last := time.Date(year, month+1, 0, 0, 0, 0, 0, time.Local).Day()
	if day > last {
		day = last
	}
	return time.Date(year, month, day, 0, 0, 0, 0, time.Local)
}

func QuarterMonths(startMonth int) []int {
	if startMonth < 1 || startMonth > 12 {
		startMonth = 1
	}
	out := make([]int, 4)
	for i := 0; i < 4; i++ {
		out[i] = ((startMonth-1+i*3)%12)+1
	}
	return out
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

func NormalizeOccurrenceStatus(s string) string {
	switch strings.TrimSpace(s) {
	case OccurrenceInProgress, OccurrenceComplete, OccurrenceOverdue, OccurrenceDeferred, OccurrenceSkipped:
		return s
	default:
		return OccurrenceScheduled
	}
}

func OccurrenceDone(t WorkTask) bool {
	return t.Status == WBTaskComplete || t.OccurrenceStatus == OccurrenceComplete
}

func OccurrenceSkippedStatus(t WorkTask) bool {
	return t.OccurrenceStatus == OccurrenceSkipped
}

func OccurrenceOpen(t WorkTask) bool {
	return !OccurrenceDone(t) && !OccurrenceSkippedStatus(t)
}

func OccurrenceDueBy(t WorkTask, today string) bool {
	d := strings.TrimSpace(t.WorkDate)
	if d == "" {
		d = strings.TrimSpace(t.DueDate)
	}
	return d != "" && d <= today
}

// OccurrenceProgress 실행 작업 진행률 (§13.15.11)
type OccurrenceProgress struct {
	Complete   int
	Overdue    int
	Scheduled  int
	InProgress int
	Deferred   int
	Skipped    int
	Open       int
	Target     int
	Percent    int
	Has        bool
}

func CalcOccurrenceProgress(items []WorkTask, includeFuture bool, today string) OccurrenceProgress {
	today = strings.TrimSpace(today)
	var p OccurrenceProgress
	for _, t := range items {
		if t.RecurrenceRole != RecurrenceRoleOccurrence {
			continue
		}
		st := NormalizeOccurrenceStatus(t.OccurrenceStatus)
		if OccurrenceDone(t) {
			st = OccurrenceComplete
		}
		switch st {
		case OccurrenceComplete:
			p.Complete++
		case OccurrenceOverdue:
			p.Overdue++
		case OccurrenceInProgress:
			p.InProgress++
		case OccurrenceDeferred:
			p.Deferred++
		case OccurrenceSkipped:
			p.Skipped++
		default:
			p.Scheduled++
		}
		if OccurrenceOpen(t) {
			p.Open++
		}
		if OccurrenceSkippedStatus(t) {
			continue
		}
		if !includeFuture && !OccurrenceDueBy(t, today) {
			continue
		}
		p.Target++
	}
	if p.Target > 0 {
		done := 0
		for _, t := range items {
			if t.RecurrenceRole != RecurrenceRoleOccurrence || OccurrenceSkippedStatus(t) {
				continue
			}
			if !includeFuture && !OccurrenceDueBy(t, today) {
				continue
			}
			if OccurrenceDone(t) {
				done++
			}
		}
		p.Percent = done * 100 / p.Target
		p.Has = true
	}
	return p
}

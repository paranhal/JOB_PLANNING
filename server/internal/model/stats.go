package model

import (
	"fmt"
	"time"
)

// StatsRow 통계 상세 리스트 1행
type StatsRow struct {
	ProductCategory string // 제품분류
	ProductModel    string // 제품모델명
	WorkForm        string // 업무형태
	ReceiptForm     string // 접수형태
	CustomerName    string // 거래처명
	Assignee        string // 수행담당자
	Symptom         string // 접수내용
	ActionTaken     string // 조치내용
	Status          string // 처리상태(코드)
	StatusLabel     string // 처리상태(표시)
	ASID            string
	ASNumber        string
	ReceiptDate     string
	VisitDate       string // 방문(조치시작)일
	CompleteDate    string // 완료일(완료 목록용)
	DurationMin     int
	DurationLabel   string
	VisitRounds     int
	Urgency         string
	Priority        string
	Href            string // 상세 링크
}

// StatsAssigneeRow 담당자별 집계 1행
type StatsAssigneeRow struct {
	Assignee string
	Count    int
}

// StatsAssigneeGroup 담당자별 상세 그룹 (사람별 리스트)
type StatsAssigneeGroup struct {
	Assignee string
	Count    int
	Rows     []StatsRow
}

// StatsMetric 통계 종류
const (
	StatsMetricProgress  = "progress"  // 업무진행(미완료)
	StatsMetricCompleted = "completed" // 완료업무
	StatsMetricReceived  = "received"  // 접수업무
	StatsMetricOverdue   = "overdue"   // 지연(접수+3일)
)

// StatsScope 집계 범위
const (
	StatsScopeTeam     = "team"      // 도서관사업팀 전체
	StatsScopeAssignee = "assignee"  // 담당자별
	StatsScopeWorkType = "work_type" // 업무구분별(AS·정기점검·행정) — 하위호환
	StatsScopeProduct  = "product"   // 업무별(앤로보틱스·KLAS 등)
)

// 중요 KPI 기준값 (사업팀 관리 목표)
const (
	StatsExecTargetPct      = 90.0 // 계획대비 실행율 목표(%)
	StatsVisitTargetDays    = 3.0  // 접수→방문(조치시작) 목표(일)
	StatsCompleteTargetDays = 7.0  // 접수→조치완료 목표(일, 1주일)
	StatsDurationOverMin    = 60   // 최장 소요 시간 기준(분)
	StatsPlanTargetPct      = 95.0 // 계획 수립률 목표(%). §8.4
)

// StatsLongestTopN 기간 건수에 따른 최장 소요 표시 건수. 기본 1, 10건↑ 2, 20건↑ 3.
func StatsLongestTopN(periodCount int) int {
	if periodCount >= 20 {
		return 3
	}
	if periodCount >= 10 {
		return 2
	}
	return 1
}

// FormatStatsDuration 분 → "2시간 15분"
func FormatStatsDuration(min int) string {
	if min < 0 {
		min = 0
	}
	h := min / 60
	m := min % 60
	switch {
	case h > 0 && m > 0:
		return fmt.Sprintf("%d시간 %d분", h, m)
	case h > 0:
		return fmt.Sprintf("%d시간", h)
	default:
		return fmt.Sprintf("%d분", min)
	}
}

// StatsPeriod 기간 단위
const (
	StatsPeriodDay     = "day"
	StatsPeriodWeek    = "week"
	StatsPeriodMonth   = "month"
	StatsPeriodQuarter = "quarter"
	StatsPeriodRange   = "range" // 원하는 기간
	StatsPeriodAll     = "all"   // 하위호환(미사용)
)

// StatsOffset 상대 기간 (하위호환)
const (
	StatsOffsetCurrent = 0
	StatsOffsetPrev    = 1
	StatsOffsetPrev2   = 2
)

// StatsQuery 통계 조회 조건
type StatsQuery struct {
	Metric string // progress|completed|received|overdue
	Scope  string // team|assignee
	Period string // day|week|month|quarter|range
	// day: Date (YYYY-MM-DD)
	// week: Date = 주 내 아무 날
	Date string
	// month: YYYY-MM
	Month string
	// quarter: YYYY-Qn (예: 2026-Q3)
	Quarter string
	// range
	From string // YYYY-MM-DD
	To   string // YYYY-MM-DD inclusive
}

// 일일/주간/월간/기간지정 요약 뷰
const (
	StatsViewDay   = "day"
	StatsViewWeek  = "week"
	StatsViewMonth = "month"
	StatsViewRange = "range"
)

// StatsWorkSlice 업무구분(AS·점검·행정)별 예정/접수/처리
type StatsWorkSlice struct {
	Planned    int
	Receipt    int
	Process    int
	Modified   int // 계획 대비 일정 변경(예정일과 다르게 처리)
	OnPlan     int // 예정일 그대로 완료(계획대로 실행)
	Quota      int // 정기점검: 해당 월 방문 목표(사이트×점검대상)
	Cumulative int // 정기점검: 그 달 1일~기간 종료 완료 누적
	Remaining  int // 정기점검: 월 목표 − 누적
	MonthLabel string
}

// StatsBucketCounts 한 기간의 집계
type StatsBucketCounts struct {
	AS            StatsWorkSlice
	Mnt           StatsWorkSlice
	Admin         StatsWorkSlice
	ProgressScope string // 실행률 대상. 비면 전 유형. 건수 합계 PlannedTotal 에는 쓰지 않는다 (§4.5.4)
}

func (b StatsBucketCounts) PlannedTotal() int {
	return b.AS.Planned + b.Mnt.Planned + b.Admin.Planned
}
func (b StatsBucketCounts) ReceiptTotal() int {
	return b.AS.Receipt + b.Mnt.Receipt + b.Admin.Receipt
}
func (b StatsBucketCounts) ProcessTotal() int {
	return b.AS.Process + b.Mnt.Process + b.Admin.Process
}
func (b StatsBucketCounts) ModifiedTotal() int {
	return b.AS.Modified + b.Mnt.Modified + b.Admin.Modified
}
func (b StatsBucketCounts) OnPlanTotal() int {
	return b.AS.OnPlan + b.Mnt.OnPlan + b.Admin.OnPlan
}

func (s StatsWorkSlice) HasExecutionRate() bool {
	return s.Planned > 0
}

func (s StatsWorkSlice) ExecutionRatePct() float64 {
	p := s.Planned
	if p <= 0 {
		return 0
	}
	on := s.OnPlan
	if on < 0 {
		on = 0
	}
	if on > p {
		on = p
	}
	return float64(on) * 100 / float64(p)
}

func (b StatsBucketCounts) HasExecutionRate() bool {
	p, _ := b.execParts()
	return p > 0
}

// execParts 계획 대비 실행률의 분모·분자만 고른다 (§4.5.4).
// progress_scope 에 admin 이 없으면 행정·지원은 여기만 빠진다. 건수·소요시간은 PlannedTotal 등을 쓴다.
// 되돌림: 행정·지원 예정일 입력률이 4주 연속 90% 이상이면 app_settings.progress_scope 에 admin 을 넣는다.
func (b StatsBucketCounts) execParts() (planned, onPlan int) {
	if ProgressScopeIncludes(b.ProgressScope, "as") {
		planned += b.AS.Planned
		onPlan += b.AS.OnPlan
	}
	if ProgressScopeIncludes(b.ProgressScope, "maintenance") {
		planned += b.Mnt.Planned
		onPlan += b.Mnt.OnPlan
	}
	if ProgressScopeIncludes(b.ProgressScope, "admin") {
		planned += b.Admin.Planned
		onPlan += b.Admin.OnPlan
	}
	return planned, onPlan
}

func (b StatsBucketCounts) ExecPlanned() int {
	p, _ := b.execParts()
	return p
}

// StatsGrade §4.4 신뢰도 등급
const (
	StatsGradeTrusted = "trusted" // 🟢 표본 ≥ 20
	StatsGradeRef     = "ref"     // 🟡 5~19
	StatsGradeNA      = "na"      // ⚪ 표본 < 5 또는 분모 0
)

// StatsValue 지표 표시값. 분모 0이면 ShowValue=false → 화면은 「—」.
type StatsValue struct {
	Value      float64
	Sample     int
	DenomZero  bool
	ShowValue  bool
	ShowTarget bool
	ShowDelta  bool
	Grade      string
	GradeLabel string
	GradeMark  string
	Reason     string
}

// StatsReliability 표본·분모로 §4.4 등급을 붙인다. value는 ExecutionRatePct 등 기존 산식 결과.
func StatsReliability(sample int, denomZero bool, value float64) StatsValue {
	v := StatsValue{Value: value, Sample: sample, DenomZero: denomZero}
	if denomZero || sample <= 0 {
		v.Grade = StatsGradeNA
		v.GradeLabel = "측정불가"
		v.GradeMark = "⚪"
		v.Reason = "해당 기간 예정 업무 없음"
		return v
	}
	if sample < 5 {
		v.Grade = StatsGradeNA
		v.GradeLabel = "측정불가"
		v.GradeMark = "⚪"
		v.Reason = fmt.Sprintf("표본 %d건", sample)
		return v
	}
	v.ShowValue = true
	v.ShowDelta = true
	v.Reason = fmt.Sprintf("표본 %d건", sample)
	if sample < 20 {
		v.Grade = StatsGradeRef
		v.GradeLabel = "참고"
		v.GradeMark = "🟡"
		return v
	}
	v.Grade = StatsGradeTrusted
	v.GradeLabel = "신뢰"
	v.GradeMark = "🟢"
	v.ShowTarget = true
	return v
}

// CapGradeIfImport 이관 데이터를 포함하면 신뢰 등급을 참고로 내린다(§4.3 오염).
func (v StatsValue) CapGradeIfImport(includeImport bool) StatsValue {
	if !includeImport || v.Grade != StatsGradeTrusted {
		return v
	}
	v.Grade = StatsGradeRef
	v.GradeLabel = "참고"
	v.GradeMark = "🟡"
	v.ShowTarget = false
	return v
}

func (v StatsValue) WithReason(reason string) StatsValue {
	if reason != "" && v.DenomZero {
		v.Reason = reason
	}
	return v
}

// CapIfLowPlanning 계획 수립률이 목표 미만이면 실행율을 ⚪ 측정불가로 내린다(§8.4).
// ExecutionRatePct 산식 자체는 바꾸지 않고 표시 등급만 조정한다.
func (v StatsValue) CapIfLowPlanning(hasPlanning bool, planningPct float64) StatsValue {
	if !hasPlanning || planningPct >= StatsPlanTargetPct {
		return v
	}
	v.Grade = StatsGradeNA
	v.GradeLabel = "측정불가"
	v.GradeMark = "⚪"
	v.ShowTarget = false
	v.ShowValue = false
	v.ShowDelta = false
	v.Reason = fmt.Sprintf("계획 수립률 %.1f%% (목표 %.0f%%)", planningPct, StatsPlanTargetPct)
	return v
}

// HasPlanningRate 미완료 업무가 있으면 계획 수립률을 표시한다.
func HasPlanningRate(open int) bool {
	return open > 0
}

// PlanningRatePct 계획 수립률(%). (예정일 있는 건 + 미정사유) ÷ 미완료 × 100. §8.4
func PlanningRatePct(planned, open int) float64 {
	if open <= 0 {
		return 0
	}
	if planned < 0 {
		planned = 0
	}
	if planned > open {
		planned = open
	}
	return float64(planned) * 100 / float64(open)
}

// DailyAvgCompleted 일일 평균 완료 건수 = 완료 ÷ 워킹데이. 분모 0이면 표시 불가(§16.1).
func DailyAvgCompleted(completed, workingDays int) (float64, bool) {
	if workingDays <= 0 {
		return 0, false
	}
	if completed < 0 {
		completed = 0
	}
	return float64(completed) / float64(workingDays), true
}

// ExecutionRatePct 계획대비 실행률(%).
// 예정 중 "계획대로(예정일 당일) 완료"한 비율. 처리 건수와 무관하며 100%를 넘지 않음.
// 예: 예정 4곳 중 2곳만 일정 변경 → (4-2)/4 = 50% (= OnPlan/Planned).
func (b StatsBucketCounts) ExecutionRatePct() float64 {
	p, on := b.execParts()
	if p <= 0 {
		return 0
	}
	if on < 0 {
		on = 0
	}
	if on > p {
		on = p
	}
	return float64(on) * 100 / float64(p)
}

// StatsPeriodColumn 요약 표의 기간 열
type StatsPeriodColumn struct {
	Key         string // prev | current | next
	Label       string // 전일, 오늘, 익일 …
	From        string // YYYY-MM-DD inclusive
	ToExclusive string // YYYY-MM-DD exclusive
	RangeLabel  string // 표시용 기간 문구
	ShowActual  bool   // 기간 시작일 ≤ 오늘: 접수·처리·수정·실행률 표시. 미래 기간은 예정만.
	Counts      StatsBucketCounts
}

// StatsMeetingFilter 팀전체 / 담당자별 / 업무구분·업무(제품)별 + 사업
type StatsMeetingFilter struct {
	Scope         string // team | assignee | work_type | product
	Key           string // 담당자명 · as|maintenance|admin · 제품명
	ProjectID     string // 사업(work_projects) 선택 시
	IncludeImport         bool // true면 data_origin=import 포함. 기본은 제외(§4.3)
	ExcludeSalesActivity  bool // true면 source_type=sales_activity 를 실행률에서 뺀다. 기본은 포함(§32.11)
	MetricsBaseDate       string // 집계 하한 YYYY-MM-DD. 설정에서 채운다. 토글로 풀리지 않는다 (§4.5.3)
	ProgressScope         string // 실행률 대상. 예: as,maintenance (§4.5.4)
}

// StatsKPICard 상단 중요 통계 카드
type StatsKPICard struct {
	ExecutionRate   float64 // 계획대비 실행율(%). 분모 0이면 0 — 화면은 ExecDisplay 사용
	ExecutionDelta  float64
	ExecutionSample int
	HasExecution    bool
	VisitAvgDays    float64
	VisitDelta      float64
	VisitSample     int
	CompleteAvgDays float64
	CompleteDelta   float64
	CompleteSample  int
	HasVisit        bool
	HasComplete     bool
	LeadTimeWarn    string // §4.6.5 방문 > 완료이면 화면 경고
	ExecDisplay     StatsValue
	VisitDisplay    StatsValue
	CompleteDisplay StatsValue
	PlanningRate    float64
	PlanningOpen    int
	PlanningPlanned int
	HasPlanning     bool
	PlanDisplay     StatsValue
}

// StatsWorkAnalysis 선택 기간 업무 분석(접수·방문·완료·이월). AS+정기점검+행정 합산.
type StatsWorkAnalysis struct {
	Receipt   int
	Visit     int
	Completed int
	CarryIn   int
	CarryOut  int
	AS        StatsWorkTypeCounts
	Mnt       StatsWorkTypeCounts
	Admin     StatsWorkTypeCounts
	AdminLead AdminLeadBreakdown
}

// StatsWorkTypeCounts 업무 유형별 분석 건수
type StatsWorkTypeCounts struct {
	Receipt   int
	Visit     int
	Completed int
	CarryIn   int
	CarryOut  int
}

func (a *StatsWorkAnalysis) Rollup() {
	a.Receipt = a.AS.Receipt + a.Mnt.Receipt + a.Admin.Receipt
	a.Visit = a.AS.Visit + a.Mnt.Visit + a.Admin.Visit
	a.Completed = a.AS.Completed + a.Mnt.Completed + a.Admin.Completed
	a.CarryIn = a.AS.CarryIn + a.Mnt.CarryIn + a.Admin.CarryIn
	a.CarryOut = a.AS.CarryOut + a.Mnt.CarryOut + a.Admin.CarryOut
}

const (
	StatsTeamLabel       = "사업팀 전체"
	StatsUnassignedLabel = "(미배정)"
	WeeklyEventReceipt   = "접수"
	WeeklyEventAction    = "조치"
	WeeklyEventComplete  = "완료"
	WeeklyEventMnt       = "점검"
	WeeklyEventAdmin     = "행정"
)

// WeeklyReport 주간업무보고서 엑셀 데이터 (§16.1 시트 2개)
type WeeklyReport struct {
	WeekFrom           string // 월요일 YYYY-MM-DD
	WeekTo             string // 일요일 YYYY-MM-DD (포함)
	WeekToEx           string
	Friday             string // 파일명 기준일(그 주 금요일)
	WorkingDays        int
	HolidayYearMissing bool
	PersonRows         []WeeklyPersonRow // 첫 행 팀 전체, 아래 담당자(완료 내림차순, 미배정 맨 아래)
	PlanningRate       float64
	PlanningOpen       int
	PlanningPlanned    int
	HasPlanning        bool
	PlanDisplay        StatsValue
	MntQuota           int
	MntReceipt         int
	MntDone            int
	MntCumulative      int
	MntRemaining       int
	MntMonthLabel      string
	AdminLead          AdminLeadBreakdown // §13.10 행정/지원 평균 리드타임·외부 대기 비중
	Events             []WeeklyEventRow
}

// WeeklyPersonRow 통계 시트 1행 (팀 또는 담당자). 실행률·소요일은 팀/담당자 기준으로 각각 재계산.
type WeeklyPersonRow struct {
	Label           string
	IsTeam          bool
	Unassigned      bool
	Receipt         int
	Completed       int
	DayAvg          float64
	HasDayAvg       bool
	DayMax          int
	DayMaxDate      string // YYYY-MM-DD, 동점이면 가장 이른 날
	CarryOut        int
	InProgress      int
	Unplanned       int
	ExecDisplay     StatsValue
	VisitDisplay    StatsValue
	CompleteDisplay StatsValue
}

// WeeklyEventRow 전체 리스트 1행. 같은 건의 접수·조치·완료는 각각 한 행.
type WeeklyEventRow struct {
	Kind           string // 접수 / 조치 / 완료 / 점검 / 행정
	WorkNo         string
	OccurDate      string
	WorkType       string // AS / 정기점검 / 행정 / 지원
	Customer       string
	Product        string
	Assignee       string
	Content        string
	Result         string
	Minutes        int
	ParentTaskID   string
	RecurrenceRole string
}

// StatsSpotlight 기간 내 주목 건(최장소요·1시간초과·방문차수·긴급·중요 상)
type StatsSpotlight struct {
	TopN         int
	PeriodCount  int // 기간 접수 건수(최장 소요 N 산정)
	Longest      []StatsRow
	OverHour     []StatsRow
	MostVisits   []StatsRow
	HighPriority []StatsRow
	Timeline     []StatsRow // 접수/방문/완료일
}

// StatsChartPoint 기간 축 1점(막대·추이)
type StatsChartPoint struct {
	Label      string  `json:"label"`       // 축 표시: 일=01/02, 주=전전주…, 월=전전월…
	RangeLabel string  `json:"range_label"` // 실제 구간(툴팁): 07/20~07/26 또는 2026-06
	Date       string  `json:"date"`        // 버킷 시작일 YYYY-MM-DD
	Received   int     `json:"received"`    // 접수
	Open       int     `json:"open"`        // 미완료(접수·배정·진행중·보류·이관 등)
	Completed  int     `json:"completed"`   // 완료
	ExecRate   float64 `json:"exec_rate"`
	Planned    int     `json:"planned"`
	Process    int     `json:"process"`
}

// DailyMeetingStat 일 단위 저장(계획대비 실행률)
type DailyMeetingStat struct {
	StatDate      string
	Scope         string
	ScopeKey      string
	Planned       int
	Receipt       int
	Process       int
	Modified      int
	ExecutionRate float64
	ASPlanned     int
	ASReceipt     int
	ASProcess     int
	MntPlanned    int
	MntReceipt    int
	MntProcess    int
	AdminPlanned  int
	AdminReceipt  int
	AdminProcess  int
}

const (
	DailyStatusReceipt   = "접수"
	DailyStatusProgress  = "진행중"
	DailyStatusComplete  = "완료"
	DailyStatusCarry     = "이월"
	DailyDetailKindTotal = "합계"
)

// DailyAssigneeReport 담당자별 일일업무 분석 (§16.4 시트 2개)
type DailyAssigneeReport struct {
	From               string
	To                 string // 포함
	ToEx               string
	FileDay            string // 파일명 YYYYMMDD
	WorkingDays        int
	HolidayYearMissing bool
	Days               []DailyAssigneeDay
	People             []DailyAssigneePerson
	TeamCounts         []int
	TeamMinutes        []int
	TeamCountTotal     int
	TeamCountAvg       float64
	HasTeamCountAvg    bool
	TeamCountMax       int
	TeamMinuteTotal    int
	TeamMinuteAvg      float64
	HasTeamMinuteAvg   bool
	TeamMinuteMax      int
	Details            []DailyAssigneeDetailRow
}

// DailyAssigneeDay 매트릭스 한 열.
type DailyAssigneeDay struct {
	Date  string // YYYY-MM-DD
	Label string // 08-10(월)
	Off   bool   // 토·일·공휴일 — 회색, 합계 제외
}

// DailyAssigneePerson 매트릭스 한 행.
type DailyAssigneePerson struct {
	Label        string
	Unassigned   bool
	Counts       []int
	Minutes      []int
	CountTotal   int
	CountAvg     float64
	HasCountAvg  bool
	CountMax     int
	MinuteTotal  int
	MinuteAvg    float64
	HasMinuteAvg bool
	MinuteMax    int
	LeaveMarks   []string // 칸마다 "연차" 또는 "". 날짜를 휴일로 칠하지 않는다. §23.13.6
}

// DailyAssigneeDetailRow 담당자별 상세 1행. IsDayTotal이면 그날 합계.
type DailyAssigneeDetailRow struct {
	Assignee   string
	Date       string
	Weekday    string
	Kind       string // AS / 정기점검 / 행정 / 지원 / 합계
	WorkNo     string
	Customer   string
	Content    string
	Status     string // 접수 / 진행중 / 완료 / 이월
	Minutes    int
	Cumulative int
	OccurAt    string
	IsDayTotal bool
	NewPerson  bool // 담당자 바뀌는 첫 행 — 구분선
}

// WeekdayKR 요일 한글 한 글자 (일~토).
func WeekdayKR(t time.Time) string {
	return [...]string{"일", "월", "화", "수", "목", "금", "토"}[t.Weekday()]
}

// DateLabelMDW 08-10(월) 형태.
func DateLabelMDW(date string) string {
	t, err := time.ParseInLocation("2006-01-02", date, time.Local)
	if err != nil {
		return date
	}
	return t.Format("01-02") + "(" + WeekdayKR(t) + ")"
}

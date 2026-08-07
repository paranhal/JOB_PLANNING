package model

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
	CompleteDate    string // 완료일(완료 목록용)
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

// 중요 KPI 기준값
const (
	StatsExecTargetPct   = 90.0 // 계획대비 실행율 목표(%)
	StatsVisitTargetDays = 3.0  // 접수→방문 목표(일)
)

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

// 일일/주간/월간 요약 뷰
const (
	StatsViewDay   = "day"
	StatsViewWeek  = "week"
	StatsViewMonth = "month"
)

// StatsWorkSlice 업무구분(AS·점검·행정)별 예정/접수/처리
type StatsWorkSlice struct {
	Planned  int
	Receipt  int
	Process  int
	Modified int // 계획 대비 일정 변경(예정일과 다르게 처리)
	OnPlan   int // 예정일 그대로 완료(계획대로 실행)
}

// StatsBucketCounts 한 기간의 집계
type StatsBucketCounts struct {
	AS    StatsWorkSlice
	Mnt   StatsWorkSlice
	Admin StatsWorkSlice
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

// ExecutionRatePct 계획대비 실행률(%).
// 예정 중 "계획대로(예정일 당일) 완료"한 비율. 처리 건수와 무관하며 100%를 넘지 않음.
// 예: 예정 4곳 중 2곳만 일정 변경 → (4-2)/4 = 50% (= OnPlan/Planned).
func (b StatsBucketCounts) ExecutionRatePct() float64 {
	p := b.PlannedTotal()
	if p <= 0 {
		return 0
	}
	on := b.OnPlanTotal()
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
	ShowActual  bool   // 전일(과거): 접수·처리·수정·실행률 표시
	Counts      StatsBucketCounts
}

// StatsMeetingFilter 팀전체 / 담당자별 / 업무구분·업무(제품)별 + 사업
type StatsMeetingFilter struct {
	Scope     string // team | assignee | work_type | product
	Key       string // 담당자명 · as|maintenance|admin · 제품명
	ProjectID string // 사업(work_projects) 선택 시
}

// StatsKPICard 상단 중요 통계 카드
type StatsKPICard struct {
	ExecutionRate   float64 // 계획대비 실행율(%)
	ExecutionDelta  float64 // 이전 기간 대비(pp)
	VisitAvgDays    float64 // 접수→방문 평균(일)
	VisitDelta      float64 // 이전 기간 대비(일)
	VisitSample     int
	CompleteAvgDays float64 // 접수→조치완료 평균(일)
	CompleteDelta   float64 // 이전 기간 대비(일)
	CompleteSample  int
	HasVisit        bool
	HasComplete     bool
}

// StatsChartPoint 기간 축 1점(막대·추이)
type StatsChartPoint struct {
	Label      string  `json:"label"`       // 축 표시: 일=01/02, 주=전전주…, 월=전전월…
	RangeLabel string  `json:"range_label"` // 실제 구간(툴팁): 07/20~07/26 또는 2026-06
	Date       string  `json:"date"`        // 버킷 시작일 YYYY-MM-DD
	Received  int `json:"received"`  // 접수
	Open      int `json:"open"`      // 미완료(접수·배정·진행중·보류·이관 등)
	Completed int `json:"completed"` // 완료
	ExecRate   float64 `json:"exec_rate"`
	Planned    int     `json:"planned"`
	Process    int     `json:"process"`
}

// DailyMeetingStat 일 단위 저장(계획대비 실행률)
type DailyMeetingStat struct {
	StatDate       string
	Scope          string
	ScopeKey       string
	Planned        int
	Receipt        int
	Process        int
	Modified       int
	ExecutionRate  float64
	ASPlanned      int
	ASReceipt      int
	ASProcess      int
	MntPlanned     int
	MntReceipt     int
	MntProcess     int
	AdminPlanned   int
	AdminReceipt   int
	AdminProcess   int
}

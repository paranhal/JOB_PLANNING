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
	ActionTaken     string // 처리내용
	Status          string // 처리상태(코드)
	StatusLabel     string // 처리상태(표시)
	ASID            string
	ASNumber        string
	ReceiptDate     string
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
	StatsScopeTeam     = "team"     // 도서관사업팀 전체
	StatsScopeAssignee = "assignee" // 담당자별
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

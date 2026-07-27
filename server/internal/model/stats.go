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

// StatsPeriod 기간 필터
const (
	StatsPeriodDay     = "day"
	StatsPeriodWeek    = "week"
	StatsPeriodMonth   = "month"
	StatsPeriodQuarter = "quarter"
	StatsPeriodAll     = "all"
)

// StatsOffset 상대 기간 (0=현재, 1=직전, 2=그 이전)
const (
	StatsOffsetCurrent = 0
	StatsOffsetPrev    = 1
	StatsOffsetPrev2   = 2
)

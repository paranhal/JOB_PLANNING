package model

// CompanyWeeklyPeriod 보고 기준일(월요일) 기준 전주·금주. §16.6.3
type CompanyWeeklyPeriod struct {
	Anchor           string // 기준 월요일 YYYY-MM-DD
	PrevFrom         string
	PrevTo           string // 포함 일요일
	PrevToEx         string
	ThisFrom         string
	ThisTo           string
	ThisToEx         string
	SheetNameDefault string // YY.M월N주(업무) — 화면에서 고칠 수 있는 기본값
	TitleE1          string
	PrevRangeLabel   string // 8.10(월)~8.16(일)
	ThisRangeLabel   string
	Year             int
	Month            int
	WeekN            int // 전주 월요일이 그 달의 몇 번째 월요일인가
}

// CompanyWeeklyRow 엑셀 20~25행 초안. §16.6.5
type CompanyWeeklyRow struct {
	RowKey          string
	SheetRow        int
	Division        string
	Team            string
	NoLabel         string
	DisplayName     string
	ProjectID       string
	RowKind         string // project | manual
	Highlight       bool
	IsActive        bool
	MissingProject  bool // 404행처럼 대응 사업이 없음
	PrevText        string // F열
	PlanText        string // G열
	HelpText        string // H열 — 화면 직접 입력
	DecisionText    string // I열 — 화면 직접 입력
	MntDoneSites    int
	ASDoneCount     int
	MntPlanSites    int
}

// CompanyWeeklyUnassigned 어느 행에도 안 붙은 활동. §16.6.5 · §16.6.9
type CompanyWeeklyUnassigned struct {
	Kind        string // maintenance | as | admin
	KindLabel   string
	ID          string
	Title       string
	Date        string
	ProjectID   string
	ProjectName string
	Href        string
	Bucket      string // prev | plan | carry
	BucketLabel string
}

// CompanyWeeklyDraft 전사 주간업무보고 초안 (시트 쓰기 전). §16.6.5~7
type CompanyWeeklyDraft struct {
	Period     CompanyWeeklyPeriod
	Rows       []CompanyWeeklyRow
	Unassigned []CompanyWeeklyUnassigned
}

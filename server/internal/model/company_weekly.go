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
	RowKey         string
	SheetRow       int
	Division       string
	Team           string
	NoLabel        string
	DisplayName    string
	ProjectID      string
	RowKind        string // project | activity | manual
	ActivityKind   string // asset | tech | mnt_as | ''
	Highlight      bool
	IsActive       bool
	MissingProject bool   // 404행처럼 대응 사업이 없음
	PrevText       string // F열
	PlanText       string // G열
	HelpText       string // H열 — 화면 직접 입력
	DecisionText   string // I열 — 화면 직접 입력
	MntDoneSites   int
	ASDoneCount    int
	MntPlanSites   int
	AutoDone       []CompanyWeeklyAutoLine // 자동 줄(키 포함). 화면 숨김·수정용
	AutoPlan       []CompanyWeeklyAutoLine
}

// CompanyWeeklyAutoLine 집계 줄 식별. 키=(종류, 대상 ID)
type CompanyWeeklyAutoLine struct {
	Key  string
	Text string
}

// IncompleteMntVisit 완료 표시가 없는 정기점검 방문. §68.6
type IncompleteMntVisit struct {
	VisitID   string
	OrgName   string
	VisitDate string
	PlanID    string
	ProjectID string
	Href      string
}

const (
	UnassignedReasonNoRow     = "no_row"
	UnassignedReasonNoProject = "no_project"
	UnassignedReasonForeign   = "foreign_org"
)

// CompanyWeeklyUnassigned 어느 행에도 안 붙은 활동. §16.6.5 · §16.6.9
type CompanyWeeklyUnassigned struct {
	Kind           string // maintenance | as | admin
	KindLabel      string
	ID             string
	Title          string
	Date           string
	ProjectID      string
	ProjectName    string
	ProjectOrgID   string
	ProjectKind    string
	Href           string
	Bucket         string // prev | plan | carry
	BucketLabel    string
	Reason         string // no_row | no_project | foreign_org
	Assignee       string
	AssigneeUserID string
}

// CompanyWeeklyUnassignedProject 보고서 행이 없는 사업 묶음.
type CompanyWeeklyUnassignedProject struct {
	ProjectID     string
	ProjectName   string
	ProjectOrgID  string
	ProjectKind   string
	Count         int
	KindSummary   string
	Items         []CompanyWeeklyUnassigned
}

// CompanyWeeklyExclusion 미포함 후보가 빠진 이유(?debug=1). 95-34
type CompanyWeeklyExclusion struct {
	ID      string
	Kind    string
	Title   string
	Date    string
	Reasons string
}

// CompanyWeeklyDraft 전사 주간업무보고 초안 (시트 쓰기 전). §16.6.5~7
type CompanyWeeklyDraft struct {
	Period                 CompanyWeeklyPeriod
	Rows                   []CompanyWeeklyRow
	Unassigned             []CompanyWeeklyUnassigned
	UnassignedNoRow        []CompanyWeeklyUnassignedProject
	UnassignedNoRowTaskN   int
	UnassignedNoProject    []CompanyWeeklyUnassigned
	UnassignedForeign      []CompanyWeeklyUnassignedProject
	UnassignedForeignTaskN int
	UnassignedMainN        int
	IncompleteMnt          []IncompleteMntVisit
	DebugExclusions        []CompanyWeeklyExclusion
	ReportStatus           string
}

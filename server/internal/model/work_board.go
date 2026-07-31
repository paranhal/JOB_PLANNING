package model

// WorkPrefix 업무 리스트 접두어
const (
	WorkPrefixAS           = "as"           // [AS]
	WorkPrefixMaintenance  = "maintenance"  // [정기점검]
	WorkPrefixConfirm      = "confirm"      // [확인]
	WorkPrefixGeneral      = "general"      // [일반업무]
)

func WorkPrefixLabel(p string) string {
	switch p {
	case WorkPrefixAS:
		return "AS"
	case WorkPrefixMaintenance:
		return "정기점검"
	case WorkPrefixConfirm:
		return "확인"
	case WorkPrefixGeneral:
		return "일반업무"
	default:
		return p
	}
}

// WorkPrefixCounts 접두어별 건수
type WorkPrefixCounts struct {
	AS          int `json:"as"`
	Maintenance int `json:"maintenance"`
	Confirm     int `json:"confirm"`
	General     int `json:"general"`
	Total       int `json:"total"`
}

func (c *WorkPrefixCounts) Add(prefix string, n int) {
	switch prefix {
	case WorkPrefixAS:
		c.AS += n
	case WorkPrefixMaintenance:
		c.Maintenance += n
	case WorkPrefixConfirm:
		c.Confirm += n
	case WorkPrefixGeneral:
		c.General += n
	}
	c.Total += n
}

// WorkDashStats 담당자/관리자 홈 대시보드 지표
type WorkDashStats struct {
	OpenTotal            int                   `json:"open_total"`
	OpenByAssignee       []AssigneePrefixStats `json:"open_by_assignee"`
	TodayScheduled       WorkPrefixCounts      `json:"today_scheduled"`
	TodayByAssignee      []AssigneePrefixStats `json:"today_by_assignee"`
	Delayed              int                   `json:"delayed"`
	DelayedByAssignee    []AssigneePrefixStats `json:"delayed_by_assignee"`
	TodayCompleted       WorkPrefixCounts      `json:"today_completed"`
	CompletedByAssignee  []AssigneePrefixStats `json:"completed_by_assignee"`
	SchedulePending      int                   `json:"schedule_pending"`
	PendingByAssignee    []AssigneePrefixStats `json:"pending_by_assignee"`
	Unassigned           int                   `json:"unassigned"`
	UnassignedPrefixLine string                `json:"unassigned_prefix_line"` // 미배정은 접두어만: "[AS] 2 / [정기점검] 1"
}

// AssigneePrefixStats 담당자별 접두어 집계 (대시보드 카드용)
type AssigneePrefixStats struct {
	Name   string           `json:"name"`
	Counts WorkPrefixCounts `json:"counts"`
	Line   string           `json:"line"` // 표시용: "양기현: [AS] 3 / [정기점검] 1"
}

// WorkListItem 통합 업무 리스트 1행
type WorkListItem struct {
	Prefix       string `json:"prefix"`
	RefID        string `json:"ref_id"`
	RefNumber    string `json:"ref_number"`
	Title        string `json:"title"`
	OrgName      string `json:"org_name"`
	Assignee     string `json:"assignee"`
	ScheduledDate string `json:"scheduled_date"`
	Status       string `json:"status"`
	StatusLabel  string `json:"status_label"`
	Href         string `json:"href"`
	DaysOverdue  int    `json:"days_overdue"`
	SubLabel     string `json:"sub_label"` // 재방문 등 보조 라벨
}

// WorkBucket 리스트 필터 버킷
const (
	WorkBucketOpen            = "open"
	WorkBucketToday           = "today"
	WorkBucketDelayed         = "delayed"
	WorkBucketCompletedToday  = "completed_today"
	WorkBucketSchedulePending = "schedule_pending"
	WorkBucketUnassigned      = "unassigned"
)

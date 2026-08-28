package model

import (
	"strings"
	"time"
)

const (
	WBTaskWaiting    = "waiting"
	WBTaskInProgress = "in_progress"
	WBTaskReview     = "review"   // 검토중 — 칸반「진행중」열 뱃지 (§33.5.2)
	WBTaskHold       = "hold"     // 보류 → 진행중 열 뱃지
	WBTaskTransfer   = "transfer" // 이관 → 진행중 열 뱃지
	WBTaskComplete   = "complete"
	// inbox / waiting_for / cancelled 는 work_gtd.go
)

const (
	WBPriorityUrgent = "urgent"
	WBPriorityHigh   = "high"
	WBPriorityNormal = "normal"
	WBPriorityLow    = "low"
)

const (
	WBProjectActive   = "active"
	WBProjectComplete = "complete"
	WBProjectArchived = "archived"
)

// 업무 구분: 행정/지원은 직접 등록, AS·정기점검은 원본 배치 시 자동 등록
const (
	WBWorkAdmin       = "admin"
	WBWorkSupport     = "support"
	WBWorkAS          = "as"
	WBWorkMaintenance = "maintenance"
)

// 업무 등록 시간표에 올리는 카드의 출처. 빈 값은 행정관련 업무(직접 등록).
const (
	WBSourceAS            = "as"
	WBSourceMaintenance   = "maintenance"
	WBSourceSalesActivity = "sales_activity"
)

// work_task_members.member_role (§7.7.2). work_tasks.assignee 는 주담당(owner)과 같다.
const (
	WBMemberOwner   = "owner"
	WBMemberSupport = "support"
)

// WBCategory 시간표 카드의 분류: as / maintenance / admin
func WBCategory(sourceType string) string {
	switch sourceType {
	case WBSourceAS:
		return WBSourceAS
	case WBSourceMaintenance:
		return WBSourceMaintenance
	case WBSourceSalesActivity:
		return WBSourceSalesActivity
	default:
		return WBWorkAdmin
	}
}

func WBCategoryLabel(cat string) string {
	switch cat {
	case WBSourceAS:
		return "AS"
	case WBSourceMaintenance:
		return "점검"
	case WBSourceSalesActivity:
		return "영업"
	default:
		return "행정"
	}
}

func WBCategoryClass(cat string) string {
	switch cat {
	case WBSourceAS:
		return "bg-rose-100 text-rose-800 border border-rose-300 border-l-[3px] border-l-rose-500 shadow-sm"
	case WBSourceMaintenance:
		return "bg-sky-100 text-sky-800 border border-sky-300 border-l-[3px] border-l-sky-500 shadow-sm"
	case WBSourceSalesActivity:
		return "bg-orange-100 text-orange-800 border border-orange-300 border-l-[3px] border-l-orange-500 shadow-sm"
	default:
		return "bg-violet-100 text-violet-800 border border-violet-300 border-l-[3px] border-l-violet-500 shadow-sm"
	}
}

// WBCard 업무 등록 화면의 카드 (시간표에 배치할 대상 · 배치된 업무 공통)
type WBCard struct {
	Kind         string `json:"kind"`          // as | maintenance | task
	RefID        string `json:"ref_id"`        // as_id / visit_id / task_id
	Category     string `json:"category"`      // as | maintenance | admin
	Title        string `json:"title"`         // 카드 본문 ([AS]고객명 등)
	SubTitle     string `json:"sub_title"`     // 증상·설명
	ProductType  string `json:"product_type"`  // 정기점검 점검 대상(색 구분용)
	SourceNumber string `json:"source_number"` // AS 접수번호 / 정기점검 방문번호
	SourceHref   string `json:"source_href"`   // 원본 화면 링크(연간 일정 등)
	ActionHref   string `json:"action_href"`   // 조치 화면(있으면 DetailHref 우선)
	Assignee     string `json:"assignee"`
	WorkDate     string `json:"work_date"`
	StartTime    string `json:"start_time"`
	EndTime      string `json:"end_time"`
	DurationMin  int    `json:"duration_min"`
	PlannedDay   string `json:"planned_day"`
	TaskID       string `json:"task_id"`
	ParentTaskID string `json:"parent_task_id,omitempty"`
	Status       string `json:"status,omitempty"`       // 업무처리현황 등 표시용
	StatusLabel  string `json:"status_label,omitempty"` // 완료·부분완료 등
	ProjectID    string `json:"project_id,omitempty"`
}

// Label 카드 앞에 붙는 분류 표시: [AS] / [점검] / [행정]
func (c WBCard) Label() string { return "[" + WBCategoryLabel(c.Category) + "]" }

// DetailHref 「조치」— AS는 `/as/{id}/action`, 정기점검은 방문 조치(`/maintenance/visits/{id}/action`).
// 수정(EditHref)과 분리. 미배치 팔레트는 빈 문자열 → 「등록」모달.
func (c WBCard) DetailHref() string {
	placed := c.TaskID != "" || (c.Kind == "task" && c.RefID != "")
	if !placed {
		return ""
	}
	if href := strings.TrimSpace(c.ActionHref); href != "" {
		return href
	}
	if c.Category == WBSourceAS {
		asID := strings.TrimPrefix(c.SourceHref, "/as/")
		if asID != "" && !strings.Contains(asID, "/") {
			return "/as/" + asID + "/action"
		}
	}
	if c.Category == WBSourceMaintenance {
		// Kind=maintenance 팔레트 RefID=visit_id (배치된 task는 ActionHref 사용)
		if c.Kind == WBSourceMaintenance && strings.TrimSpace(c.RefID) != "" {
			return "/maintenance/visits/" + c.RefID + "/action"
		}
	}
	return c.EditHref()
}

// EditHref 「수정」— 일일 업무 수정 화면(`/workboard/tasks/{id}`).
func (c WBCard) EditHref() string {
	if c.TaskID != "" {
		return "/workboard/tasks/" + c.TaskID
	}
	if c.Kind == "task" && c.RefID != "" {
		return "/workboard/tasks/" + c.RefID
	}
	return ""
}

func WBWorkTypeLabel(s string) string {
	switch s {
	case WBWorkAdmin:
		return "행정업무"
	case WBWorkSupport:
		return "지원업무"
	case WBWorkAS: // = WBSourceAS
		return "AS"
	case WBWorkMaintenance: // = WBSourceMaintenance
		return "정기점검"
	default:
		return s
	}
}

func WBWorkTypeClass(s string) string {
	switch s {
	case WBWorkAdmin:
		return "bg-violet-100 text-violet-700"
	case WBWorkSupport:
		return "bg-teal-100 text-teal-700"
	case WBWorkAS:
		return "bg-rose-100 text-rose-700"
	case WBWorkMaintenance:
		return "bg-sky-100 text-sky-700"
	default:
		return "bg-gray-100 text-gray-600"
	}
}

func WBTaskStatusLabel(s string) string {
	switch s {
	case WBTaskWaiting:
		return "오늘예정업무"
	case WBTaskInProgress:
		return "진행중"
	case WBTaskHold:
		return "보류"
	case WBTaskTransfer:
		return "이관"
	case WBTaskReview:
		return "검토"
	case WBTaskInbox:
		return "수집함"
	case WBTaskWaitingFor:
		return "회신 대기"
	case WBTaskCancelled:
		return "취소"
	case WBTaskComplete:
		return "완료"
	case StatusPartialComplete:
		return "부분완료"
	case "done", "visit_done":
		return "완료"
	default:
		return s
	}
}

// ASStatusDisplayLabel AS 원본 상태 → 업무처리현황 카드 표기
func ASStatusDisplayLabel(asStatus string) string {
	switch strings.TrimSpace(asStatus) {
	case "completed", "closed":
		return "완료"
	case StatusPartialComplete:
		return "부분완료"
	case "hold":
		return "보류"
	case "transfer":
		return "이관"
	case "in_progress":
		return "진행중"
	case "assigned":
		return "담당자 배정"
	case "received":
		return "접수"
	case "cancelled":
		return "접수취소"
	default:
		return asStatus
	}
}

// WBKanbanBucket 칸반 열 키. §33.5.2 3열 — 할 일 / 진행중 / 완료.
// 보류·이관·회신 대기·검토중은 진행중. 취소는 칸반에 올리지 않는다.
func WBKanbanBucket(status string) string {
	switch status {
	case WBTaskCancelled:
		return ""
	case WBTaskInProgress, WBTaskHold, WBTaskTransfer, WBTaskReview, WBTaskWaitingFor:
		return WBTaskInProgress
	case WBTaskComplete:
		return WBTaskComplete
	default:
		return WBTaskWaiting
	}
}

// WBKanbanBadge 진행중 열에서 보류·이관·회신대기를 구분하는 뱃지 문구. §33.5.2
func WBKanbanBadge(status string) string {
	switch strings.TrimSpace(status) {
	case WBTaskHold:
		return "보류"
	case WBTaskTransfer:
		return "이관"
	case WBTaskWaitingFor:
		return "회신대기"
	case WBTaskReview:
		return "검토중"
	default:
		return ""
	}
}

func WBKanbanBadgeClass(status string) string {
	switch strings.TrimSpace(status) {
	case WBTaskHold:
		return "bg-amber-100 text-amber-900"
	case WBTaskTransfer:
		return "bg-sky-100 text-sky-800"
	case WBTaskWaitingFor:
		return "bg-indigo-100 text-indigo-800"
	case WBTaskReview:
		return "bg-orange-100 text-orange-800"
	default:
		return ""
	}
}

// MapASStatusToWB AS 접수 상태 → 일일 업무 현황 표시 상태.
// 보류·이관은 진행중 열 뱃지. 취소는 빈 문자열(제외).
func MapASStatusToWB(asStatus string) string {
	switch strings.TrimSpace(asStatus) {
	case "hold":
		return WBTaskHold
	case "transfer":
		return WBTaskTransfer
	case "completed", "closed":
		return WBTaskComplete
	case "partial_complete", "in_progress", "assigned":
		return WBTaskInProgress
	case "received":
		return WBTaskWaiting
	case "cancelled":
		return ""
	default:
		return WBTaskInProgress
	}
}

// WBTaskOnDate 배정일(없으면 예정일)이 date(YYYY-MM-DD)와 같은지.
func WBTaskOnDate(t WorkTask, date string) bool {
	d := strings.TrimSpace(t.WorkDate)
	if d == "" {
		d = strings.TrimSpace(t.DueDate)
	}
	return d != "" && d == date
}

func WBPriorityLabel(s string) string {
	switch s {
	case WBPriorityUrgent:
		return "긴급"
	case WBPriorityHigh:
		return "높음"
	case WBPriorityNormal:
		return "보통"
	case WBPriorityLow:
		return "낮음"
	default:
		return s
	}
}

func WBProjectStatusLabel(s string) string {
	switch s {
	case WBProjectActive:
		return "진행"
	case WBProjectComplete:
		return "완료"
	case WBProjectArchived:
		return "보관"
	default:
		return s
	}
}

type WorkProject struct {
	ProjectID       string `json:"project_id"`
	Name            string `json:"name"`              // 사업명
	ShortName       string `json:"short_name"`        // 목록용 짧은 이름
	PlanYear        int    `json:"plan_year"`         // 사업 연도
	IsPaid          bool   `json:"is_paid"`           // 유상(true)/무상(false)
	SortOrder       int    `json:"sort_order"`
	OrderingPartyID string `json:"ordering_party_id"` // 발주처(고객 마스터 연동)
	OrderingParty   string `json:"ordering_party"`    // 발주처 직접입력
	CustomerID      string `json:"customer_id"`       // 고객
	ContractType    string `json:"contract_type"`     // 계약방식
	BillingType     string `json:"billing_type"`      // 청구방식
	StartDate       string `json:"start_date"`        // 계약기간 시작
	EndDate         string `json:"end_date"`          // 계약기간 마감
	Notes           string `json:"notes"`             // 비고
	ContactID       string `json:"contact_id"`        // 고객 담당자
	Color           string `json:"color"`
	Status          string `json:"status"`
	SalesProjectID  string `json:"sales_project_id,omitempty"` // 승격 원본 (§32.10). 승격 외에 쓰지 않는다.
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`

	CustomerName      string `json:"customer_name,omitempty"`
	ContactName       string `json:"contact_name,omitempty"`
	OrderingPartyName string `json:"ordering_party_name,omitempty"`

	// 상세·목록 보조
	ScopeRules []ProjectScopeRule `json:"scope_rules,omitempty"`
	ASCount    int                `json:"as_count,omitempty"`
	MntCount   int                `json:"mnt_count,omitempty"`
	TaskCount  int                `json:"task_count,omitempty"`
	AssetCount int                `json:"asset_count,omitempty"`
}

// 범위 규칙 제품키 · 업무유형
const (
	ProductKeyKLAS       = "klas"
	ProductKeySejongKLAS = "sejong_klas"
	ProductKeyAnrobotics = "anrobotics"

	ScopeWorkAS          = "as"
	ScopeWorkMaintenance = "maintenance"
	ScopeWorkAdmin       = "admin"
)

// ProjectScopeRule 사업 귀속 규칙(상위기관 × 제품 × 업무유형)
type ProjectScopeRule struct {
	RuleID           string `json:"rule_id"`
	ProjectID        string `json:"project_id"`
	ParentCustomerID string `json:"parent_customer_id"`
	ProductKeys      string `json:"product_keys"` // klas,sejong_klas,anrobotics
	WorkKinds        string `json:"work_kinds"`   // as,maintenance,admin
	Notes            string `json:"notes"`

	ParentOrgName string `json:"parent_org_name,omitempty"`
}

// UnresolvedProjectRow 사업 귀속 해석 실패 건 (§16.6.9). 조용히 버리지 않고 관리 화면에 모은다.
type UnresolvedProjectRow struct {
	Kind        string // as | maintenance
	KindLabel   string
	ID          string
	Number      string
	Date        string
	CustomerID  string
	OrgName     string
	ProductType string
	Reason      string
	Href        string
}

func (r ProjectScopeRule) HasWorkKind(kind string) bool {
	kind = strings.TrimSpace(kind)
	for _, p := range strings.Split(r.WorkKinds, ",") {
		if strings.TrimSpace(p) == kind {
			return true
		}
	}
	return false
}

func (r ProjectScopeRule) ProductKeySlice() []string {
	var out []string
	for _, p := range strings.Split(r.ProductKeys, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func ProductKeyLabel(k string) string {
	switch strings.TrimSpace(k) {
	case ProductKeyKLAS:
		return "KLAS"
	case ProductKeySejongKLAS:
		return "세종 KLAS"
	case ProductKeyAnrobotics:
		return "앤로보틱스"
	default:
		return k
	}
}

func ScopeWorkKindLabel(k string) string {
	switch strings.TrimSpace(k) {
	case ScopeWorkAS:
		return "AS"
	case ScopeWorkMaintenance:
		return "정기점검"
	case ScopeWorkAdmin:
		return "행정/지원"
	default:
		return k
	}
}

func ProductKeysLabel(csv string) string {
	var parts []string
	for _, k := range strings.Split(csv, ",") {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		parts = append(parts, ProductKeyLabel(k))
	}
	if len(parts) == 0 {
		return "—"
	}
	return strings.Join(parts, ", ")
}

func ScopeWorkKindsLabel(csv string) string {
	var parts []string
	for _, k := range strings.Split(csv, ",") {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		parts = append(parts, ScopeWorkKindLabel(k))
	}
	if len(parts) == 0 {
		return "—"
	}
	return strings.Join(parts, ", ")
}

func (p WorkProject) PaidLabel() string {
	if p.IsPaid {
		return "유상"
	}
	return "무상"
}

func (p WorkProject) DisplayName() string {
	if strings.TrimSpace(p.ShortName) != "" {
		return p.ShortName
	}
	return p.Name
}

// ProjectMatchRow 사업 상세에 보이는 매칭 건 요약
type ProjectMatchRow struct {
	ID           string `json:"id"`
	Number       string `json:"number"`
	Date         string `json:"date"`
	OrgName      string `json:"org_name"`
	Title        string `json:"title"`
	Status       string `json:"status"`
	StatusLabel  string `json:"status_label"`
	Href         string `json:"href"`
	ProductHint  string `json:"product_hint"`
}

// OrderingPartyLabel 발주처 표시명 (고객 마스터 연동 우선, 없으면 직접입력값)
func (p WorkProject) OrderingPartyLabel() string {
	if p.OrderingPartyName != "" {
		return p.OrderingPartyName
	}
	return p.OrderingParty
}

type WorkTask struct {
	TaskID       string    `json:"task_id"`
	WorkType     string    `json:"work_type"` // admin / support / as / maintenance
	ProjectID    string    `json:"project_id"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	DueDate      string    `json:"due_date"`
	WorkDate     string    `json:"work_date"`  // 수행일 YYYY-MM-DD
	StartTime    string    `json:"start_time"` // HH:MM (15분 단위)
	EndTime      string    `json:"end_time"`   // HH:MM
	DurationMin  int       `json:"duration_min"`
	Status       string    `json:"status"`
	Priority     string    `json:"priority"`
	Assignee     string    `json:"assignee"`
	Tags         string    `json:"tags"`
	Progress     int       `json:"progress"`
	SourceType   string    `json:"source_type"` // as / maintenance / 빈 값
	SourceID     string    `json:"source_id"`   // as_id / visit_id
	ParentTaskID string    `json:"parent_task_id"`
	CustomerID   string    `json:"customer_id"`   // 거래처(고객마스터)
	CustomerName   string    `json:"customer_name"` // 거래처 직접입력
	HoldReason     string    `json:"hold_reason"`
	ReviewDate     string    `json:"review_date"`
	CancelReason   string    `json:"cancel_reason"`
	WaitPartyKind  string    `json:"wait_party_kind"`
	WaitParty      string    `json:"wait_party"`
	WaitRequest    string    `json:"wait_request"`
	ReplyDueDate   string    `json:"reply_due_date"`
	NextCheckDate  string    `json:"next_check_date"`
	CompleteNote   string    `json:"complete_note"`
	ReceiptDate    string    `json:"receipt_date"`  // 접수일 YYYY-MM-DD (§13.4)
	CompleteDate   string    `json:"complete_date"` // 완료일 YYYY-MM-DD. 완료 시 서버 기록
	RecurrenceRole    string `json:"recurrence_role,omitempty"`
	OccurrenceSeq     int    `json:"occurrence_seq,omitempty"`
	OccurrenceStatus  string `json:"occurrence_status,omitempty"`
	NotDoneReason     string `json:"not_done_reason,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	ProjectName  string `json:"project_name,omitempty"`
	ProjectColor string `json:"project_color,omitempty"`
	OrgName      string `json:"org_name,omitempty"` // 거래처 표시명(기관명 또는 직접입력)
	DaysLeft     int    `json:"days_left"`
	ChildCount   int    `json:"child_count,omitempty"`
	Depth        int    `json:"depth,omitempty"` // 목록 들여쓰기(1=상위)
	BoardHref    string `json:"board_href,omitempty"` // 칸반·목록 링크(원본 AS/점검 등)
	WaitingActionCount int `json:"waiting_action_count,omitempty"` // 목록 뱃지: 회신 대기 n건
}

// WorkTaskMember 업무 참여자 1명. work_task_members (§7.7.2).
// DurationMin 0 이면 업무 전체 소요시간을 따른다.
type WorkTaskMember struct {
	TaskID      string    `json:"task_id"`
	Assignee    string    `json:"assignee"`
	Role        string    `json:"member_role"`
	DurationMin int       `json:"duration_min"`
	SortOrder   int       `json:"sort_order"`
	CreatedAt   time.Time `json:"created_at"`
}

func (m WorkTaskMember) IsOwner() bool   { return m.Role == WBMemberOwner }
func (m WorkTaskMember) IsSupport() bool { return m.Role == WBMemberSupport }

// CustomerLabel 거래처 표시. 고객마스터 기관명 우선, 없으면 직접입력.
func (t WorkTask) CustomerLabel() string {
	if s := strings.TrimSpace(t.OrgName); s != "" {
		return s
	}
	return strings.TrimSpace(t.CustomerName)
}

// DetailHref 일일 업무 현황 카드/목록 링크
func (t WorkTask) DetailHref() string {
	if strings.TrimSpace(t.BoardHref) != "" {
		return t.BoardHref
	}
	if t.TaskID != "" && !strings.HasPrefix(t.TaskID, "ext:") {
		return "/workboard/tasks/" + t.TaskID
	}
	if t.SourceType == WBSourceAS && t.SourceID != "" {
		return "/as/" + t.SourceID
	}
	if t.SourceType == WBSourceMaintenance && t.SourceID != "" {
		return "/maintenance/visits/" + t.SourceID
	}
	return "#"
}

// WBCardAsWaitingTask 오늘 예정 팔레트 카드 → 오늘예정업무 열용 업무
func WBCardAsWaitingTask(c WBCard) WorkTask {
	workType := WBWorkAdmin
	sourceType := ""
	switch c.Category {
	case WBSourceAS: // = WBWorkAS
		workType = WBWorkAS
		sourceType = WBSourceAS
	case WBSourceMaintenance: // = WBWorkMaintenance
		workType = WBWorkMaintenance
		sourceType = WBSourceMaintenance
	case WBWorkSupport:
		workType = WBWorkSupport
	}
	taskID := strings.TrimSpace(c.TaskID)
	if taskID == "" {
		taskID = "ext:" + c.Kind + ":" + c.RefID
	}
	href := strings.TrimSpace(c.SourceHref)
	if href == "" && taskID != "" && !strings.HasPrefix(taskID, "ext:") {
		href = "/workboard/tasks/" + taskID
	}
	return WorkTask{
		TaskID:      taskID,
		WorkType:    workType,
		Title:       c.Title,
		Description: c.SubTitle,
		DueDate:     c.PlannedDay,
		WorkDate:    c.PlannedDay,
		Status:      WBTaskWaiting,
		Priority:    WBPriorityNormal,
		Assignee:    c.Assignee,
		SourceType:  sourceType,
		SourceID:    c.RefID,
		BoardHref:   href,
		DaysLeft:    0,
	}
}

type WBSummary struct {
	TotalTasks   int
	AS           int // 오늘 예정 AS
	Maintenance  int // 오늘 예정 정기점검
	Admin        int // 오늘 예정 행정·사업지원
	Waiting      int // 상태별(하위호환)
	InProgress   int
	Review       int
	Complete     int
	Urgent       int
	ProjectCount int
}

// WBWorkTypeBucket 일일 업무 현황 열 키: as / maintenance / admin(행정·지원)
func WBWorkTypeBucket(workType string) string {
	switch workType {
	case WBWorkAS:
		return WBWorkAS
	case WBWorkMaintenance:
		return WBWorkMaintenance
	default:
		return WBWorkAdmin // admin + support
	}
}

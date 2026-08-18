package model

import (
	"strings"
	"time"
)

// ASReceipt AS 접수 (기획서 §8)
type ASReceipt struct {
	ASID               string     `json:"as_id"`
	ASNumber           string     `json:"as_number"` // 자동생성 (AS-202600001)
	ReceiptDatetime    time.Time  `json:"receipt_datetime"`
	CustomerID         string     `json:"customer_id"`
	AssetID            string     `json:"asset_id"`
	ReceiptChannel     string     `json:"receipt_channel"` // 전화, 이메일, 방문, 협력사요청
	Requester          string     `json:"requester"`       // 요청자 실명
	Symptom            string     `json:"symptom"`         // 증상내용
	Urgency            string     `json:"urgency"`         // 상, 중, 하
	Priority           string     `json:"priority"`
	RequesterType      string     `json:"requester_type"` // 고객직접, 제조사, 협력사 등
	RequesterName      string     `json:"requester_name"`
	AssignedTo         string     `json:"assigned_to"`          // 배정 담당자 표시명
	AssignedUserID     string     `json:"assigned_user_id"`     // 배정 사용자 ID
	ReceivedBy         string     `json:"received_by"`          // 접수자
	VisitScheduledDate string     `json:"visit_scheduled_date"` // 예정업무일 (YYYY-MM-DD, 선택)
	ScheduleConfirmed  bool       `json:"schedule_confirmed"`   // 일정 확정 시 진행중
	Status             string     `json:"status"`               // 접수, 담당자배정, 진행중, 보류, 이관, 접수취소, 완료, 종료
	StartDatetime      *time.Time `json:"start_datetime"`
	CompleteDatetime   *time.Time `json:"complete_datetime"`
	ProcessType        string     `json:"process_type"` // 원격지원, 방문, 교체 등
	WorkPlace          string     `json:"work_place"`   // office=내근, field=외근
	CauseType          string     `json:"cause_type"`   // HW고장, SW오류, 네트워크 등 (통계용 코드)
	CauseDetail        string     `json:"cause_detail"` // 장애원인 서술. cause_type 과 별개 (§12.10.4)
	Conclusion         string     `json:"conclusion"`   // 결론 서술. 보고서용 초안·수정 (§12.10.4)
	ActionTaken        string     `json:"action_taken"` // 조치내용
	PartsUsed          string     `json:"parts_used"`
	IsRecurrence       bool       `json:"is_recurrence"`    // 재발여부
	IsReopen           bool       `json:"is_reopen"`        // 재접수여부 (완료 건의 동일 증상 재접수)
	ParentASID         string     `json:"parent_as_id"`     // 재접수의 원 접수번호
	ReopenReason       string     `json:"reopen_reason"`    // 재접수 사유
	ResultCode         string     `json:"result_code"`      // 완료, 타사이관, 재방문필요
	TransferDetail     string     `json:"transfer_detail"`  // completed | waiting
	ConfirmTarget      string     `json:"confirm_target"`   // 확인대상자
	ConfirmContact     string     `json:"confirm_contact"`  // 연락처
	RevisitReason      string     `json:"revisit_reason"`   // 재방문 사유 (재방문필요 시)
	HoldReason         string     `json:"hold_reason"`      // 보류 사유
	HoldNextAction     string     `json:"hold_next_action"` // 보류 시 선택: action|transfer|cancel
	CancelDatetime     *time.Time `json:"cancel_datetime"`  // 접수취소 일자
	CustomerConfirmer  string     `json:"customer_confirmer"`
	ConfirmDatetime    *time.Time `json:"confirm_datetime"`
	FollowupAction     string     `json:"followup_action"` // 후속조치
	ReplaceReview      bool       `json:"replace_review"`  // 교체검토여부
	ProjectID          string     `json:"project_id"`      // 사업 귀속. 생성·수정 시 ResolveProjectID로 저장 (§16.6.9)
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`

	// JOIN용
	OrgName         string `json:"org_name,omitempty"`
	ProductName     string `json:"product_name,omitempty"`
	InstallLocation string `json:"install_location,omitempty"` // 연결 자산의 설치위치
}

// ASHistoryItem 기관·자산 AS 이력 리스트
type ASHistoryItem struct {
	ASID         string `json:"as_id"`
	ASNumber     string `json:"as_number"`
	ReceiptDate  string `json:"receipt_date"`  // YYYY-MM-DD
	VisitDate    string `json:"visit_date"`    // 방문/예정업무일
	CompleteDate string `json:"complete_date"` // 최종완료일
	Visitor      string `json:"visitor"`       // 방문자(처리담당)
	Symptom      string `json:"symptom"`
	ActionTaken  string `json:"action_taken"`
	DurationText string `json:"duration_text"` // 접수→완료 소요기간
	Status       string `json:"status,omitempty"`
	StatusLabel  string `json:"status_label,omitempty"`
}

// ASProcess AS 처리 이력 (접수 1건에 N개 처리 기록 가능)
type ASProcess struct {
	ProcessID       string    `json:"process_id"`
	ProcessNumber   string    `json:"process_number"`
	ASID            string    `json:"as_id"`
	ProcessDatetime time.Time `json:"process_datetime"`
	Worker          string    `json:"worker"`
	WorkType        string    `json:"work_type"`
	WorkContent     string    `json:"work_content"`
	PartsUsed       string    `json:"parts_used"`
	TimeSpent       int       `json:"time_spent"` // 분 단위
	Notes           string    `json:"notes"`
	ResultCode      string    `json:"result_code"`      // 이 회차 조치 결과
	TransferDetail  string    `json:"transfer_detail"`  // completed | waiting
	NextActionDate  string    `json:"next_action_date"` // 다음 작업 예정일
	WaitReason      string    `json:"wait_reason"`      // 대기·재방문 사유
	PrepNotes       string    `json:"prep_notes"`       // 재방문 준비사항
}

// ASListItem AS 목록 표시용
type ASListItem struct {
	ASID               string       `json:"as_id"`
	ASNumber           string       `json:"as_number"`
	ReceiptDatetime    time.Time    `json:"receipt_datetime"`
	OrgName            string       `json:"org_name"`
	ProductName        string       `json:"product_name"`
	Symptom            string       `json:"symptom"`
	Urgency            string       `json:"urgency"`
	Status             string       `json:"status"`
	AssignedTo         string       `json:"assigned_to"`
	DaysElapsed        int          `json:"days_elapsed"`            // 경과일: 완료=완료일−접수일, 미완료=오늘−접수일
	VisitScheduledDate string       `json:"visit_scheduled_date"`    // 예정업무일 YYYY-MM-DD
	VisitDaysOverdue   int          `json:"visit_days_overdue"`      // 방문일 기준 경과(오늘-예정일, 양수=지남)
	VisitDone          bool         `json:"visit_done"`              // 예정일 이후 방문(처리) 이력이 있음 — 경과가 아니라 다음 일정 미정
	IsReopen           bool         `json:"is_reopen"`               // 완료 건의 동일 증상 재접수
	WorkChildren       []ASWorkItem `json:"work_children,omitempty"` // 목록 들여쓰기용 하부업무
	PhotoCount         int          `json:"photo_count,omitempty"`   // 접수 사진 수 (목록 뱃지)
}

// ASWorkItem 접수 하부 확인·재방문 업무 ({접수번호}-Wnn).
// 담당자·완료는 원 접수와 독립이다(원 접수는 참조만).
type ASWorkItem struct {
	WorkID            string    `json:"work_id"`
	WorkNumber        string    `json:"work_number"`
	ASID              string    `json:"as_id"`
	WorkKind          string    `json:"work_kind"` // confirm | revisit
	ScheduledDate     string    `json:"scheduled_date"`
	ScheduleConfirmed bool      `json:"schedule_confirmed"`
	ConfirmTarget     string    `json:"confirm_target"`
	ConfirmContact    string    `json:"confirm_contact"`
	AssignedTo        string    `json:"assigned_to"`
	AssignedUserID    string    `json:"assigned_user_id"`
	Status            string    `json:"status"` // open | done
	Notes             string    `json:"notes"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// WorkKind labels
const (
	WorkKindConfirm = "confirm"
	WorkKindRevisit = "revisit"
)

func WorkKindLabel(kind string) string {
	switch kind {
	case WorkKindConfirm:
		return "확인"
	case WorkKindRevisit:
		return "재방문"
	default:
		return kind
	}
}

// ASStats AS 현황 통계
type ASStats struct {
	TotalReceived   int `json:"total_received"`
	Assigned        int `json:"assigned"` // 담당자 배정
	InProgress      int `json:"in_progress"`
	Completed       int `json:"completed"` // 대시보드: 오늘 완료
	Overdue         int `json:"overdue"`   // 접수 지연(3일↑)
	TodayReceived   int `json:"today_received"`
	WeekReceived    int `json:"week_received"`    // 월~일 주간 접수
	WeekCompleted   int `json:"week_completed"`   // 월~일 주간 완료
	VisitPast       int `json:"visit_past"`       // 진행중 · 예정일 < 오늘 · 방문 이력 없음
	VisitDoneOpen   int `json:"visit_done_open"`  // 진행중 · 예정일 < 오늘 · 이미 다녀옴(다음 일정 미정)
	VisitToday      int `json:"visit_today"`      // 진행중 · 예정일 = 오늘
	VisitUpcoming   int `json:"visit_upcoming"`   // 진행중 · 예정일 > 오늘
	TransferOverdue int `json:"transfer_overdue"` // 이관 · 회신확인일 없음/경과
}

// AssigneeDashStats 대시보드 담당자별 주간/일간 지표
type AssigneeDashStats struct {
	UserID          string  `json:"user_id"`
	Name            string  `json:"name"`
	Username        string  `json:"username"`
	WeekInProgress  int     `json:"week_in_progress"`  // 이번 주 접수분 중 미완료
	WeekCompleted   int     `json:"week_completed"`    // 이번 주 완료 합계
	DayAvgCompleted float64 `json:"day_avg_completed"` // 일평균 완료(주간완료÷월~오늘 일수)
	DayCompleted    int     `json:"day_completed"`     // 오늘 완료(참고)
	Overdue         int     `json:"overdue"`           // 지연(전체)
}

const (
	WorkPlaceOffice = "office" // 내근
	WorkPlaceField  = "field"  // 외근
)

// WorkPlaceLabel 조치 근무구분 표시.
func WorkPlaceLabel(s string) string {
	switch strings.TrimSpace(s) {
	case WorkPlaceOffice, "내근":
		return "내근"
	case WorkPlaceField, "외근":
		return "외근"
	default:
		return ""
	}
}

// BuildASConclusionDraft 완료 시 결론 초안. 빈 칸은 그대로 두고 문장을 강제하지 않는다. §12.10.4
func BuildASConclusionDraft(causeDetail, symptom, workContent string) string {
	causeDetail = strings.TrimSpace(causeDetail)
	symptom = strings.TrimSpace(symptom)
	workContent = strings.TrimSpace(workContent)
	if causeDetail == "" && symptom == "" && workContent == "" {
		return ""
	}
	if symptom == "" {
		symptom = "해당"
	}
	if workContent == "" {
		workContent = "조치"
	}
	if causeDetail == "" {
		return symptom + " 문제가 발생했고, " + workContent + "하여 정상작동하는 것으로 확인됨."
	}
	return causeDetail + koreanEuro(causeDetail) + " " + symptom + " 문제가 발생했고, " + workContent + "하여 정상작동하는 것으로 확인됨."
}

// koreanEuro 받침에 따라 (으)로.
func koreanEuro(s string) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) == 0 {
		return "로"
	}
	last := r[len(r)-1]
	if last < 0xAC00 || last > 0xD7A3 {
		return "로"
	}
	jong := (last - 0xAC00) % 28
	if jong == 0 || jong == 8 { // 없음 · ㄹ
		return "로"
	}
	return "으로"
}

// NormalizeWorkPlace office/field 만 허용.
func NormalizeWorkPlace(s string) string {
	switch strings.TrimSpace(s) {
	case WorkPlaceOffice, "내근":
		return WorkPlaceOffice
	case WorkPlaceField, "외근":
		return WorkPlaceField
	default:
		return ""
	}
}

package model

import "time"

// ASReceipt AS 접수 (기획서 §8)
type ASReceipt struct {
	ASID              string    `json:"as_id"`
	ASNumber          string    `json:"as_number"`          // 자동생성 (AS-202600001)
	ReceiptDatetime   time.Time `json:"receipt_datetime"`
	CustomerID        string    `json:"customer_id"`
	AssetID           string    `json:"asset_id"`
	ReceiptChannel    string    `json:"receipt_channel"`    // 전화, 이메일, 방문, 협력사요청
	Requester         string    `json:"requester"`          // 요청자 실명
	Symptom           string    `json:"symptom"`            // 증상내용
	Urgency           string    `json:"urgency"`            // 상, 중, 하
	Priority          string    `json:"priority"`
	RequesterType     string    `json:"requester_type"`     // 고객직접, 제조사, 협력사 등
	RequesterName     string    `json:"requester_name"`
	AssignedTo        string    `json:"assigned_to"`         // 배정 담당자 표시명
	AssignedUserID    string    `json:"assigned_user_id"`    // 배정 사용자 ID
	ReceivedBy        string    `json:"received_by"`         // 접수자
	VisitScheduledDate string   `json:"visit_scheduled_date"` // 예정업무일 (YYYY-MM-DD, 선택)
	ScheduleConfirmed  bool     `json:"schedule_confirmed"`  // 일정 확정 시 진행중
	Status            string    `json:"status"`             // 접수, 담당자배정, 진행중, 보류, 이관, 접수취소, 완료, 종료
	StartDatetime     *time.Time `json:"start_datetime"`
	CompleteDatetime  *time.Time `json:"complete_datetime"`
	ProcessType       string    `json:"process_type"`       // 원격지원, 방문, 교체 등
	CauseType         string    `json:"cause_type"`         // HW고장, SW오류, 네트워크 등
	ActionTaken       string    `json:"action_taken"`       // 조치내용
	PartsUsed         string    `json:"parts_used"`
	IsRecurrence      bool      `json:"is_recurrence"`      // 재발여부
	IsReopen          bool      `json:"is_reopen"`          // 재오픈여부
	ResultCode        string    `json:"result_code"`        // 완료, 타사이관, 재방문필요
	TransferDetail    string    `json:"transfer_detail"`    // completed | waiting
	ConfirmTarget     string    `json:"confirm_target"`     // 확인대상자
	ConfirmContact    string    `json:"confirm_contact"`    // 연락처
	RevisitReason     string    `json:"revisit_reason"`     // 재방문 사유 (재방문필요 시)
	HoldReason        string    `json:"hold_reason"`        // 보류 사유
	HoldNextAction    string    `json:"hold_next_action"`   // 보류 시 선택: action|transfer|cancel
	CancelDatetime    *time.Time `json:"cancel_datetime"`   // 접수취소 일자
	CustomerConfirmer string    `json:"customer_confirmer"`
	ConfirmDatetime   *time.Time `json:"confirm_datetime"`
	FollowupAction    string    `json:"followup_action"`    // 후속조치
	ReplaceReview     bool      `json:"replace_review"`     // 교체검토여부
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`

	// JOIN용
	OrgName          string `json:"org_name,omitempty"`
	ProductName      string `json:"product_name,omitempty"`
	InstallLocation  string `json:"install_location,omitempty"` // 연결 자산의 설치위치
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
}

// ASListItem AS 목록 표시용
type ASListItem struct {
	ASID               string    `json:"as_id"`
	ASNumber           string    `json:"as_number"`
	ReceiptDatetime    time.Time `json:"receipt_datetime"`
	OrgName            string    `json:"org_name"`
	ProductName        string    `json:"product_name"`
	Symptom            string    `json:"symptom"`
	Urgency            string    `json:"urgency"`
	Status             string    `json:"status"`
	AssignedTo         string    `json:"assigned_to"`
	DaysElapsed        int       `json:"days_elapsed"`         // 접수 경과일
	VisitScheduledDate string    `json:"visit_scheduled_date"` // 예정업무일 YYYY-MM-DD
	VisitDaysOverdue   int       `json:"visit_days_overdue"`   // 방문일 기준 경과(오늘-예정일, 양수=지남)
	WorkChildren       []ASWorkItem `json:"work_children,omitempty"` // 목록 들여쓰기용 하부업무
}

// ASWorkItem 접수 하부 확인·재방문 업무 ({접수번호}-Wnn)
type ASWorkItem struct {
	WorkID            string    `json:"work_id"`
	WorkNumber        string    `json:"work_number"`
	ASID              string    `json:"as_id"`
	WorkKind          string    `json:"work_kind"` // confirm | revisit
	ScheduledDate     string    `json:"scheduled_date"`
	ScheduleConfirmed bool      `json:"schedule_confirmed"`
	ConfirmTarget     string    `json:"confirm_target"`
	ConfirmContact    string    `json:"confirm_contact"`
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
	TotalReceived  int `json:"total_received"`
	InProgress     int `json:"in_progress"`
	Completed      int `json:"completed"` // 대시보드: 오늘 완료
	Overdue        int `json:"overdue"`   // 접수 지연(3일↑)
	TodayReceived  int `json:"today_received"`
	WeekReceived   int `json:"week_received"`  // 월~일 주간 접수
	WeekCompleted  int `json:"week_completed"` // 월~일 주간 완료
	VisitPast      int `json:"visit_past"`      // 진행중 · 예정일 < 오늘
	VisitToday     int `json:"visit_today"`     // 진행중 · 예정일 = 오늘
	VisitUpcoming  int `json:"visit_upcoming"`  // 진행중 · 예정일 > 오늘
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

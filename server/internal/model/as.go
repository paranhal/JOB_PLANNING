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
	AssignedTo        string    `json:"assigned_to"`        // 배정 담당자
	Status            string    `json:"status"`             // 접수, 진행중, 보류, 완료, 종료
	StartDatetime     *time.Time `json:"start_datetime"`
	CompleteDatetime  *time.Time `json:"complete_datetime"`
	ProcessType       string    `json:"process_type"`       // 원격지원, 방문, 교체 등
	CauseType         string    `json:"cause_type"`         // HW고장, SW오류, 네트워크 등
	ActionTaken       string    `json:"action_taken"`       // 조치내용
	PartsUsed         string    `json:"parts_used"`
	IsRecurrence      bool      `json:"is_recurrence"`      // 재발여부
	ResultCode        string    `json:"result_code"`        // 완료, 임시조치, 타사이관 등
	CustomerConfirmer string    `json:"customer_confirmer"`
	ConfirmDatetime   *time.Time `json:"confirm_datetime"`
	FollowupAction    string    `json:"followup_action"`    // 후속조치
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`

	// JOIN용
	OrgName     string `json:"org_name,omitempty"`
	ProductName string `json:"product_name,omitempty"`
}

// ASProcess AS 처리 이력 (접수 1건에 N개 처리 기록 가능)
type ASProcess struct {
	ProcessID       string    `json:"process_id"`
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
	ASID            string    `json:"as_id"`
	ASNumber        string    `json:"as_number"`
	ReceiptDatetime time.Time `json:"receipt_datetime"`
	OrgName         string    `json:"org_name"`
	ProductName     string    `json:"product_name"`
	Symptom         string    `json:"symptom"`
	Urgency         string    `json:"urgency"`
	Status          string    `json:"status"`
	AssignedTo      string    `json:"assigned_to"`
	DaysElapsed     int       `json:"days_elapsed"` // 처리 경과일
}

// ASStats AS 현황 통계
type ASStats struct {
	TotalReceived  int `json:"total_received"`
	InProgress     int `json:"in_progress"`
	Completed      int `json:"completed"`
	Overdue        int `json:"overdue"`       // 지연 건수
	TodayReceived  int `json:"today_received"`
}

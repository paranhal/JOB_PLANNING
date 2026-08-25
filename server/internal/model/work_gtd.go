package model

import (
	"sort"
	"strings"
	"time"
)

// 행정·지원 GTD 상태 (§10.3.2). AS·정기점검은 쓰지 않는다.
const (
	WBTaskInbox      = "inbox"       // 수집함
	WBTaskWaitingFor = "waiting_for" // 회신 대기
	WBTaskCancelled  = "cancelled"   // 취소(삭제하지 않음)
)

// 다음 행동 상태
const (
	WBActionTodo       = "todo"
	WBActionInProgress = "in_progress"
	WBActionWaiting    = "waiting"
	WBActionComplete   = "complete"
	WBActionCancelled  = "cancelled"
)

// 조치 이력 유형
const (
	WBActivityWrite   = "write"
	WBActivityEdit    = "edit"
	WBActivityReview  = "review_request"
	WBActivityReply   = "reply"
	WBActivitySend    = "send"
	WBActivityFollow  = "followup"
	WBActivityDone    = "complete"
	WBActivityOther   = "other"
)

// 회신 대기 대상 구분
const (
	WBWaitInternal = "internal"
	WBWaitDept     = "dept"
	WBWaitVendor   = "vendor"
	WBWaitCustomer = "customer"
)

// WorkAction 행정/지원 다음 행동 (업무 1:N)
type WorkAction struct {
	ActionID       string `json:"action_id"`
	TaskID         string `json:"task_id"`
	Title          string `json:"title"`
	Status         string `json:"status"`
	Required       bool   `json:"required"`
	ScheduledDate  string `json:"scheduled_date"`
	DueDate        string `json:"due_date"`
	Assignee       string `json:"assignee"`
	WaitPartyKind  string `json:"wait_party_kind"`
	WaitParty      string `json:"wait_party"`
	WaitRequest    string `json:"wait_request"`
	ReplyDueDate   string `json:"reply_due_date"`
	NextCheckDate  string `json:"next_check_date"`
	Confirmed      bool   `json:"confirmed"`
	SortOrder      int    `json:"sort_order"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// WorkActivity 행정/지원 조치 이력 (덮어쓰지 않고 추가)
type WorkActivity struct {
	ActivityID    string `json:"activity_id"`
	TaskID        string `json:"task_id"`
	ActionID      string `json:"action_id"`
	ActivityType  string `json:"activity_type"`
	Content       string `json:"content"`
	Actor         string `json:"actor"`
	SpentMinutes  int    `json:"spent_minutes"`
	CreatedAt     string `json:"created_at"`
}

// IsAdminGTDTask 행정·지원 직접 등록 건만 GTD 규칙을 적용한다.
func IsAdminGTDTask(t WorkTask) bool {
	if strings.TrimSpace(t.SourceType) != "" {
		return false
	}
	switch t.WorkType {
	case WBWorkAdmin, WBWorkSupport, "":
		return true
	default:
		return false
	}
}

// WBAdminStatusLabel 행정·지원 화면 문구. waiting = 「할 일」(AS 「오늘예정업무」와 구분).
func WBAdminStatusLabel(s string) string {
	switch s {
	case WBTaskInbox:
		return "수집함"
	case WBTaskWaiting:
		return "할 일"
	case WBTaskInProgress:
		return "진행중"
	case WBTaskWaitingFor:
		return "회신 대기"
	case WBTaskHold:
		return "보류"
	case WBTaskTransfer:
		return "이관"
	case WBTaskReview:
		return "검토중"
	case WBTaskComplete:
		return "완료"
	case WBTaskCancelled:
		return "취소"
	default:
		return WBTaskStatusLabel(s)
	}
}

func WBActionStatusLabel(s string) string {
	switch s {
	case WBActionTodo:
		return "할 일"
	case WBActionInProgress:
		return "진행중"
	case WBActionWaiting:
		return "대기"
	case WBActionComplete:
		return "완료"
	case WBActionCancelled:
		return "취소"
	default:
		return s
	}
}

func WBActivityTypeLabel(s string) string {
	switch s {
	case WBActivityWrite:
		return "작성"
	case WBActivityEdit:
		return "수정"
	case WBActivityReview:
		return "검토 요청"
	case WBActivityReply:
		return "회신"
	case WBActivitySend:
		return "발송"
	case WBActivityFollow:
		return "독촉"
	case WBActivityDone:
		return "완료"
	case WBActivityOther:
		return "기타"
	default:
		return s
	}
}

func WBWaitPartyKindLabel(s string) string {
	switch s {
	case WBWaitInternal:
		return "내부 담당"
	case WBWaitDept:
		return "내부 부서"
	case WBWaitVendor:
		return "외부 업체"
	case WBWaitCustomer:
		return "고객"
	default:
		return s
	}
}

func WBActionOpen(status string) bool {
	switch status {
	case WBActionComplete, WBActionCancelled:
		return false
	default:
		return true
	}
}

// ActionProgressPct 완료 필수 다음 행동 ÷ 필수 다음 행동. 분모 0이면 표시하지 않는다(§4.3·§13.6).
// 산식은 PlanningRatePct를 재사용한다.
func ActionProgressPct(doneRequired, required int) (pct float64, ok bool) {
	if !HasPlanningRate(required) {
		return 0, false
	}
	return PlanningRatePct(doneRequired, required), true
}

func (a WorkAction) ReplyOverdue(today string) bool {
	if a.Status != WBActionWaiting || a.Confirmed {
		return false
	}
	due := strings.TrimSpace(a.ReplyDueDate)
	return due != "" && today != "" && due < today
}

// WBActivityDefaultSpent 조치 이력 소요시간 기본값(분). AS time_spent 기본 30분과 같다.
const WBActivityDefaultSpent = 30

func IsExternalWaitStart(typ string) bool {
	switch strings.TrimSpace(typ) {
	case WBActivitySend, WBActivityReview, WBActivityFollow:
		return true
	default:
		return false
	}
}

func IsExternalWaitEnd(typ string) bool {
	return strings.TrimSpace(typ) == WBActivityReply
}

// DateInterval 요청→회신 대기 구간.
type DateInterval struct {
	Start time.Time
	End   time.Time
}

func ParseActivityTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{
		"2006-01-02 15:04:05",
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02",
	} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, true
		}
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// MergeDateIntervals 겹치거나 맞닿은 구간을 하나로 합친다(§13.10).
func MergeDateIntervals(in []DateInterval) []DateInterval {
	var valid []DateInterval
	for _, iv := range in {
		if iv.Start.IsZero() || iv.End.IsZero() || !iv.End.After(iv.Start) {
			continue
		}
		valid = append(valid, iv)
	}
	if len(valid) == 0 {
		return nil
	}
	sort.Slice(valid, func(i, j int) bool {
		if valid[i].Start.Equal(valid[j].Start) {
			return valid[i].End.Before(valid[j].End)
		}
		return valid[i].Start.Before(valid[j].Start)
	})
	out := []DateInterval{valid[0]}
	for _, iv := range valid[1:] {
		last := &out[len(out)-1]
		if !iv.Start.After(last.End) {
			if iv.End.After(last.End) {
				last.End = iv.End
			}
			continue
		}
		out = append(out, iv)
	}
	return out
}

func IntervalDays(in []DateInterval) float64 {
	var sum float64
	for _, iv := range MergeDateIntervals(in) {
		sum += iv.End.Sub(iv.Start).Hours() / 24
	}
	if sum < 0 {
		return 0
	}
	return sum
}

// PairActionWaitIntervals 한 다음 행동의 요청→회신 구간. 미회신은 closeAt 까지.
func PairActionWaitIntervals(acts []WorkActivity, fallbackStart, closeAt time.Time) []DateInterval {
	type ev struct {
		at    time.Time
		start bool
	}
	var events []ev
	for _, a := range acts {
		at, ok := ParseActivityTime(a.CreatedAt)
		if !ok {
			continue
		}
		if IsExternalWaitStart(a.ActivityType) {
			events = append(events, ev{at: at, start: true})
		} else if IsExternalWaitEnd(a.ActivityType) {
			events = append(events, ev{at: at, start: false})
		}
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].at.Equal(events[j].at) {
			return events[i].start && !events[j].start
		}
		return events[i].at.Before(events[j].at)
	})
	var out []DateInterval
	var open time.Time
	opened := false
	for _, e := range events {
		if e.start {
			if !opened {
				open = e.at
				opened = true
			}
			continue
		}
		if opened && e.at.After(open) {
			out = append(out, DateInterval{Start: open, End: e.at})
		}
		opened = false
	}
	if opened {
		end := closeAt
		if end.IsZero() || !end.After(open) {
			end = open
		}
		if end.After(open) {
			out = append(out, DateInterval{Start: open, End: end})
		}
	}
	if len(out) == 0 && !fallbackStart.IsZero() && !closeAt.IsZero() && closeAt.After(fallbackStart) {
		out = append(out, DateInterval{Start: fallbackStart, End: closeAt})
	}
	return out
}

// ExternalWaitSharePct 외부 대기 일수 ÷ 총 리드타임 일수. 분모 0이면 표시하지 않는다.
// 분·일 환산 후 PlanningRatePct를 재사용한다.
func ExternalWaitSharePct(waitDays, leadDays float64) (pct float64, ok bool) {
	if leadDays < 0 {
		leadDays = 0
	}
	if waitDays < 0 {
		waitDays = 0
	}
	leadMin := int(leadDays*24*60 + 0.5)
	waitMin := int(waitDays*24*60 + 0.5)
	if !HasPlanningRate(leadMin) {
		return 0, false
	}
	if waitMin > leadMin {
		waitMin = leadMin
	}
	return PlanningRatePct(waitMin, leadMin), true
}

// AdminLeadBreakdown 행정·지원 리드타임 분리(§13.10).
type AdminLeadBreakdown struct {
	Sample         int
	LeadDaysAvg    float64
	LeadDaysSum    float64
	WorkMinutes    int
	WaitDaysSum    float64
	IdleMinutes    int
	LeadDisplay    StatsValue
	WaitShareDisplay StatsValue
}

func (b *AdminLeadBreakdown) FillDisplays() {
	if b == nil {
		return
	}
	b.LeadDisplay = StatsReliability(b.Sample, b.Sample == 0, b.LeadDaysAvg)
	if b.Sample == 0 {
		b.LeadDisplay.Reason = "완료된 행정·지원 업무 없음"
	}
	pct, ok := ExternalWaitSharePct(b.WaitDaysSum, b.LeadDaysSum)
	b.WaitShareDisplay = StatsReliability(b.Sample, !ok, pct)
	if !ok {
		b.WaitShareDisplay.Reason = "총 리드타임 없음"
	}
}

func AdminLeadTimeDays(receipt, complete string) (int, bool) {
	receipt = strings.TrimSpace(receipt)
	complete = strings.TrimSpace(complete)
	if receipt == "" || complete == "" {
		return 0, false
	}
	a, err1 := time.ParseInLocation("2006-01-02", receipt, time.Local)
	b, err2 := time.ParseInLocation("2006-01-02", complete, time.Local)
	if err1 != nil || err2 != nil {
		return 0, false
	}
	d := int(b.Sub(a).Hours() / 24)
	if d < 0 {
		d = 0
	}
	return d, true
}

// AdminGTDErr 상태 전환 검증 결과 코드(리다이렉트 err=).
func AdminGTDErr(status, holdReason, reviewDate, cancelReason, waitParty, waitRequest, replyDue, nextCheck, completeNote string, openRequired, unconfirmedWait int, isAdmin, force bool, forceReason string) string {
	switch strings.TrimSpace(status) {
	case WBTaskHold:
		if strings.TrimSpace(holdReason) == "" || strings.TrimSpace(reviewDate) == "" {
			return "hold_required"
		}
	case WBTaskWaitingFor:
		if strings.TrimSpace(waitParty) == "" || strings.TrimSpace(waitRequest) == "" {
			return "waiting_for"
		}
		if strings.TrimSpace(replyDue) == "" && strings.TrimSpace(nextCheck) == "" {
			return "waiting_for"
		}
	case WBTaskCancelled:
		if strings.TrimSpace(cancelReason) == "" {
			return "cancel_reason"
		}
	case WBTaskComplete:
		if strings.TrimSpace(completeNote) == "" {
			return "complete_note"
		}
		blocked := openRequired > 0 || unconfirmedWait > 0
		if blocked {
			if !isAdmin || !force {
				return "complete_block"
			}
			if strings.TrimSpace(forceReason) == "" {
				return "force_reason"
			}
		}
	}
	return ""
}

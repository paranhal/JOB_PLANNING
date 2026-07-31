package model

import (
	"errors"
	"strings"
	"time"
)

// DeriveASWorkflowStatus 접수·배정·일정확정 기준 워크플로 상태
func DeriveASWorkflowStatus(assignedTo, assignedUserID string, scheduleConfirmed bool) string {
	if scheduleConfirmed {
		return "in_progress"
	}
	if strings.TrimSpace(assignedTo) != "" || strings.TrimSpace(assignedUserID) != "" {
		return "assigned"
	}
	return "received"
}

// IsASWorkflowStatus 자동 파생 대상 상태(특수·완료 상태 제외)
func IsASWorkflowStatus(status string) bool {
	switch status {
	case "", "received", "assigned", "in_progress":
		return true
	default:
		return false
	}
}

// 처리결과코드 (조치 화면: 완료 · 타사이관 · 재방문필요)
const (
	ResultDone    = "done"
	ResultTransfer = "transfer"
	ResultRevisit = "revisit_needed"
	// 하위 호환(더 이상 조치 UI에 노출하지 않음)
	ResultTemporary  = "temporary"
	ResultEscalation = "escalation"
)

// 타사이관 세부
const (
	TransferDetailCompleted = "completed" // 이관 완료 → 접수 완료
	TransferDetailWaiting   = "waiting"   // 조치 결과 대기 → 진행중 + 확인 하부업무
)

var (
	ErrRevisitReasonRequired   = errors.New("revisit_reason_required")
	ErrRevisitDateRequired     = errors.New("revisit_date_required")
	ErrTemporaryDateRequired   = errors.New("temporary_date_required")
	ErrCompleteDateRequired    = errors.New("complete_date_required")
	ErrTransferDetailRequired  = errors.New("transfer_detail_required")
	ErrConfirmDateRequired     = errors.New("confirm_date_required")
	ErrConfirmTargetRequired   = errors.New("confirm_target_required")
	ErrConfirmContactRequired  = errors.New("confirm_contact_required")
)

// ActionApplyInput 조치 저장 시 결과코드별 부가 입력
type ActionApplyInput struct {
	NextDate          string // 방문예정일 또는 확인예정일
	ScheduleConfirmed bool
	TransferDetail    string // completed | waiting
	ConfirmTarget     string
	ConfirmContact    string
	CreateRevisitAfterConfirm bool // 결과 대기 후 재방문 일정 등록
	RevisitDate               string
	RevisitScheduleConfirmed  bool
}

// ActionApplyResult 조치 반영 후 생성할 하부업무
type ActionApplyResult struct {
	WorkItems []ASWorkItemDraft
}

// ASWorkItemDraft 하부업무 생성 초안
type ASWorkItemDraft struct {
	WorkKind          string
	ScheduledDate     string
	ScheduleConfirmed bool
	ConfirmTarget     string
	ConfirmContact    string
	Notes             string
}

// ApplyActionResult 조치 결과코드에 따라 상태·예정일을 반영하고 하부업무 초안을 반환한다.
func ApplyActionResult(as *ASReceipt, in ActionApplyInput, now time.Time) (*ActionApplyResult, error) {
	code := strings.TrimSpace(as.ResultCode)
	in.NextDate = strings.TrimSpace(in.NextDate)
	in.TransferDetail = strings.TrimSpace(in.TransferDetail)
	in.ConfirmTarget = strings.TrimSpace(in.ConfirmTarget)
	in.ConfirmContact = strings.TrimSpace(in.ConfirmContact)
	in.RevisitDate = strings.TrimSpace(in.RevisitDate)
	out := &ActionApplyResult{}

	switch code {
	case ResultDone:
		as.Status = "completed"
		as.RevisitReason = ""
		if as.CompleteDatetime == nil || as.CompleteDatetime.IsZero() {
			t := now
			as.CompleteDatetime = &t
		}

	case ResultTransfer:
		switch in.TransferDetail {
		case TransferDetailCompleted:
			as.Status = "completed"
			as.RevisitReason = ""
			if as.CompleteDatetime == nil || as.CompleteDatetime.IsZero() {
				t := now
				as.CompleteDatetime = &t
			}
		case TransferDetailWaiting:
			if in.NextDate == "" {
				return nil, ErrConfirmDateRequired
			}
			if in.ConfirmTarget == "" {
				return nil, ErrConfirmTargetRequired
			}
			if in.ConfirmContact == "" {
				return nil, ErrConfirmContactRequired
			}
			as.Status = "in_progress"
			as.RevisitReason = ""
			as.CompleteDatetime = nil
			as.VisitScheduledDate = in.NextDate
			as.ScheduleConfirmed = in.ScheduleConfirmed
			out.WorkItems = append(out.WorkItems, ASWorkItemDraft{
				WorkKind:          WorkKindConfirm,
				ScheduledDate:     in.NextDate,
				ScheduleConfirmed: in.ScheduleConfirmed,
				ConfirmTarget:     in.ConfirmTarget,
				ConfirmContact:    in.ConfirmContact,
				Notes:             "타사이관·조치 결과 대기",
			})
			// 결과가 나와 재방문 일정을 함께 넣는 경우
			if in.CreateRevisitAfterConfirm {
				if in.RevisitDate == "" {
					return nil, ErrRevisitDateRequired
				}
				as.VisitScheduledDate = in.RevisitDate
				as.ScheduleConfirmed = in.RevisitScheduleConfirmed
				out.WorkItems = append(out.WorkItems, ASWorkItemDraft{
					WorkKind:          WorkKindRevisit,
					ScheduledDate:     in.RevisitDate,
					ScheduleConfirmed: in.RevisitScheduleConfirmed,
					Notes:             "이관 결과 수신 후 재방문",
				})
			}
		default:
			return nil, ErrTransferDetailRequired
		}

	case ResultRevisit:
		as.RevisitReason = strings.TrimSpace(as.RevisitReason)
		if in.NextDate == "" {
			return nil, ErrRevisitDateRequired
		}
		as.VisitScheduledDate = in.NextDate
		as.ScheduleConfirmed = in.ScheduleConfirmed
		as.Status = "in_progress"
		as.CompleteDatetime = nil
		out.WorkItems = append(out.WorkItems, ASWorkItemDraft{
			WorkKind:          WorkKindRevisit,
			ScheduledDate:     in.NextDate,
			ScheduleConfirmed: in.ScheduleConfirmed,
			Notes:             as.RevisitReason,
		})

	case ResultTemporary:
		// 하위 호환: 진행중 + 방문일(있으면)
		as.Status = "in_progress"
		as.RevisitReason = ""
		if in.NextDate == "" {
			return nil, ErrTemporaryDateRequired
		}
		as.VisitScheduledDate = in.NextDate
		as.ScheduleConfirmed = in.ScheduleConfirmed
		out.WorkItems = append(out.WorkItems, ASWorkItemDraft{
			WorkKind:          WorkKindRevisit,
			ScheduledDate:     in.NextDate,
			ScheduleConfirmed: in.ScheduleConfirmed,
			Notes:             "임시조치(호환)",
		})

	case ResultEscalation:
		// 하위 호환: 조치 결과 대기와 동일하게 진행중+확인
		if in.NextDate == "" {
			in.NextDate = now.AddDate(0, 0, 7).Format("2006-01-02")
		}
		as.Status = "in_progress"
		as.VisitScheduledDate = in.NextDate
		as.ScheduleConfirmed = in.ScheduleConfirmed
		out.WorkItems = append(out.WorkItems, ASWorkItemDraft{
			WorkKind:          WorkKindConfirm,
			ScheduledDate:     in.NextDate,
			ScheduleConfirmed: in.ScheduleConfirmed,
			ConfirmTarget:     in.ConfirmTarget,
			ConfirmContact:    in.ConfirmContact,
			Notes:             "제조사에스컬레이션(호환)",
		})

	case "":
		as.RevisitReason = ""

	default:
		if code != ResultRevisit {
			as.RevisitReason = ""
		}
	}
	return out, nil
}

// IsOpenIncompleteStatus 담당자 미완료(open)에 포함되는 상태
func IsOpenIncompleteStatus(status string) bool {
	switch status {
	case "received", "assigned", "in_progress", "hold", "transfer":
		return true
	default:
		return false
	}
}

// DefaultTransferFollowupDate 이관 확인예정일 기본값 (오늘+7일)
func DefaultTransferFollowupDate(now time.Time) string {
	return now.AddDate(0, 0, 7).Format("2006-01-02")
}

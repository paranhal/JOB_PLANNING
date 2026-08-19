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

// AS 상태값
const (
	StatusPartialComplete = "partial_complete" // 부분완료: 통계=완료, 운영=진행중
)

// SQL IN 절용 상태 집합
const (
	// SQLStatusStatsCompleted 통계 완료 집계(부분완료 포함)
	SQLStatusStatsCompleted = "('completed','closed','partial_complete')"
	// SQLStatusStatsOpen 통계 미완료(부분완료 제외 — 하위업무는 별도 집계)
	SQLStatusStatsOpen = "('received','assigned','in_progress','hold','transfer')"
	// SQLStatusFullyClosed 완전 종료(읽기전용·재접수 대상)
	SQLStatusFullyClosed = "('completed','closed')"
	// SQLStatusOpenIncomplete 담당자 미완료(운영)
	SQLStatusOpenIncomplete = "('received','assigned','in_progress','hold','transfer','partial_complete')"
	// SQLStatusOpsInProgress 운영상 진행중(방문일정 버킷 등)
	SQLStatusOpsInProgress = "('in_progress','partial_complete')"
)

// 처리결과코드 (조치 화면: 완료 · 추가조치 필요 · 재방문 필요 · 이관 · 대기)
const (
	ResultDone     = "done"
	ResultPartial  = "partial" // 부분완료 — 화면 「추가조치 필요」
	ResultTransfer = "transfer"
	ResultRevisit  = "revisit_needed"
	ResultHold     = "hold" // 화면 「대기」 → 접수 상태 hold. 저장 코드 신설 없음
	// 하위 호환(더 이상 조치 UI에 노출하지 않음)
	ResultTemporary  = "temporary"
	ResultEscalation = "escalation"
)

// ShowsASCauseReport 장애원인·결론(조치 정보)을 여는 결과. 완료·추가조치 필요. §12.10.4 v2.12
func ShowsASCauseReport(resultCode string) bool {
	switch strings.TrimSpace(resultCode) {
	case ResultDone, ResultPartial:
		return true
	default:
		return false
	}
}

// ActionResultOption 조치 화면 결과 선택지 (저장 코드는 유지, 문구만 화면용)
type ActionResultOption struct {
	Value string
	Label string
}

// ActionResultOptions 조치 등록 화면의 결과 다섯 가지.
func ActionResultOptions() []ActionResultOption {
	return []ActionResultOption{
		{ResultDone, "완료"},
		{ResultPartial, "추가조치 필요"},
		{ResultRevisit, "재방문 필요"},
		{ResultTransfer, "이관"},
		{ResultHold, "대기"},
	}
}

// ActionResultLabel 조치 결과 화면 문구. 저장 코드는 바꾸지 않는다.
func ActionResultLabel(code string) string {
	switch strings.TrimSpace(code) {
	case ResultDone:
		return "완료"
	case ResultPartial:
		return "추가조치 필요"
	case ResultRevisit:
		return "재방문 필요"
	case ResultTransfer:
		return "이관"
	case ResultHold:
		return "대기"
	case ResultTemporary:
		return "임시조치"
	case ResultEscalation:
		return "제조사에스컬레이션"
	default:
		return strings.TrimSpace(code)
	}
}

// TransferDetailLabel 이관 후 처리 화면 문구.
func TransferDetailLabel(detail string) string {
	switch strings.TrimSpace(detail) {
	case TransferDetailCompleted:
		return "처리 확인 후 완료"
	case TransferDetailWaiting:
		return "우리 팀 추가 작업"
	default:
		return strings.TrimSpace(detail)
	}
}

// 타사이관 세부
const (
	TransferDetailCompleted = "completed" // 이관 완료 → 접수 완료
	TransferDetailWaiting   = "waiting"   // 조치 결과 대기 → 부분완료 + 확인 하부업무
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
	AssignedTo        string
	AssignedUserID    string
}

func setCompleteDatetimeIfEmpty(as *ASReceipt, now time.Time) {
	if as.CompleteDatetime == nil || as.CompleteDatetime.IsZero() {
		t := now
		as.CompleteDatetime = &t
	}
}

// markPartialComplete 부분완료: 통계용 완료일시 기록 + 운영상 미완료 유지
func markPartialComplete(as *ASReceipt, now time.Time) {
	as.Status = StatusPartialComplete
	setCompleteDatetimeIfEmpty(as, now)
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
		setCompleteDatetimeIfEmpty(as, now)

	case ResultPartial:
		// 추가 업무가 남은 부분완료 — 통계 완료 + 운영 진행중
		as.RevisitReason = strings.TrimSpace(as.RevisitReason)
		if in.NextDate == "" {
			return nil, ErrRevisitDateRequired
		}
		as.VisitScheduledDate = in.NextDate
		as.ScheduleConfirmed = in.ScheduleConfirmed
		markPartialComplete(as, now)
		out.WorkItems = append(out.WorkItems, ASWorkItemDraft{
			WorkKind:          WorkKindRevisit,
			ScheduledDate:     in.NextDate,
			ScheduleConfirmed: in.ScheduleConfirmed,
			Notes:             firstNonEmptyNote(as.RevisitReason, "부분완료·추가 업무"),
		})

	case ResultTransfer:
		switch in.TransferDetail {
		case TransferDetailCompleted:
			as.Status = "completed"
			as.RevisitReason = ""
			setCompleteDatetimeIfEmpty(as, now)
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
			as.RevisitReason = ""
			as.VisitScheduledDate = in.NextDate
			as.ScheduleConfirmed = in.ScheduleConfirmed
			markPartialComplete(as, now)
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
		markPartialComplete(as, now)
		out.WorkItems = append(out.WorkItems, ASWorkItemDraft{
			WorkKind:          WorkKindRevisit,
			ScheduledDate:     in.NextDate,
			ScheduleConfirmed: in.ScheduleConfirmed,
			Notes:             as.RevisitReason,
		})

	case ResultTemporary:
		// 하위 호환: 부분완료 + 방문일
		if in.NextDate == "" {
			return nil, ErrTemporaryDateRequired
		}
		as.RevisitReason = ""
		as.VisitScheduledDate = in.NextDate
		as.ScheduleConfirmed = in.ScheduleConfirmed
		markPartialComplete(as, now)
		out.WorkItems = append(out.WorkItems, ASWorkItemDraft{
			WorkKind:          WorkKindRevisit,
			ScheduledDate:     in.NextDate,
			ScheduleConfirmed: in.ScheduleConfirmed,
			Notes:             "임시조치(호환)",
		})

	case ResultEscalation:
		// 하위 호환: 부분완료 + 확인
		if in.NextDate == "" {
			in.NextDate = now.AddDate(0, 0, 7).Format("2006-01-02")
		}
		as.VisitScheduledDate = in.NextDate
		as.ScheduleConfirmed = in.ScheduleConfirmed
		markPartialComplete(as, now)
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
		if code != ResultRevisit && code != ResultPartial {
			as.RevisitReason = ""
		}
	}
	return out, nil
}

func firstNonEmptyNote(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// IsOpenIncompleteStatus 담당자 미완료(open)에 포함되는 상태
func IsOpenIncompleteStatus(status string) bool {
	switch status {
	case "received", "assigned", "in_progress", "hold", "transfer", StatusPartialComplete:
		return true
	default:
		return false
	}
}

// IsStatsCompletedStatus 통계 완료 집계에 포함되는 상태
func IsStatsCompletedStatus(status string) bool {
	switch status {
	case "completed", "closed", StatusPartialComplete:
		return true
	default:
		return false
	}
}

// CanReopenAS 재접수할 수 있는 상태인지 — 이미 끝난 건만 다시 접수한다.
// 부분완료·진행 중인 건은 재접수 대신 조치로 이어간다.
func CanReopenAS(status string) bool {
	return status == "completed" || status == "closed"
}

// CanIssueASReport 조치완료보고서 발급 가능 상태. 완료·종료·부분완료. §12.10.5
func CanIssueASReport(status string) bool {
	return status == "completed" || status == "closed" || status == StatusPartialComplete
}

// NewReopenReceipt 완료된 접수를 바탕으로 같은 증상의 새 접수를 만든다.
// 접수번호·상태는 저장 단계에서 새로 매겨지므로 여기서는 내용만 옮긴다.
func NewReopenReceipt(src *ASReceipt, reason string, now time.Time) *ASReceipt {
	reason = strings.TrimSpace(reason)
	return &ASReceipt{
		ReceiptDatetime: now,
		CustomerID:      src.CustomerID,
		AssetID:         src.AssetID,
		ReceiptChannel:  src.ReceiptChannel,
		Requester:       src.Requester,
		RequesterType:   src.RequesterType,
		RequesterName:   src.RequesterName,
		Symptom:         src.Symptom,
		Urgency:         src.Urgency,
		Priority:        src.Priority,
		AssignedTo:      src.AssignedTo,
		AssignedUserID:  src.AssignedUserID,
		IsRecurrence:    true,
		IsReopen:        true,
		ParentASID:      src.ASID,
		ReopenReason:    reason,
	}
}

// DefaultTransferFollowupDate 이관 확인예정일 기본값 (오늘+7일)
func DefaultTransferFollowupDate(now time.Time) string {
	return now.AddDate(0, 0, 7).Format("2006-01-02")
}

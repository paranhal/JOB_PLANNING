package service

import (
	"strings"

	"customer-support/internal/model"
)

const (
	KanbanErrPlace         = "place"
	KanbanErrAssignee      = "assignee"
	KanbanErrCompleteNote  = "complete_note"
	KanbanErrUnplanned     = "kanban_unplanned"
	KanbanErrReopen        = "kanban_reopen"
	KanbanErrForbidden     = "kanban_forbidden"
	KanbanErrPartialItems  = "kanban_partial_items"
	KanbanErrPartialEmpty  = "kanban_partial_empty"
	KanbanErrRemainItems   = "kanban_remain"
	KanbanErrActionScreen  = "kanban_action"
	KanbanErrCarryConfirm  = "kanban_carry"
)

type KanbanMoveError struct{ Code string }

func (e *KanbanMoveError) Error() string {
	if e == nil {
		return ""
	}
	return e.Code
}

func errKanban(code string) error {
	return &KanbanMoveError{Code: code}
}

// KanbanOps 구 칸반·신 칸반이 공유하는 실제 저장 동작.
type KanbanOps interface {
	CurrentAssignee(key string) string
	CheckAssignee(org, assignee, current string) error
	AssignDate(org, key, date, assignee string, start bool) error
	SetProgress(key string, start bool) error
	Complete(key, date, note string) error
	Reopen(key, to, date, assignee string) error
}

type KanbanMove struct {
	Key, From, To, Date, Assignee, Note string
	Org                                 string
	ConfirmReopen                       bool
}

// ApplyKanbanMove 일일 업무 등록 칸반 전환 로직(구 화면과 동일).
func ApplyKanbanMove(ops KanbanOps, in KanbanMove) error {
	if ops == nil {
		return errKanban(KanbanErrPlace)
	}
	from := strings.TrimSpace(in.From)
	to := strings.TrimSpace(in.To)
	key := strings.TrimSpace(in.Key)
	if key == "" || to == "" {
		return errKanban(KanbanErrPlace)
	}
	if to == model.WorkBucketUnplanned {
		return errKanban(KanbanErrUnplanned)
	}
	if from == to {
		return nil
	}
	if from == model.WorkBucketCompletedToday && !in.ConfirmReopen {
		return errKanban(KanbanErrReopen)
	}
	assignee := strings.TrimSpace(in.Assignee)
	needAssignee := (to == model.WorkBucketToday || to == model.WorkBucketInProgress) &&
		(from == model.WorkBucketUnplanned)
	cur := ops.CurrentAssignee(key)
	if needAssignee {
		if strings.TrimSpace(cur) == "" && assignee == "" {
			return errKanban(KanbanErrAssignee)
		}
		if assignee == "" {
			assignee = cur
		}
	}
	if strings.TrimSpace(assignee) != "" {
		if err := ops.CheckAssignee(in.Org, assignee, cur); err != nil {
			return err
		}
	}
	switch {
	case to == model.WorkBucketCompletedToday:
		if strings.TrimSpace(in.Note) == "" {
			return errKanban(KanbanErrCompleteNote)
		}
		return ops.Complete(key, in.Date, in.Note)
	case from == model.WorkBucketCompletedToday:
		if err := ops.Reopen(key, to, in.Date, assignee); err != nil {
			return err
		}
		if to == model.WorkBucketToday || to == model.WorkBucketInProgress {
			return applyOpenMove(ops, in.Org, key, model.WorkBucketToday, to, in.Date, assignee)
		}
		return nil
	default:
		return applyOpenMove(ops, in.Org, key, from, to, in.Date, assignee)
	}
}

func applyOpenMove(ops KanbanOps, org, key, from, to, date, assignee string) error {
	switch {
	case from == model.WorkBucketUnplanned && to == model.WorkBucketToday:
		return ops.AssignDate(org, key, date, assignee, false)
	case from == model.WorkBucketUnplanned && to == model.WorkBucketInProgress:
		return ops.AssignDate(org, key, date, assignee, true)
	case from == model.WorkBucketDelayed && to == model.WorkBucketToday:
		return ops.AssignDate(org, key, date, assignee, false)
	case from == model.WorkBucketToday && to == model.WorkBucketInProgress:
		return ops.SetProgress(key, true)
	case from == model.WorkBucketDelayed && to == model.WorkBucketInProgress:
		return ops.SetProgress(key, true)
	case from == model.WorkBucketInProgress && to == model.WorkBucketToday:
		return ops.SetProgress(key, false)
	default:
		if to == model.WorkBucketToday {
			return ops.AssignDate(org, key, date, assignee, false)
		}
		if to == model.WorkBucketInProgress {
			if err := ops.AssignDate(org, key, date, assignee, false); err != nil {
				return err
			}
			return ops.SetProgress(key, true)
		}
		return errKanban(KanbanErrPlace)
	}
}

// BoardKanbanCol 신화면 칸반 열.
const (
	BoardKanbanDelay    = "delay"
	BoardKanbanOff      = "off"
	BoardKanbanPlan     = "today_plan"
	BoardKanbanProgress = "today_progress"
	BoardKanbanPartial  = "today_partial"
	BoardKanbanDone     = "today_done"
)

func BoardKanbanToBucket(col string) string {
	switch strings.TrimSpace(col) {
	case BoardKanbanDelay:
		return model.WorkBucketDelayed
	case BoardKanbanOff, BoardKanbanProgress:
		return model.WorkBucketInProgress
	case BoardKanbanPlan:
		return model.WorkBucketToday
	case BoardKanbanDone:
		return model.WorkBucketCompletedToday
	case BoardKanbanPartial:
		return model.StatusPartialComplete
	default:
		return strings.TrimSpace(col)
	}
}

func BoardKanbanForbidden(from, to string) bool {
	from, to = strings.TrimSpace(from), strings.TrimSpace(to)
	today := from == BoardKanbanPlan || from == BoardKanbanProgress || from == BoardKanbanPartial || from == BoardKanbanDone
	side := to == BoardKanbanDelay || to == BoardKanbanOff
	return today && side
}

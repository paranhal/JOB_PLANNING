package approval

import "strings"

// Access 열람·처리 범위. PermView / PermAct 비트.
func AccessFor(actor Actor, req Request, steps []Step, delegateOfHead string) int {
	if strings.TrimSpace(actor.UserID) == "" {
		return 0
	}
	view, act := 0, 0
	if actor.UserID == req.DrafterID {
		view = PermView
		if req.Status == StatusInProgress {
			act = PermAct // 회수
		}
	}
	for _, s := range steps {
		if s.StepType == TypeCC && s.ApproverID == actor.UserID {
			view = PermView
		}
		if s.StepType == TypeApprove && s.ApproverID == actor.UserID {
			view = PermView
			if s.Status == StepPending {
				act = PermAct
			}
		}
	}
	if delegateOfHead != "" && actor.UserID == delegateOfHead {
		view = PermView
		for _, s := range steps {
			if s.Status == StepPending && s.ApproverID != actor.UserID && s.StepType == TypeApprove && actor.UserID != req.DrafterID {
				act = PermAct
			}
		}
	}
	if isAdminRole(actor.Role) && actor.OrgID == req.OrgID {
		view = PermView
	}
	if actor.ReadOnly || actor.ViewAs {
		act = 0
	}
	return view | act
}

func (e *Engine) IsLocked(docType, docID string) bool {
	var st string
	err := e.DB.QueryRow(`SELECT status FROM approval_requests
		WHERE doc_type=? AND doc_id=? ORDER BY revision DESC LIMIT 1`, docType, docID).Scan(&st)
	if err != nil {
		return false
	}
	return st == StatusInProgress || st == StatusApproved
}

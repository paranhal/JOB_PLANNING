package approval

import (
	"database/sql"
	"strings"
	"time"
)

const (
	StatusDraft      = "draft"
	StatusInProgress = "in_progress"
	StatusApproved   = "approved"
	StatusRejected   = "rejected"
	StatusWithdrawn  = "withdrawn"

	StepWaiting  = "waiting"
	StepPending  = "pending"
	StepApproved = "approved"
	StepRejected = "rejected"
	StepSkipped  = "skipped"

	TypeApprove = "approve"
	TypeAgree   = "agree"
	TypeCC      = "cc"

	DocLeave = "leave"
	DocQuote = "quote"
	DocTest  = "test_doc"

	ModeDirect         = "direct"
	ModeSelf           = "self"
	ModeDelegate       = "delegate"
	ModeAdminEmergency = "admin_emergency"

	SrcHead  = "head"
	SrcAdmin = "admin"
	SrcLeave = "leave"

	EvSubmit          = "submit"
	EvApprove         = "approve"
	EvSelfApprove     = "self_approve"
	EvDelegateApprove = "delegate_approve"
	EvAdminEmergency  = "admin_emergency_approve"
	EvReject          = "reject"
	EvWithdraw        = "withdraw"
	EvResubmit        = "resubmit"

	PermView = 1
	PermAct  = 2
)

type Actor struct {
	UserID   string
	Username string
	Name     string
	OrgID    string
	Role     string
	ReadOnly bool
	ViewAs   bool
}

func (a Actor) CanProcess() bool {
	return !a.ReadOnly && !a.ViewAs && strings.TrimSpace(a.UserID) != ""
}

type Policy struct {
	OrgID                  string
	DelegByHead            bool
	DelegByAdmin           bool
	DelegWithLeave         bool
	AdminEmergencyApprove  bool
	SelfApproveLeave       bool
	SelfApproveQuote       bool
	NotifyAdminSelfApprove bool
	UpdatedBy              string
}

func DefaultPolicy(orgID string) Policy {
	return Policy{
		OrgID: orgID, DelegByHead: true, DelegByAdmin: true,
		SelfApproveLeave: true, SelfApproveQuote: true,
	}
}

func (p Policy) HeadSelfApprove(docType string) bool {
	switch strings.TrimSpace(docType) {
	case DocLeave:
		return p.SelfApproveLeave
	case DocQuote:
		return p.SelfApproveQuote
	default:
		return p.SelfApproveLeave || p.SelfApproveQuote
	}
}

type Request struct {
	RequestID    string
	OrgID        string
	DocType      string
	DocID        string
	Revision     int
	DrafterID    string
	Title        string
	Status       string
	SnapshotJSON string
	SubmittedAt  string
	CompletedAt  string
}

type Step struct {
	StepID     string
	RequestID  string
	StepNo     int
	StepType   string
	ApproverID string
	ActedBy    string
	ActMode    string
	Status     string
	Comment    string
	ActedAt    string
}

type Delegation struct {
	DelegationID string
	OrgID        string
	FromUserID   string
	ToUserID     string
	StartDate    string
	EndDate      string
	Source       string
	LeaveReqID   string
	CreatedBy    string
	RevokedAt    string
}

type Engine struct {
	DB    *sql.DB
	Now   func() time.Time
	Hooks Hooks
}

func (e *Engine) now() time.Time {
	if e != nil && e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

func (e *Engine) today() string {
	return e.now().Format("2006-01-02")
}

func ClockDash(n int) string {
	if n <= 0 {
		return "—"
	}
	return ""
}

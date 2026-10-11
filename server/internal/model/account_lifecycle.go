package model

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

const (
	AccountReqPending   = "pending"
	AccountReqApproved  = "approved"
	AccountReqRejected  = "rejected"
	AccountReqCancelled = "cancelled"

	ReserveCurrent    = "current"
	ReserveRenamedFrom = "renamed_from"
	ReserveDeleted    = "deleted"

	LifeCreated      = "created"
	LifeDeactivated  = "deactivated"
	LifeReactivated  = "reactivated"
	LifeDeleted      = "deleted"
	LifeRestored     = "restored"
	LifeRenamed      = "renamed"
	LifeLeaveExpire  = "leave_expire_todo"

	UsernameReservedMsg = "이미 사용했거나 삭제된 아이디입니다"
)

// NormalizeUsername 앞뒤·내부 공백 제거, NFC, 소문자.
func NormalizeUsername(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = norm.NFC.String(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		b.WriteRune(r)
	}
	return strings.ToLower(b.String())
}

func RequestRoleOK(role string) bool {
	r := NormalizeRole(role)
	return r == RoleSales || r == RoleTech || r == RoleSupport
}

type AccountRequest struct {
	RequestID     string
	OrgID         string
	RequestedBy   string
	FullName      string
	Username      string
	Role          string
	Mobile        string
	Reason        string
	Status        string
	RestoreUserID string
	DecidedBy     string
	DecidedAt     string
	DecisionNote  string
	CreatedUserID string
	CreatedAt     string
	OrgName       string
	RequesterName string
}

type UsernameReservation struct {
	UsernameNorm string
	UserID       string
	Reason       string
	CreatedAt    string
}

type UserLifecycleEvent struct {
	EventID   string
	UserID    string
	Kind      string
	ActorID   string
	Note      string
	Detail    string
	CreatedAt string
}

func (u *User) IsDeleted() bool {
	return u != nil && strings.TrimSpace(u.DeletedAt) != ""
}

func (u *User) DisplayName() string {
	if u == nil {
		return ""
	}
	if u.IsDeleted() {
		return strings.TrimSpace(u.FullName) + " (삭제됨)"
	}
	return u.FullName
}

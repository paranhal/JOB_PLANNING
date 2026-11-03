package model

import (
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

const (
	AccountReqPending   = "pending"
	AccountReqApproved  = "approved"
	AccountReqRejected  = "rejected"
	AccountReqCancelled = "cancelled"

	ReserveCurrent     = "current"
	ReserveRenamedFrom = "renamed_from"
	ReserveDeleted     = "deleted"

	LifeCreated             = "created"
	LifeDeactivated         = "deactivated"
	LifeReactivated         = "reactivated"
	LifeDeleted             = "deleted"
	LifeRestored            = "restored"
	LifeRenamed             = "renamed"
	LifeLeaveExpire         = "leave_expire_todo"
	LifeReservationReleased = "reservation_released"

	UsernameReservedMsg = "이미 사용했거나 삭제된 아이디입니다"
	UsernameInquiryMsg  = "이미 사용했던 아이디입니다. 관리자에게 문의하세요"

	UsernameConflictLive           = "live"
	UsernameConflictDeleted        = "deleted"
	UsernameConflictRenamed        = "renamed"
	UsernameConflictReserved       = "reserved"
	UsernameConflictReservedOrphan = "reserved_orphan"
)

// UsernameConflict 아이디 중복 판정. 95-72·95-73.
type UsernameConflict struct {
	Kind          string
	UserID        string
	User          *User
	Username      string
	ReservedAt    string
	Reason        string
	OwnerName     string
	OwnerUsername string
	OwnerDeleted  bool
}

func (c UsernameConflict) Empty() bool {
	return strings.TrimSpace(c.Kind) == ""
}

func (c UsernameConflict) PublicMessage() string {
	switch c.Kind {
	case UsernameConflictLive:
		return "이미 사용 중인 아이디입니다"
	case UsernameConflictDeleted:
		return UsernameReservedMsg
	case UsernameConflictRenamed, UsernameConflictReserved, UsernameConflictReservedOrphan:
		return UsernameInquiryMsg
	default:
		if c.Empty() {
			return ""
		}
		return UsernameReservedMsg
	}
}

func (c UsernameConflict) Message() string {
	return c.PublicMessage()
}

func reserveDay(at string) string {
	at = strings.TrimSpace(at)
	if len(at) >= 10 {
		return at[:10]
	}
	return at
}

func (c UsernameConflict) VisionMessage() string {
	name := strings.TrimSpace(c.Username)
	if name == "" {
		name = "이 아이디"
	}
	day := reserveDay(c.ReservedAt)
	owner := strings.TrimSpace(c.OwnerName)
	cur := strings.TrimSpace(c.OwnerUsername)
	who := owner
	if owner == "" {
		who = "알 수 없는 사용자"
	}
	if cur != "" {
		who = fmt.Sprintf("%s(현재 아이디 %s)", who, cur)
	}
	switch c.Kind {
	case UsernameConflictRenamed:
		if day != "" {
			return fmt.Sprintf("%s 는 %s 에 %s 계정이 아이디를 변경하면서 예약된 아이디입니다.", name, day, who)
		}
		return fmt.Sprintf("%s 는 %s 계정이 아이디를 변경하면서 예약된 아이디입니다.", name, who)
	case UsernameConflictReservedOrphan:
		if day != "" {
			return fmt.Sprintf("%s 는 %s 에 소유자를 알 수 없는 예약 아이디입니다.", name, day)
		}
		return fmt.Sprintf("%s 는 소유자를 알 수 없는 예약 아이디입니다.", name)
	case UsernameConflictReserved:
		if day != "" {
			return fmt.Sprintf("%s 는 %s 에 예약된 아이디입니다.", name, day)
		}
		return fmt.Sprintf("%s 는 예약된 아이디입니다.", name)
	case UsernameConflictDeleted:
		return UsernameReservedMsg
	case UsernameConflictLive:
		return "이미 사용 중인 아이디입니다"
	default:
		return c.PublicMessage()
	}
}

func (c UsernameConflict) DialogDup() string {
	switch c.Kind {
	case UsernameConflictDeleted, UsernameConflictRenamed, UsernameConflictReserved, UsernameConflictReservedOrphan:
		return c.Kind
	default:
		return ""
	}
}

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
	UsernameNorm  string
	UserID        string
	Reason        string
	CreatedAt     string
	OwnerName     string
	OwnerUsername string
	OwnerDeleted  bool
}

func (r UsernameReservation) ReasonLabel() string {
	switch r.Reason {
	case ReserveRenamedFrom:
		return "아이디 변경"
	case ReserveDeleted:
		return "삭제"
	case ReserveCurrent:
		return "사용 중"
	default:
		if strings.TrimSpace(r.Reason) == "" {
			return "예약"
		}
		return r.Reason
	}
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

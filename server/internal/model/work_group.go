package model

import "strings"

const (
	WorkGroupActive = "active"
	WorkGroupClosed = "closed"

	WorkGroupLeader = "leader"
	WorkGroupMember = "member"

	WorkLogScopeGroup       = "group"
	WorkLogScopeIndividual  = "individual"
	WorkEffortSelf          = "self"
	WorkEffortByLeader      = "by_leader"
)

type WorkGroup struct {
	GroupID   string
	OrgID     string
	Name      string
	Purpose   string
	ProjectID string
	Status    string
	StartDate string
	EndDate   string
	CreatedBy string
	CreatedAt string
	ClosedBy  string
	ClosedAt  string
	LeaderID  string
	LeaderName string
	MemberN   int
}

func (g WorkGroup) ExpiredOn(day string) bool {
	if g.Status == WorkGroupClosed {
		return true
	}
	day = strings.TrimSpace(day)
	end := strings.TrimSpace(g.EndDate)
	return end != "" && day != "" && end < day
}

type WorkGroupPerson struct {
	ID        int64
	GroupID   string
	UserID    string
	FullName  string
	Role      string
	JoinedOn  string
	LeftOn    string
	AddedBy   string
	RemovedBy string
}

type WorkLogNote struct {
	LogID     string
	OrgID     string
	WorkDate  string
	ProjectID string
	GroupID   string
	Scope     string
	AuthorID  string
	Content   string
	NextPlan  string
	Status    string
}

type WorkEffortEntry struct {
	EntryID    string
	OrgID      string
	WorkDate   string
	UserID     string
	ProjectID  string
	SourceType string
	SourceID   string
	LogID      string
	GroupID    string
	GroupRole  string
	RoleLabel  string
	Minutes    int
	EntryMode  string
	EnteredBy  string
	Note       string
	Revision   int
	VoidedAt   string
}

type WorkEffortSum struct {
	Key     string
	Minutes int
}

type WorkGroupActor struct {
	UserID   string
	OrgID    string
	Role     string
	ReadOnly bool
	ViewAs   bool
}

func (a WorkGroupActor) HomeOrg() string {
	id := strings.TrimSpace(a.OrgID)
	if id == "" {
		return OrgIDLibrary
	}
	return id
}

func (a WorkGroupActor) CanWrite() bool {
	if a.ReadOnly || a.ViewAs {
		return false
	}
	r := NormalizeRole(a.Role)
	return r != RoleCLevel && r != RoleTester && r != RoleObserver
}

func (a WorkGroupActor) CanManageOrg(orgID string) bool {
	if !a.CanWrite() {
		return false
	}
	r := NormalizeRole(a.Role)
	if r == RoleVisionAdmin {
		return strings.TrimSpace(orgID) != "" && orgID != "*"
	}
	if r == RoleOrgAdmin || r == RoleAdmin {
		return a.HomeOrg() == strings.TrimSpace(orgID)
	}
	return false
}

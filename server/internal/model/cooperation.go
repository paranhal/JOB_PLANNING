package model

import "strings"

const (
	CoopAS         = "as"
	CoopInspection = "inspection"
	CoopAdmin      = "admin"
	CoopProject    = "project"

	CoopRequested = "requested"
	CoopAccepted  = "accepted"
	CoopAssigned  = "assigned"
	CoopCompleted = "completed"
	CoopRejected  = "rejected"
	CoopCancelled = "cancelled"

	CoopAssignLead   = "lead"
	CoopAssignMember = "member"
)

func CoopKindLabel(k string) string {
	switch k {
	case CoopAS:
		return "AS"
	case CoopInspection:
		return "정기점검"
	case CoopAdmin:
		return "행정·지원"
	case CoopProject:
		return "사업 수행"
	default:
		return k
	}
}

func CoopStatusLabel(s string) string {
	switch s {
	case CoopRequested:
		return "요청됨"
	case CoopAccepted:
		return "수락"
	case CoopAssigned:
		return "배정됨"
	case CoopCompleted:
		return "완료"
	case CoopRejected:
		return "거부"
	case CoopCancelled:
		return "취소"
	default:
		return s
	}
}

type Cooperation struct {
	CoopID          string
	ReqOrgID        string
	ReqUserID       string
	ReqUserName     string
	ReqOrgName      string
	TargetOrgID     string
	TargetOrgName   string
	Kind            string
	Title           string
	Body            string
	ProjectID       string
	ProjectName     string
	WantStart       string
	WantEnd         string
	Status          string
	RejectReason    string
	PlanStart       string
	PlanEnd         string
	Instruction     string
	WorkGroupID     string
	WorkGroupName   string
	DecidedBy       string
	DecidedAt       string
	CompletedReason string
	CreatedAt       string
	Overdue         bool
	IncompleteAS    bool
	Assignees       []CoopAssignee
}

type CoopAssignee struct {
	UserID   string
	FullName string
	Role     string
}

type CoopEvent struct {
	At       string
	ActorID  string
	Actor    string
	Action   string
	Detail   string
	OnBehalf string
}

type CoopFile struct {
	ID       int64
	FileName string
	Path     string
	MIME     string
	Size     int64
}

type CoopActor struct {
	UserID   string
	OrgID    string
	Role     string
	ReadOnly bool
	ViewAs   bool
}

func (a CoopActor) HomeOrg() string {
	if strings.TrimSpace(a.OrgID) == "" {
		return OrgIDLibrary
	}
	return strings.TrimSpace(a.OrgID)
}

func (a CoopActor) CanWrite() bool {
	if a.ReadOnly || a.ViewAs {
		return false
	}
	r := NormalizeRole(a.Role)
	return r != RoleCLevel && r != RoleTester && r != RoleObserver && r != "partner" && strings.TrimSpace(a.Role) != "partner"
}

func (a CoopActor) CanRequest() bool {
	if !a.CanWrite() {
		return false
	}
	r := NormalizeRole(a.Role)
	switch r {
	case RoleVisionAdmin, RoleOrgAdmin, RoleSupport, RoleTech, RoleSales, RoleAdmin, RoleOffice:
		return true
	default:
		return false
	}
}

type CoopProjectView struct {
	ProjectID    string `json:"project_id"`
	Name         string `json:"name"`
	ShortName    string `json:"short_name"`
	PlanYear     int    `json:"plan_year"`
	Status       string `json:"status"`
	StartDate    string `json:"start_date"`
	EndDate      string `json:"end_date"`
	CustomerName string `json:"customer_name,omitempty"`
	Notes        string `json:"notes,omitempty"`
	Kind         string `json:"project_kind,omitempty"`
}

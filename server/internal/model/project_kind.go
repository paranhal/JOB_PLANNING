package model

import "strings"

const (
	ProjectKindConstruction = "construction"
	ProjectKindBuild        = "build"
	ProjectKindDev          = "dev"
	ProjectKindMaintenance  = "maintenance"
)

const (
	ProjectDutySales      = "sales"
	ProjectDutyExecution  = "execution"
	ProjectDutyManagement = "management"
	ProjectDutyAccounting = "accounting"
)

func ValidProjectKind(v string) bool {
	switch strings.TrimSpace(v) {
	case ProjectKindConstruction, ProjectKindBuild, ProjectKindDev, ProjectKindMaintenance:
		return true
	default:
		return false
	}
}

func ProjectKindLabel(v string) string {
	switch strings.TrimSpace(v) {
	case ProjectKindConstruction:
		return "공사"
	case ProjectKindBuild:
		return "구축"
	case ProjectKindDev:
		return "개발"
	case ProjectKindMaintenance:
		return "유지보수"
	default:
		return "—"
	}
}

func ProjectKindOptions() []struct{ Value, Label string } {
	return []struct{ Value, Label string }{
		{ProjectKindConstruction, "공사"},
		{ProjectKindBuild, "구축"},
		{ProjectKindDev, "개발"},
		{ProjectKindMaintenance, "유지보수"},
	}
}

func ValidProjectDuty(v string) bool {
	switch strings.TrimSpace(v) {
	case ProjectDutySales, ProjectDutyExecution, ProjectDutyManagement, ProjectDutyAccounting:
		return true
	default:
		return false
	}
}

func ProjectDutyLabel(v string) string {
	switch strings.TrimSpace(v) {
	case ProjectDutySales:
		return "영업 담당"
	case ProjectDutyExecution:
		return "수행 담당"
	case ProjectDutyManagement:
		return "사업관리 담당"
	case ProjectDutyAccounting:
		return "회계 담당"
	default:
		return v
	}
}

func ProjectDutyOptions() []struct{ Value, Label string } {
	return []struct{ Value, Label string }{
		{ProjectDutySales, "영업 담당"},
		{ProjectDutyExecution, "수행 담당"},
		{ProjectDutyManagement, "사업관리 담당"},
		{ProjectDutyAccounting, "회계 담당"},
	}
}

type ProjectAssignment struct {
	ID        int64  `json:"id"`
	ProjectID string `json:"project_id"`
	UserID    string `json:"user_id"`
	Duty      string `json:"duty"`
	UserName  string `json:"user_name,omitempty"`
	Deleted   bool   `json:"deleted,omitempty"`
}

func AssignmentSelected(list []ProjectAssignment, duty, userID string) bool {
	for _, a := range list {
		if a.Duty == duty && a.UserID == userID {
			return true
		}
	}
	return false
}

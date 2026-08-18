package model

import "strings"

const (
	LeaveKindAnnual = "annual"
	LeaveKindHalfAM = "half_am"
	LeaveKindHalfPM = "half_pm"
)

// StaffLeave 직원 연차 1건. staff_leaves (§23.13.3).
type StaffLeave struct {
	LeaveID   string
	UserID    string
	UserName  string // JOIN users.full_name (표시용)
	Date      string // YYYY-MM-DD
	Kind      string // annual | half_am | half_pm
	Note      string
	CreatedBy string
	CreatedAt string
}

func ValidLeaveKind(kind string) bool {
	switch strings.TrimSpace(kind) {
	case LeaveKindAnnual, LeaveKindHalfAM, LeaveKindHalfPM:
		return true
	default:
		return false
	}
}

func LeaveKindLabel(kind string) string {
	switch strings.TrimSpace(kind) {
	case LeaveKindAnnual:
		return "연차"
	case LeaveKindHalfAM:
		return "오전반차"
	case LeaveKindHalfPM:
		return "오후반차"
	default:
		return kind
	}
}

// LeaveBadgeText "{이름} 연차" 형태. §23.13.5
func LeaveBadgeText(name, kind string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "직원"
	}
	return name + " " + LeaveKindLabel(kind)
}

// LeaveCellMark §16.4 담당자별 일일업무 칸 표기.
func LeaveCellMark(kind string) string {
	if strings.TrimSpace(kind) == "" {
		return ""
	}
	return "연차"
}

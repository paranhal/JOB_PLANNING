package model

import "strings"

// DeriveASWorkflowStatus 접수·배정·일정확정 기준 워크플로 상태
// 일정 확정 → in_progress(진행중)
// 배정담당자 있음 → assigned(담당자 배정)
// 그 외 → received(접수)
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

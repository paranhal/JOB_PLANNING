package model

import "strings"

const (
	ActFilterAll  = ""
	ActFilterAS   = "as"
	ActFilterMnt  = "mnt"
	ActFilterTask = "task"
)

// WorkTaskActClass work_tasks.work_type 분류. 83-D. 매핑 안 되면 기타 업무.
func WorkTaskActClass(workType string) string {
	switch strings.TrimSpace(workType) {
	case WBWorkAS, "replace":
		return ActFilterAS
	case WBWorkMaintenance, "visit":
		return ActFilterMnt
	default:
		return ActFilterTask
	}
}

func WorkTaskActLabel(workType string) string {
	switch WorkTaskActClass(workType) {
	case ActFilterAS:
		return "AS"
	case ActFilterMnt:
		return "정기점검"
	default:
		return "기타 업무"
	}
}

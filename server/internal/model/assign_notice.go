package model

import "strings"

// AssignNotice 배정 알림 한 줄. 메일·푸시 없이 화면에서만 쓴다. §42.5
type AssignNotice struct {
	NoticeID   string
	UserID     string
	SourceType string // as | maintenance | task
	SourceID   string
	AssignedBy string
	AssignedAt string
	SeenAt     string
	ActedAt    string

	Number  string // AS 번호 · 방문번호 · 업무번호
	OrgName string
	Title   string
}

const (
	AssignNoticeSourceAS          = "as"
	AssignNoticeSourceMaintenance = "maintenance"
	AssignNoticeSourceTask        = "task"
)

func (n AssignNotice) Unseen() bool { return strings.TrimSpace(n.SeenAt) == "" }

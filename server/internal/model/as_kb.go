package model

import (
	"strconv"
	"strings"
	"time"
)

const (
	KBOriginFromAction = "from_action"
	KBOriginAdded      = "added"
	KBOriginRevised    = "revised"

	KBStatusDraft     = "draft"
	KBStatusPublished = "published"
	KBStatusArchived  = "archived"

	KBAuthorUnknown = "작성자 미상"
	KBDash          = "—"
)

// ASKBEntry 지식 한 행. 고치면 덮어쓰지 않고 새 행. §41.8 · §41.10
type ASKBEntry struct {
	KBID          string
	ASID          string
	SymptomText   string
	ActionText    string
	Origin        string
	Rev           int
	IsCurrent     bool
	PrevKBID      string
	AuthorID      string
	AuthorName    string
	CreatedAt     time.Time
	ChangeNote    string
	Status        string
	HelpfulCount  int
	Past          []ASKBEntry
}

func DisplayPerson(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return KBAuthorUnknown
	}
	return name
}

func KnowledgeWhen(t time.Time) (date, full string) {
	if t.IsZero() {
		return KBDash, ""
	}
	return t.Format("2006-01-02"), t.Format("2006-01-02 15:04")
}

func KnowledgeWhenString(s string) (date, full string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return KBDash, ""
	}
	for _, f := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02 15:04", "2006-01-02"} {
		if t, err := time.ParseInLocation(f, s, time.Local); err == nil {
			return KnowledgeWhen(t)
		}
	}
	if len(s) >= 10 {
		return s[:10], s
	}
	return s, s
}

func InferKBOrigin(originalAction, actionText string) string {
	orig := strings.TrimSpace(originalAction)
	text := strings.TrimSpace(actionText)
	if orig == "" {
		return KBOriginAdded
	}
	if orig == text {
		return KBOriginFromAction
	}
	return KBOriginRevised
}

func KBOriginLabel(origin string) string {
	switch origin {
	case KBOriginAdded:
		return "지식 추가"
	case KBOriginRevised:
		return "정정"
	default:
		return "조치 기록"
	}
}

func KBRevLabel(rev int) string {
	if rev >= 2 {
		return strconv.Itoa(rev) + "차 수정"
	}
	return ""
}

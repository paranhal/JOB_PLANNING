package model

import (
	"strings"
	"unicode/utf8"
)

const (
	ActionNAPrefix     = "해당 없음:"
	ActionContentMin   = 10
	ActionErrRequired  = "action_required"
	ActionErrNAReason  = "action_na_reason"
	ActionErrShort     = "action_short"
)

// ResolveActionContent 조치 본문. 해당 없음+사유 회피 경로. §5 · §41.4
func ResolveActionContent(taken string, na bool, reason string) (string, string) {
	taken = strings.TrimSpace(taken)
	reason = strings.TrimSpace(reason)
	if na {
		if reason == "" {
			return "", ActionErrNAReason
		}
		return ActionNAPrefix + " " + reason, ""
	}
	if taken == "" {
		return "", ActionErrRequired
	}
	return taken, ""
}

func ActionContentTooShort(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasPrefix(s, ActionNAPrefix) {
		return false
	}
	return utf8.RuneCountInString(s) < ActionContentMin
}

func IsActionNA(s string) bool {
	return strings.HasPrefix(strings.TrimSpace(s), ActionNAPrefix)
}

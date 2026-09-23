package model

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var laborLeadNum = regexp.MustCompile(`^[0-9]+[.)),\s]*`)

// CleanJobName 맨 앞의 번호·원문자를 떼고 앞뒤 공백을 지운다. 가운데 공백은 남긴다("IT PM"). §47.20.2
func CleanJobName(s string) string {
	s = strings.TrimSpace(s)
	for {
		if s == "" {
			return ""
		}
		r, size := utf8.DecodeRuneInString(s)
		if isJobPrefixMark(r) {
			s = strings.TrimSpace(s[size:])
			continue
		}
		break
	}
	s = laborLeadNum.ReplaceAllString(s, "")
	s = strings.TrimLeftFunc(s, func(r rune) bool {
		return r == '·' || r == '-' || r == '.' || unicode.IsSpace(r)
	})
	return strings.TrimSpace(s)
}

func isJobPrefixMark(r rune) bool {
	switch {
	case r >= '①' && r <= '⑳':
		return true
	case r >= '❶' && r <= '❿':
		return true
	case r >= '\u3200' && r <= '\u321E': // ㈀–㈞ (㈎류)
		return true
	default:
		return false
	}
}

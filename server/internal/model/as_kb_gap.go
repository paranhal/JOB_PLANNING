package model

import (
	"strings"
	"unicode"
)

// ASKBGap 답을 못 찾은 검색. hit_count=0 이 아직 비어 있는 것. §41.13
type ASKBGap struct {
	GapID        string
	Query        string
	QueryNorm    string
	SearchedBy   string
	SearchedAt   string
	HitCount     int
	ASID         string
	SearchCount  int
	ResolvedKBID string
	ResolvedAt   string
	RecentLabel  string
}

// ASMissingAction 조치 본문이 없는 접수. 화면에는 묶음만 낸다.
type ASMissingAction struct {
	ASID    string
	Symptom string
}

// ASGapSymptomGroup 조치 없는 AS 를 사전 낱말로 묶은 줄. §41.13.2
type ASGapSymptomGroup struct {
	Keyword       string
	KeywordID     string
	Count         int
	SampleASID    string
	SampleSymptom string
	Other         bool
}

func NormalizeKBQuery(q string) string {
	q = strings.TrimSpace(q)
	if q == "" {
		return ""
	}
	var b strings.Builder
	prevSpace := false
	for _, r := range q {
		if unicode.IsSpace(r) {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		prevSpace = false
		b.WriteRune(unicode.ToLower(r))
	}
	return strings.TrimSpace(b.String())
}

func GapRecentLabel(at string) string {
	d, _ := KnowledgeWhenString(at)
	if d == KBDash || len(d) < 10 {
		return d
	}
	return d[5:7] + "-" + d[8:10]
}

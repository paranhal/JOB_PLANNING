package model

import "strings"

const (
	KWGroupTarget  = "대상"
	KWGroupSymptom = "증상"
	KWGroupCause   = "원인"
	KWGroupAction  = "조치"

	KWSourceAuto   = "auto"
	KWSourceManual = "manual"
	KWSourceDict   = "dict"
	KWFieldSymptom = "symptom"
	KWFieldAction  = "action"
)

// ASKeyword 큐레이션한 표준 키워드. §12.11.6 — 자동 확정하지 않는다.
type ASKeyword struct {
	KeywordID string `json:"keyword_id"`
	Keyword   string `json:"keyword"`
	Group     string `json:"kw_group"`
	Synonyms  string `json:"synonyms"`
	IsActive  bool   `json:"is_active"`
	SortOrder int    `json:"sort_order"`
	Note      string `json:"note"`
	LinkCount int    `json:"link_count,omitempty"`
	Suggested bool   `json:"suggested,omitempty"`
	Linked    bool   `json:"linked,omitempty"`
	Source    string `json:"source,omitempty"`
}

func (k ASKeyword) SynonymList() []string {
	return SplitCommaList(k.Synonyms)
}

func (k ASKeyword) MatchTerms() []string {
	terms := []string{k.Keyword}
	return append(terms, k.SynonymList()...)
}

// ASKeywordLink 건별 연결. source=auto 는 제안된 것을 담당자가 체크한 것.
type ASKeywordLink struct {
	ASID      string
	KeywordID string
	Source    string
	Field     string
	Keyword   string
	Group     string
}

// ASKeywordCandidate 본문 빈도 후보. 사전에 넣기 전 단계. 확정 아님.
type ASKeywordCandidate struct {
	Word  string
	Count int
	Field string
}

func KWGroups() []string {
	return []string{KWGroupTarget, KWGroupSymptom, KWGroupCause, KWGroupAction}
}

func SplitCommaList(s string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '，' }) {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

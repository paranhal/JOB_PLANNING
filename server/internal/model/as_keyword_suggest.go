package model

import (
	"regexp"
	"strings"
	"unicode"
)

// KeywordSuggestStopwords 새 키워드 후보에서 뺄 말. 한 곳에서 관리. 95-85 §3.
var KeywordSuggestStopwords = []string{
	"했음", "완료", "함", "등", "및", "후", "수행", "진행", "확인", "작업", "이후",
	"해당", "때문", "있음", "없음", "정상", "그리고", "또는", "접수", "조치", "현장", "방문",
}

var keywordJosaSuffixes = []string{"은", "는", "이", "가", "을", "를", "에", "의", "로", "도", "과", "와"}

var keywordTokenSplit = regexp.MustCompile(`[\s,.·。、/\\|()\[\]{}<>'"~!@#$%^&*_+=?;:]+`)

type KeywordCand struct {
	Keyword string `json:"keyword"`
	Group   string `json:"kw_group"`
}

type ParseKeywordResult struct {
	OK  []KeywordCand
	Err []string
}

const maxKeywordTokens = 20

// SuggestNewKeywords 규칙 기반 신규 후보. 외부 형태소·AI 없음. 최대 6개. 95-85 §3.
func SuggestNewKeywords(text string, dictTerms []string) []KeywordCand {
	stop := map[string]bool{}
	for _, w := range KeywordSuggestStopwords {
		stop[strings.ToLower(w)] = true
	}
	dict := map[string]bool{}
	for _, t := range dictTerms {
		t = strings.ToLower(strings.TrimSpace(t))
		if t != "" {
			dict[t] = true
		}
	}
	raw := keywordTokenSplit.Split(text, -1)
	type hit struct {
		word string
		n    int
		ord  int
	}
	seen := map[string]*hit{}
	order := 0
	for _, w := range raw {
		w = strings.TrimSpace(w)
		w = stripKeywordJosa(w)
		if !keywordCandOK(w, stop, dict) {
			continue
		}
		key := strings.ToLower(w)
		if h, ok := seen[key]; ok {
			h.n++
			continue
		}
		order++
		seen[key] = &hit{word: w, n: 1, ord: order}
	}
	list := make([]*hit, 0, len(seen))
	for _, h := range seen {
		list = append(list, h)
	}
	for i := 0; i < len(list); i++ {
		for j := i + 1; j < len(list); j++ {
			if list[j].n > list[i].n || (list[j].n == list[i].n && list[j].ord < list[i].ord) {
				list[i], list[j] = list[j], list[i]
			}
		}
	}
	if len(list) > 6 {
		list = list[:6]
	}
	out := make([]KeywordCand, 0, len(list))
	for _, h := range list {
		out = append(out, KeywordCand{Keyword: h.word, Group: GuessKeywordGroup(h.word)})
	}
	return out
}

func keywordCandOK(w string, stop, dict map[string]bool) bool {
	r := []rune(w)
	n := len(r)
	if n < 2 || n > 15 {
		return false
	}
	if isKeywordSerial(w) {
		return false
	}
	low := strings.ToLower(w)
	if stop[low] {
		return false
	}
	if dict[low] {
		return false
	}
	for term := range dict {
		if term != "" && strings.Contains(low, term) {
			return false
		}
	}
	return true
}

func isKeywordSerial(w string) bool {
	digits, letters := 0, 0
	for _, r := range w {
		if unicode.IsDigit(r) {
			digits++
		} else if unicode.IsLetter(r) {
			letters++
		}
	}
	if digits == len([]rune(w)) {
		return true
	}
	if digits >= 4 && letters == 0 {
		return true
	}
	if looksLikeDateToken(w) {
		return true
	}
	return false
}

func looksLikeDateToken(w string) bool {
	if regexp.MustCompile(`^\d{4}[-./]?\d{1,2}([-./]?\d{1,2})?$`).MatchString(w) {
		return true
	}
	return false
}

func stripKeywordJosa(w string) string {
	for _, j := range keywordJosaSuffixes {
		if strings.HasSuffix(w, j) && len([]rune(w)) > len([]rune(j))+1 {
			return strings.TrimSuffix(w, j)
		}
	}
	return w
}

// GuessKeywordGroup 규칙 추정. 사용자는 체크 해제 또는 직접 입력으로 고친다. Q-3.
func GuessKeywordGroup(w string) string {
	switch {
	case containsAny(w, "교체", "설정", "점검", "청소", "복구", "조정") || strings.HasPrefix(w, "재"):
		return KWGroupAction
	case containsAny(w, "불량", "고장", "오류", "단선", "노후"):
		return KWGroupCause
	case containsAny(w, "끊김", "지연", "멈춤", "꺼짐") || strings.Contains(w, "안나") || strings.Contains(w, "안 나"):
		return KWGroupSymptom
	default:
		return KWGroupTarget
	}
}

func containsAny(w string, words ...string) bool {
	for _, x := range words {
		if strings.Contains(w, x) {
			return true
		}
	}
	return false
}

// ParseKeywordTokens 쉼표 구분 · 이름(그룹). 그룹 생략 시 기본=조치. 95-85 §3.4.
func ParseKeywordTokens(raw string) ParseKeywordResult {
	res := ParseKeywordResult{}
	toks := SplitCommaList(raw)
	if len(toks) > maxKeywordTokens {
		res.Err = append(res.Err, "키워드는 20개까지입니다")
		toks = toks[:maxKeywordTokens]
	}
	re := regexp.MustCompile(`^(.+?)\s*[\(（]\s*(.+?)\s*[\)）]$`)
	for _, tok := range toks {
		name, g := tok, KWGroupAction
		if m := re.FindStringSubmatch(tok); len(m) == 3 {
			name = strings.TrimSpace(m[1])
			g = strings.TrimSpace(m[2])
			if g == "조치사항" {
				g = KWGroupAction
			}
		}
		okG := false
		for _, allow := range KWGroups() {
			if g == allow {
				okG = true
				break
			}
		}
		if !okG {
			res.Err = append(res.Err, "‘"+tok+"’의 그룹을 확인하세요. 대상·증상·원인·조치 중 하나")
			continue
		}
		rn := []rune(name)
		if len(rn) < 1 || len(rn) > 30 {
			res.Err = append(res.Err, "‘"+tok+"’의 이름은 1~30자여야 합니다")
			continue
		}
		res.OK = append(res.OK, KeywordCand{Keyword: name, Group: g})
	}
	return res
}

func DictMatchTerms(kws []ASKeyword) []string {
	var out []string
	for _, k := range kws {
		out = append(out, k.MatchTerms()...)
	}
	return out
}

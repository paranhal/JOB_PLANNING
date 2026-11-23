package model

import "strings"

const IndustryUnspecified = "업종 미지정"

// industryAlias 고객 industry 코드·라벨 묶음. 고객 값은 바꾸지 않는다.
var industryAlias = map[string]string{
	"school":   "학교",
	"학교":       "학교",
	"library":  "도서관",
	"도서관":      "도서관",
	"public":   "공공기관",
	"공공기관":     "공공기관",
	"private":  "민간",
	"민간":       "민간",
}

// IndustryKey 코드/라벨을 화면 묶음 키로 정규화한다.
func IndustryKey(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return IndustryUnspecified
	}
	if k, ok := industryAlias[s]; ok {
		return k
	}
	if k, ok := industryAlias[strings.ToLower(s)]; ok {
		return k
	}
	return s
}

// IndustryRawValues 묶음 키에 해당하는 원 저장 값(조회 IN 절).
func IndustryRawValues(key string) []string {
	k := IndustryKey(key)
	if k == IndustryUnspecified {
		return []string{""}
	}
	seen := map[string]bool{}
	var out []string
	for raw, mapped := range industryAlias {
		if mapped != k || seen[raw] {
			continue
		}
		seen[raw] = true
		out = append(out, raw)
	}
	if !seen[k] {
		out = append(out, k)
	}
	return out
}

// ── 95-219 업종 다중 선택 ─────────────────────────────────────────────────────────
// customers.industry 는 같은 컬럼에 「고정 순서 쉼표」로 여러 업종을 담는다(예: 「도서관,학교」). 값이 하나면 기존 데이터와 같은 모양이다.
// 「도서관,학교」는 도서관이면서 학교다 — 업종마다 따로 판단하고 대표 값을 정하지 않는다.

// IndustryOptions 선택지(고정 순서). 화면·검증·정렬이 같은 목록을 쓴다.
var IndustryOptions = []string{"도서관", "학교", "공공기관", "민간"}

func industryOrder(k string) int {
	for i, o := range IndustryOptions {
		if o == k {
			return i
		}
	}
	return len(IndustryOptions)
}

// industrySplit 쉼표(,)·전각 쉼표(，)·가운데점(、)·슬래시(/)·세미콜론으로 나눈다.
func industrySplit(raw string) []string {
	f := func(r rune) bool {
		switch r {
		case ',', '，', '、', '/', '／', ';', '；', '\n', '\r':
			return true
		}
		return false
	}
	return strings.FieldsFunc(raw, f)
}

// ParseIndustries 저장값(또는 입력)을 업종 목록으로. 별칭(school→학교 등)은 정규화하고, 중복은 한 번만 두며,
// 선택지는 고정 순서로, 목록에 없는 기존 값은 버리지 않고 뒤에 입력 순서대로 둔다. 빈 값이면 nil.
func ParseIndustries(raw string) []string {
	seen := map[string]bool{}
	var known, other []string
	for _, p := range industrySplit(raw) {
		p = strings.TrimSpace(p)
		if p == "" || p == IndustryUnspecified {
			continue
		}
		k := IndustryKey(p)
		if seen[k] {
			continue
		}
		seen[k] = true
		if industryOrder(k) < len(IndustryOptions) {
			known = append(known, k)
		} else {
			other = append(other, k)
		}
	}
	// 선택지 고정 순서
	var out []string
	for _, o := range IndustryOptions {
		for _, k := range known {
			if k == o {
				out = append(out, k)
			}
		}
	}
	return append(out, other...)
}

// JoinIndustries 업종 목록을 저장 문자열로(고정 순서·중복 제거). ParseIndustries 와 왕복이 된다.
func JoinIndustries(list []string) string {
	return strings.Join(ParseIndustries(strings.Join(list, ",")), ",")
}

// HasIndustry 저장값 raw 가 업종 key(별칭 포함)를 포함하는가. key 가 비었거나 「업종 미지정」이면 raw 가 비었는가.
func HasIndustry(raw, key string) bool {
	k := IndustryKey(key)
	if k == IndustryUnspecified {
		return len(ParseIndustries(raw)) == 0
	}
	for _, v := range ParseIndustries(raw) {
		if v == k {
			return true
		}
	}
	return false
}

// IndustryExtras 선택지에 없는 기존 값(수정 화면에 「기타 기존 값」 체크 항목으로 보여 저장 때 사라지지 않게 한다).
func IndustryExtras(raw string) []string {
	var out []string
	for _, v := range ParseIndustries(raw) {
		if industryOrder(v) >= len(IndustryOptions) {
			out = append(out, v)
		}
	}
	return out
}

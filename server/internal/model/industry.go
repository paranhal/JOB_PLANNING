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

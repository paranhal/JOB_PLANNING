package repository

import (
	"regexp"
	"sort"
	"strings"
)

// koreaRegionRules 주소에서 시·도 추출 (긴 표기 우선 — Contains 매칭)
var koreaRegionRules = []struct{ Prefix, Label string }{
	{"서울특별시", "서울"},
	{"부산광역시", "부산"},
	{"대구광역시", "대구"},
	{"인천광역시", "인천"},
	{"광주광역시", "광주"},
	{"대전광역시", "대전"},
	{"울산광역시", "울산"},
	{"세종특별자치시", "세종"},
	{"세종특별자치", "세종"}, // 표기 누락(시 없음) 호환
	{"제주특별자치도", "제주"},
	{"강원특별자치도", "강원"},
	{"전북특별자치도", "전북"},
	{"충청북도", "충북"},
	{"충청남도", "충남"},
	{"전라북도", "전북"},
	{"전라남도", "전남"},
	{"경상북도", "경북"},
	{"경상남도", "경남"},
	{"경기도", "경기"},
	{"강원도", "강원"},
	{"서울시", "서울"},
	{"부산시", "부산"},
	{"대구시", "대구"},
	{"인천시", "인천"},
	{"광주시", "광주"},
	{"대전시", "대전"},
	{"울산시", "울산"},
	{"세종시", "세종"},
	{"충북", "충북"},
	{"충남", "충남"},
	{"전북", "전북"},
	{"전남", "전남"},
	{"경북", "경북"},
	{"경남", "경남"},
	{"서울", "서울"},
	{"부산", "부산"},
	{"대구", "대구"},
	{"인천", "인천"},
	{"광주", "광주"},
	{"대전", "대전"},
	{"울산", "울산"},
	{"세종", "세종"},
	{"경기", "경기"},
	{"강원", "강원"},
	{"제주", "제주"},
}

// 우편번호·(우) 표기 제거: (우)32255, (31116), (31253충청… 등
var (
	rePostalParen = regexp.MustCompile(`\(\s*우?\s*\d{5}\s*\)?`)
	rePostalLead  = regexp.MustCompile(`^\s*\d{5}\b`)
	reWooMark     = regexp.MustCompile(`\(?\s*우\s*\)?`)
)

// NormalizeAddressForRegion 우편번호·장식 문자를 제거하고 공백을 정리한다.
func NormalizeAddressForRegion(address string) string {
	s := strings.TrimSpace(address)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "(우)", " ")
	s = reWooMark.ReplaceAllString(s, " ")
	s = rePostalParen.ReplaceAllString(s, " ")
	s = rePostalLead.ReplaceAllString(s, " ")
	// 남은 고아 괄호
	s = strings.ReplaceAll(s, "(", " ")
	s = strings.ReplaceAll(s, ")", " ")
	return strings.Join(strings.Fields(s), " ")
}

// ExtractKoreaRegion 주소 문자열에서 시·도(지역) 라벨 추출. 없으면 "".
// 설치자산에 지역 항목이 없으므로 고객(또는 관련) 주소에서 파생한다.
func ExtractKoreaRegion(address string) string {
	a := NormalizeAddressForRegion(address)
	if a == "" {
		return ""
	}
	// 1) 앞부분 접두 매칭
	for _, r := range koreaRegionRules {
		if strings.HasPrefix(a, r.Prefix) {
			return r.Label
		}
	}
	// 2) 주소 어디에든 시·도 표기가 있으면 추출 (긴 표기 우선)
	for _, r := range koreaRegionRules {
		if strings.Contains(a, r.Prefix) {
			return r.Label
		}
	}
	return ""
}

// CollectDistinctRegions 주소·점검사이트 지역에서 지역 목록 수집
func CollectDistinctRegions(addresses, siteRegions []string) []string {
	set := map[string]struct{}{}
	for _, a := range addresses {
		if lab := ExtractKoreaRegion(a); lab != "" {
			set[lab] = struct{}{}
		}
	}
	for _, r := range siteRegions {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		if lab := ExtractKoreaRegion(r); lab != "" {
			set[lab] = struct{}{}
		} else {
			set[r] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

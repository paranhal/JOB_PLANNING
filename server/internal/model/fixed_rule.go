package model

import (
	"sort"
	"strconv"
	"strings"
)

const FixedRuleLastMonday = "LAST_MONDAY_OF_MONTH"

// ParseFixedRule 사이트 설정의 고정일 규칙.
// LAST_MONDAY_OF_MONTH 이거나 1~31 일자를 콤마로 나열한다 (예: "5" · "5,15").
func ParseFixedRule(raw string) (days []int, lastMonday bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false
	}
	seen := map[int]bool{}
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.EqualFold(p, FixedRuleLastMonday) {
			lastMonday = true
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 31 || seen[n] {
			continue
		}
		seen[n] = true
		days = append(days, n)
	}
	sort.Ints(days)
	return days, lastMonday
}

// FormatFixedRule 체크한 일자와 마지막 월요일 여부를 저장 문자열로 만든다.
func FormatFixedRule(days []int, lastMonday bool) string {
	seen := map[int]bool{}
	var clean []int
	for _, n := range days {
		if n < 1 || n > 31 || seen[n] {
			continue
		}
		seen[n] = true
		clean = append(clean, n)
	}
	sort.Ints(clean)
	if lastMonday && len(clean) == 0 {
		return FixedRuleLastMonday
	}
	parts := make([]string, 0, len(clean)+1)
	for _, n := range clean {
		parts = append(parts, strconv.Itoa(n))
	}
	if lastMonday {
		parts = append(parts, FixedRuleLastMonday)
	}
	return strings.Join(parts, ",")
}

// HasFixedRule 고정일·마지막 월요일이 있으면 true.
func HasFixedRule(raw string) bool {
	days, last := ParseFixedRule(raw)
	return last || len(days) > 0
}

// FixedDaySet 템플릿 체크박스용 1~31 맵.
func FixedDaySet(raw string) map[int]bool {
	days, _ := ParseFixedRule(raw)
	out := map[int]bool{}
	for _, d := range days {
		out[d] = true
	}
	return out
}

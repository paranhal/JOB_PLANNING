package model

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	CauseCatCustom    = "custom"
	CauseCatCustomMax = 40
)

// NormalizeCauseCatCustom 직접 입력값. 앞뒤·연속 공백 정리.
func NormalizeCauseCatCustom(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if prevSpace {
				continue
			}
			b.WriteRune(' ')
			prevSpace = true
			continue
		}
		prevSpace = false
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

func CauseCatCustomOK(s string) bool {
	s = NormalizeCauseCatCustom(s)
	n := utf8.RuneCountInString(s)
	return n > 0 && n <= CauseCatCustomMax
}

// ApplyCauseCat1 1차 분류와 직접 입력값을 맞춘다. 오류 키를 돌려준다.
func ApplyCauseCat1(as *ASReceipt, cat1, custom string) string {
	if as == nil {
		return "cause_cat1"
	}
	cat1 = strings.TrimSpace(cat1)
	custom = NormalizeCauseCatCustom(custom)
	as.CauseCat1 = cat1
	if cat1 == CauseCatCustom {
		if custom == "" {
			as.CauseCat1Custom = ""
			as.CauseCat2 = ""
			as.CauseCat3 = ""
			return "cause_cat1_custom"
		}
		if utf8.RuneCountInString(custom) > CauseCatCustomMax {
			as.CauseCat1Custom = custom
			return "cause_cat1_custom_len"
		}
		as.CauseCat1Custom = custom
		as.CauseCat2 = ""
		as.CauseCat3 = ""
		return ""
	}
	as.CauseCat1Custom = ""
	return ""
}

// CauseCat1Label 화면·통계용 1차 라벨. custom 이면 입력값.
func CauseCat1Label(cat1, custom string, labels map[string]string) string {
	cat1 = strings.TrimSpace(cat1)
	if cat1 == CauseCatCustom {
		if s := NormalizeCauseCatCustom(custom); s != "" {
			return s
		}
		return "직접 입력"
	}
	if cat1 == "" {
		return ""
	}
	if labels != nil {
		if s := strings.TrimSpace(labels[cat1]); s != "" {
			return s
		}
	}
	return cat1
}

// CauseCat1GroupKey 통계 합산 키. custom 은 대소문자 무시한 입력값별.
func CauseCat1GroupKey(cat1, custom string) string {
	cat1 = strings.TrimSpace(cat1)
	if cat1 == CauseCatCustom {
		return CauseCatCustom + ":" + strings.ToLower(NormalizeCauseCatCustom(custom))
	}
	return cat1
}

func CauseCategoryLabels(cats []CauseCategory) map[string]string {
	out := map[string]string{}
	for _, c := range cats {
		if strings.TrimSpace(c.Code) == "" {
			continue
		}
		out[c.Code] = strings.TrimSpace(c.Label)
	}
	return out
}

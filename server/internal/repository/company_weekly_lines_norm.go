package repository

import (
	"crypto/sha1"
	"encoding/hex"
	"regexp"
	"strings"
)

// 95-218 ③ 주간 보고서 줄 비교의 유일한 규칙. 화면·저장·다시 집계·엑셀·HWPX 가 모두 이 함수로 「같은 줄」을 판정한다.

var weeklyLeadMarkRe = regexp.MustCompile(`^(?:[·•\-–—*+▪■□◦○●]+|[①-⑳]|\(?\d{1,2}[.)]\)?|\d{1,2}\s*[.)])\s*`)

// NormalizeWeeklyLine 줄 앞 표식(「·」「-」「*」「+」「•」「①」「1.」「(1)」)과 전각·특수 공백·연속 공백을 정리한 비교용 문자열.
// 표시용 글자는 바꾸지 않고 비교에만 쓴다. 「(기관용)」「(1주 지연)」처럼 괄호 안이 숫자만이 아닌 글자는 건드리지 않는다.
func NormalizeWeeklyLine(s string) string {
	s = strings.NewReplacer(" ", " ", "　", " ", " ", " ", " ", " ", "​", "", "\r", "").Replace(s)
	s = strings.TrimSpace(s)
	for i := 0; i < 3; i++ { // 「·1. 가나다」처럼 표식이 겹친 경우
		t := weeklyLeadMarkRe.ReplaceAllString(s, "")
		if t == s {
			break
		}
		s = strings.TrimSpace(t)
	}
	return strings.Join(strings.Fields(s), " ")
}

// WeeklyLineHashKey 키가 없는 자동 줄에 붙이는 안정 키(정규화한 글자의 해시). 같은 내용이면 같은 키라 숨김·짝짓기가 된다.
func WeeklyLineHashKey(text string) string {
	sum := sha1.Sum([]byte(NormalizeWeeklyLine(text)))
	return "txt:" + hex.EncodeToString(sum[:6])
}

// DedupeWeeklyLines 같은 구역 안에서 정규화가 같은 줄은 첫 줄만 남긴다(수동·자동 모두).
func DedupeWeeklyLines(in []CompanyWeeklyLine) []CompanyWeeklyLine {
	seen := map[string]bool{}
	out := make([]CompanyWeeklyLine, 0, len(in))
	for _, l := range in {
		n := NormalizeWeeklyLine(l.Text)
		if n == "" {
			continue
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, l)
	}
	return out
}

// SplitWeeklyLinesKeyed SplitWeeklyLines 와 같지만 키 없는 줄에 해시 키를 붙인다.
func SplitWeeklyLinesKeyed(text string) []CompanyWeeklyLine {
	lines := SplitWeeklyLines(text)
	for i := range lines {
		if strings.TrimSpace(lines[i].Key) == "" {
			lines[i].Key = WeeklyLineHashKey(lines[i].Text)
		}
	}
	return lines
}

// SameWeeklyTexts 두 줄 목록이 정규화 후 순서까지 같은지.
func SameWeeklyTexts(a, b []string) bool {
	na, nb := normList(a), normList(b)
	if len(na) != len(nb) {
		return false
	}
	for i := range na {
		if na[i] != nb[i] {
			return false
		}
	}
	return true
}

func normList(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range in {
		n := NormalizeWeeklyLine(s)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

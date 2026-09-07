package model

import "testing"

func TestParseFormatFixedRule(t *testing.T) {
	days, last := ParseFixedRule("")
	if len(days) != 0 || last {
		t.Fatalf("empty: %v %v", days, last)
	}
	days, last = ParseFixedRule(FixedRuleLastMonday)
	if !last || len(days) != 0 {
		t.Fatalf("last monday: %v %v", days, last)
	}
	days, last = ParseFixedRule("5,15,5,99,0")
	if last || len(days) != 2 || days[0] != 5 || days[1] != 15 {
		t.Fatalf("days: %v last=%v", days, last)
	}
	if got := FormatFixedRule([]int{15, 5}, false); got != "5,15" {
		t.Fatalf("format days=%q", got)
	}
	if got := FormatFixedRule(nil, true); got != FixedRuleLastMonday {
		t.Fatalf("format last=%q", got)
	}
	if !HasFixedRule("5") || HasFixedRule("") {
		t.Fatal("HasFixedRule")
	}
}

func TestDominantRegionTieBreak(t *testing.T) {
	// 조치원 3 · 연동 1 · 전의 1 → 조치원
	ids := []string{"a", "a2", "a3", "b", "c"}
	reg := map[string]string{"a": "조치원", "a2": "조치원", "a3": "조치원", "b": "연동", "c": "전의"}
	got := DominantRegion(CountRegionVisits(ids, func(id string) string { return reg[id] }))
	if got != "조치원" {
		t.Fatalf("최다 방문=%q", got)
	}

	// 방문 동수 2:2, 사이트 수 조치원 2 · 연동 1 → 조치원
	ids = []string{"s1", "s2", "e1", "e1"}
	reg = map[string]string{"s1": "조치원", "s2": "조치원", "e1": "연동"}
	got = DominantRegion(CountRegionVisits(ids, func(id string) string { return reg[id] }))
	if got != "조치원" {
		t.Fatalf("사이트 수=%q", got)
	}

	// 방문·사이트 모두 동수 → 가나다순 (가 먼저)
	ids = []string{"x", "y"}
	reg = map[string]string{"x": "전의", "y": "가람"}
	got = DominantRegion(CountRegionVisits(ids, func(id string) string { return reg[id] }))
	if got != "가람" {
		t.Fatalf("가나다=%q", got)
	}
}

package model

import (
	"strings"
	"testing"
)

func TestAssigneeColorOrderDistinct(t *testing.T) {
	names := []string{"양기헌", "이해진", "최혜영", "김기술", "이접수"}
	SetAssigneeColorOrder(names)
	seen := map[string]string{}
	for _, n := range names {
		b, _, _ := AssigneeColor(n)
		if prev, ok := seen[b]; ok {
			t.Fatalf("color collision %s and %s → %s", prev, n, b)
		}
		seen[b] = n
	}
	b, _, _ := AssigneeColor("")
	if b != ColorUnassigned.Border {
		t.Fatalf("empty: %s", b)
	}
}

func TestAssigneeColorMatchesProductFamilies(t *testing.T) {
	b, soft, text := AssigneeColor("양기헌")
	if b != ColorAnrobotics.Border || soft != ColorAnrobotics.Soft || text != ColorAnrobotics.Text {
		t.Fatalf("양기헌 want anrobotics got %s %s %s", b, soft, text)
	}
	b, soft, text = AssigneeColor("최혜영")
	if b != ColorKLAS.Border || soft != ColorKLAS.Soft || text != ColorKLAS.Text {
		t.Fatalf("최혜영 want klas got %s %s %s", b, soft, text)
	}
}

func TestWorkCardColorStylePrefersAssignee(t *testing.T) {
	// 제품이 KLAS여도 담당자가 양기헌이면 앤로보틱스 계열
	st := WorkCardColorStyle("양기헌", "KLAS", WBSourceAS)
	if !containsAll(st, ColorAnrobotics.Border, ColorAnrobotics.Soft) {
		t.Fatalf("assignee should win: %s", st)
	}
	st = WorkCardColorStyle("최혜영", "앤로보틱스", WBSourceMaintenance)
	if !containsAll(st, ColorKLAS.Border, ColorKLAS.Soft) {
		t.Fatalf("최혜영→KLAS: %s", st)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if p == "" || !strings.Contains(s, p) {
			return false
		}
	}
	return true
}

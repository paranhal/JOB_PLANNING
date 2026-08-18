package model

import (
	"strings"
	"testing"
	"time"
)

func TestVisitDeleteProtectedCurrentAndPast(t *testing.T) {
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.Local)
	cases := []struct {
		date string
		want bool
	}{
		{"2026-07-31", true},
		{"2026-08-01", true},
		{"2026-08-17", true},
		{"2026-08-31", true},
		{"2026-09-01", false},
		{"2026-11-10", false},
		{"", false},
		{"2026-8-1", true},
	}
	for _, c := range cases {
		if got := VisitDeleteProtected(c.date, now); got != c.want {
			t.Errorf("VisitDeleteProtected(%q)=%v want %v", c.date, got, c.want)
		}
	}
}

func TestDatedVisitsSkipsEmpty(t *testing.T) {
	list := []MaintenanceVisit{
		{VisitDate: "2026-11-10"},
		{VisitDate: ""},
		{VisitDate: "  "},
	}
	got := DatedVisits(list)
	if len(got) != 1 || got[0].VisitDate != "2026-11-10" {
		t.Fatalf("%+v", got)
	}
}

func TestProtectedVisitRangeMessageAugust(t *testing.T) {
	now := time.Date(2026, 8, 17, 0, 0, 0, 0, time.Local)
	got := ProtectedVisitRangeMessage(now, 62)
	if !strings.Contains(got, "2026년 1~8월 방문 62건") {
		t.Fatalf("문구: %s", got)
	}
	if !strings.Contains(got, "9월 이후만") {
		t.Fatalf("다음 달 안내 없음: %s", got)
	}
}

package model

import (
	"strings"
	"testing"
)

func TestFormatSalesDurationAndNextChip(t *testing.T) {
	if got := FormatSalesDuration(150); got != "2시간 30분" {
		t.Fatalf("요약=%q", got)
	}
	if got := FormatMinutesAsCard(90); got != "90분" {
		t.Fatalf("카드=%q", got)
	}
	label, class := SalesNextActionChip("", "", "2026-09-04")
	if label != "다음 행동 없음" || !strings.Contains(class, "text-gray-500") {
		t.Fatalf("없음=%s %s", label, class)
	}
	label, class = SalesNextActionChip("제안서 작성 및 제출", "2026-09-15", "2026-09-04")
	if label != "다음: 제안서 작성 및 제출 · 2026-09-15" || !strings.Contains(class, "text-orange-800") {
		t.Fatalf("미래=%s %s", label, class)
	}
	_, class = SalesNextActionChip("재통화", "2026-09-01", "2026-09-04")
	if !strings.Contains(class, "text-red-700") {
		t.Fatalf("지난 날=%s", class)
	}
	_, class = SalesNextActionChip("오늘 할 일", "2026-09-04", "2026-09-04")
	if !strings.Contains(class, "font-semibold") {
		t.Fatalf("오늘=%s", class)
	}
}

func TestGroupSalesActivitiesByDateNewestFirst(t *testing.T) {
	g := GroupSalesActivitiesByDate([]SalesActivity{
		{ActivityID: "1", ActivityDate: "2026-09-01", DurationMin: 90},
		{ActivityID: "2", ActivityDate: "2026-08-20", DurationMin: 30},
	})
	if len(g) != 2 || g[0].Date != "2026-09-01" || g[1].Date != "2026-08-20" {
		t.Fatalf("%+v", g)
	}
	if g[0].Heading != "2026-09-01 (화) · 1건" {
		t.Fatalf("머리글=%q", g[0].Heading)
	}
	sum := BuildSalesActivitySummary([]SalesActivity{
		{ActivityType: SalesActTypeVisit, DurationMin: 90},
		{ActivityType: SalesActTypeVisit, DurationMin: 60},
	}, []Code{{CodeValue: SalesActTypeVisit, CodeName: "방문"}})
	if sum.Count != 2 || sum.Minutes != 150 || sum.DurationLabel != "2시간 30분" || sum.TopLabel != "방문" {
		t.Fatalf("%+v", sum)
	}
}

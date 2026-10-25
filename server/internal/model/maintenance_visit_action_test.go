package model

import "testing"

func TestVisitSiteDurationMin(t *testing.T) {
	got, ok := VisitSiteDurationMin("10:00", "11:30")
	if !ok || got != 90 {
		t.Fatalf("90분: %d ok=%v", got, ok)
	}
	if _, ok := VisitSiteDurationMin("11:30", "10:00"); ok {
		t.Fatal("종료가 빠르면 거부")
	}
	if _, ok := VisitSiteDurationMin("10:00", "10:00"); ok {
		t.Fatal("같으면 거부")
	}
	got, ok = VisitSiteDurationMin("", "")
	if !ok || got != 0 {
		t.Fatalf("빈 값: %d ok=%v", got, ok)
	}
}

func TestFormatVisitTargetsLine(t *testing.T) {
	items := []MaintenanceVisitTarget{
		{Name: "A"}, {Name: "B"}, {Name: "C"}, {Name: "D"}, {Name: "E"},
	}
	if got := FormatVisitTargetsLine(items); got != "A, B, C 외 2대" {
		t.Fatalf("%q", got)
	}
	if FormatVisitTargetsLine(items[:2]) != "A, B" {
		t.Fatal("2건")
	}
}

func TestNormalizeVisitClock(t *testing.T) {
	got, ok := NormalizeVisitClock("9:5")
	if !ok || got != "09:05" {
		t.Fatalf("%q %v", got, ok)
	}
	if _, ok := NormalizeVisitClock("25:00"); ok {
		t.Fatal("잘못된 시각")
	}
}

package model

import (
	"testing"
	"time"
)

func TestParseStatsLookbackDefaultMonth(t *testing.T) {
	now := time.Date(2026, 9, 1, 15, 0, 0, 0, time.Local)
	lb := ParseStatsLookback("", "", "", "", "", "", now)
	if lb.RangeParam != "1m" || lb.From != "2026-08-02" || lb.To != "2026-09-01" {
		t.Fatalf("default 1m: %+v", lb)
	}
	if lb.Headline != "최근 1개월 (2026-08-02 ~ 2026-09-01)" {
		t.Fatalf("headline=%q", lb.Headline)
	}
}

func TestParseStatsLookbackTwoDaysIncludesToday(t *testing.T) {
	now := time.Date(2026, 9, 3, 10, 0, 0, 0, time.Local)
	lb := ParseStatsLookback("2d", "", "", "day", "", "", now)
	if lb.From != "2026-09-02" || lb.To != "2026-09-03" {
		t.Fatalf("2d: %s ~ %s", lb.From, lb.To)
	}
}

func TestParseStatsLookbackTwoWeeks(t *testing.T) {
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)
	lb := ParseStatsLookback("2w", "", "", "", "", "", now)
	if lb.From != "2026-08-19" || lb.To != "2026-09-01" {
		t.Fatalf("2w: %s ~ %s", lb.From, lb.To)
	}
}

func TestParseStatsLookbackClipsMetricsBase(t *testing.T) {
	now := time.Date(2026, 9, 3, 0, 0, 0, 0, time.Local)
	lb := ParseStatsLookback("6m", "", "", "month", "", "2026-08-03", now)
	if !lb.ClippedByBase || lb.From != "2026-08-03" || lb.To != "2026-09-03" {
		t.Fatalf("clip: %+v", lb)
	}
}

func TestParseStatsLookbackCustomFromBeforeBase(t *testing.T) {
	now := time.Date(2026, 9, 3, 0, 0, 0, 0, time.Local)
	lb := ParseStatsLookback("", "2026-01-01", "2026-09-01", "week", "custom", "2026-08-03", now)
	if !lb.Custom || !lb.ClippedByBase || lb.From != "2026-08-03" || lb.To != "2026-09-01" {
		t.Fatalf("custom clip: %+v", lb)
	}
}

func TestFitStatsViewShortRange(t *testing.T) {
	fitted, note, narrowed := FitStatsView(StatsViewMonth, 2)
	if fitted != StatsViewDay || !narrowed || note == "" {
		t.Fatalf("2일+월: %s %v %q", fitted, narrowed, note)
	}
	keep, _, n2 := FitStatsView(StatsViewWeek, 90)
	if keep != StatsViewWeek || n2 {
		t.Fatalf("3개월+주: %s %v", keep, n2)
	}
}

func TestAutoStatsViewByDays(t *testing.T) {
	if AutoStatsView(31) != StatsViewDay {
		t.Fatal("31일 → 일별")
	}
	if AutoStatsView(32) != StatsViewWeek || AutoStatsView(180) != StatsViewWeek {
		t.Fatal("32~180일 → 주별")
	}
	if AutoStatsView(181) != StatsViewMonth {
		t.Fatal("181일 → 월별")
	}
	if ChartBucketNote(StatsViewWeek) != "주별로 묶어 표시" {
		t.Fatalf("note=%q", ChartBucketNote(StatsViewWeek))
	}
}

func TestParseStatsLookbackCaps(t *testing.T) {
	now := time.Date(2026, 9, 3, 0, 0, 0, 0, time.Local)
	lb := ParseStatsLookback("999d", "", "", "", "", "", now)
	if lb.N != LookbackMaxDays || lb.Unit != LookbackUnitDay {
		t.Fatalf("cap days: %+v", lb)
	}
}

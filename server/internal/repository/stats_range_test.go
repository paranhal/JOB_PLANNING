package repository

import (
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestResolveStatsRange_DayAndOverdueMetric(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.Local)
	from, to, ok, label := ResolveStatsRange(model.StatsQuery{
		Period: model.StatsPeriodDay,
		Date:   "2026-07-30",
	}, now)
	if !ok || label != "2026/07/30" {
		t.Fatalf("day: ok=%v label=%q", ok, label)
	}
	if from.Format("2006-01-02") != "2026-07-30" || to.Format("2006-01-02") != "2026-07-31" {
		t.Fatalf("day range %v ~ %v", from, to)
	}

	from, to, ok, _ = ResolveStatsRange(model.StatsQuery{
		Period: model.StatsPeriodRange,
		From:   "2026-07-01",
		To:     "2026-07-31",
	}, now)
	if !ok || from.Format("2006-01-02") != "2026-07-01" || to.Format("2006-01-02") != "2026-08-01" {
		t.Fatalf("range %v ~ %v ok=%v", from, to, ok)
	}

	from, to, ok, label = ResolveStatsRange(model.StatsQuery{
		Period:  model.StatsPeriodQuarter,
		Quarter: "2026-Q3",
	}, now)
	if !ok || label != "2026년 3분기" {
		t.Fatalf("quarter label=%q ok=%v", label, ok)
	}
	if from.Format("2006-01-02") != "2026-07-01" || to.Format("2006-01-02") != "2026-10-01" {
		t.Fatalf("quarter range %v ~ %v", from, to)
	}
}

func TestMetricStatusCond_Overdue(t *testing.T) {
	c := metricStatusCond(model.StatsMetricOverdue)
	if c == "" || !containsAll(c, "in_progress", "> 3") {
		t.Fatalf("overdue cond unexpected: %s", c)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !containsStr(s, p) {
			return false
		}
	}
	return true
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		})())
}

package model

import "testing"

func TestExecutionRatePctPlanFidelity(t *testing.T) {
	// 예정 4곳, 계획대로 2곳 완료 · 2곳 일정 변경 → 50% (처리 건수가 4여도 동일)
	b := StatsBucketCounts{
		Mnt: StatsWorkSlice{Planned: 4, Process: 4, Modified: 2, OnPlan: 2},
	}
	if got := b.ExecutionRatePct(); got != 50 {
		t.Fatalf("got %.1f want 50", got)
	}

	// 전부 계획대로 → 100%
	b2 := StatsBucketCounts{
		AS: StatsWorkSlice{Planned: 3, Process: 3, Modified: 0, OnPlan: 3},
	}
	if got := b2.ExecutionRatePct(); got != 100 {
		t.Fatalf("got %.1f want 100", got)
	}

	// 처리 건수가 예정보다 많아도 100% 초과 불가 (OnPlan은 Planned로 캡)
	b3 := StatsBucketCounts{
		AS: StatsWorkSlice{Planned: 2, Process: 10, OnPlan: 5},
	}
	if got := b3.ExecutionRatePct(); got != 100 {
		t.Fatalf("cap at 100: got %.1f", got)
	}

	// 예정 0 → 0
	if (StatsBucketCounts{}).ExecutionRatePct() != 0 {
		t.Fatal("empty planned")
	}
}

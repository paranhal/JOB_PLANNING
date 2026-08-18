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

	// 예정 0 → 산식은 0을 반환하되, 화면은 HasExecutionRate로 「—」와 구분한다(§4.3).
	empty := StatsBucketCounts{}
	if empty.ExecutionRatePct() != 0 {
		t.Fatal("empty planned")
	}
	if empty.HasExecutionRate() {
		t.Fatal("denom 0 must not have execution rate")
	}
	if !b.HasExecutionRate() {
		t.Fatal("planned>0 must have execution rate")
	}
}

func TestStatsReliabilityGrades(t *testing.T) {
	zero := StatsReliability(0, true, 0)
	if zero.ShowValue || zero.Grade != StatsGradeNA || zero.GradeMark != "⚪" {
		t.Fatalf("denom0: %+v", zero)
	}
	if zero.Reason != "해당 기간 예정 업무 없음" {
		t.Fatalf("denom0 reason=%q", zero.Reason)
	}

	small := StatsReliability(3, false, 80)
	if small.ShowValue || small.ShowTarget || small.Grade != StatsGradeNA {
		t.Fatalf("sample<5 should hide value: %+v", small)
	}

	ref := StatsReliability(10, false, 80)
	if !ref.ShowValue || ref.ShowTarget || ref.Grade != StatsGradeRef || ref.GradeMark != "🟡" {
		t.Fatalf("sample 5-19: %+v", ref)
	}

	ok := StatsReliability(20, false, 91)
	if !ok.ShowValue || !ok.ShowTarget || ok.Grade != StatsGradeTrusted || ok.GradeMark != "🟢" {
		t.Fatalf("sample>=20: %+v", ok)
	}

	capped := ok.CapGradeIfImport(true)
	if capped.Grade != StatsGradeRef || capped.ShowTarget {
		t.Fatalf("import must cap trusted→ref: %+v", capped)
	}
	if ok.CapGradeIfImport(false).Grade != StatsGradeTrusted {
		t.Fatal("exclude import keeps trusted")
	}
}

func TestStatsLongestTopN(t *testing.T) {
	if StatsLongestTopN(0) != 1 || StatsLongestTopN(9) != 1 {
		t.Fatal("default 1")
	}
	if StatsLongestTopN(10) != 2 || StatsLongestTopN(19) != 2 {
		t.Fatal("10+ → 2")
	}
	if StatsLongestTopN(20) != 3 {
		t.Fatal("20+ → 3")
	}
}

func TestFormatStatsDuration(t *testing.T) {
	if FormatStatsDuration(90) != "1시간 30분" {
		t.Fatalf("90: %s", FormatStatsDuration(90))
	}
	if FormatStatsDuration(60) != "1시간" {
		t.Fatalf("60: %s", FormatStatsDuration(60))
	}
	if FormatStatsDuration(45) != "45분" {
		t.Fatalf("45: %s", FormatStatsDuration(45))
	}
}

func TestDailyAvgCompleted(t *testing.T) {
	avg, ok := DailyAvgCompleted(18, 5)
	if !ok || avg != 3.6 {
		t.Fatalf("18/5=%v ok=%v want 3.6", avg, ok)
	}
	if _, ok := DailyAvgCompleted(10, 0); ok {
		t.Fatal("워킹데이 0은 표시 불가")
	}
}

func TestDateLabelMDW(t *testing.T) {
	if got := DateLabelMDW("2026-08-10"); got != "08-10(월)" {
		t.Fatalf("got %q", got)
	}
}

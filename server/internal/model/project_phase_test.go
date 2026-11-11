package model

import "testing"

func Test95_120P_SplitAndLate(t *testing.T) {
	spans := SplitPhaseDates("2026-01-01", "2026-01-10", 3)
	if len(spans) != 3 || spans[0].Start != "2026-01-01" || spans[0].End != "2026-01-04" ||
		spans[1].Start != "2026-01-05" || spans[1].End != "2026-01-07" ||
		spans[2].Start != "2026-01-08" || spans[2].End != "2026-01-10" {
		t.Fatalf("10일 3단계 %+v", spans)
	}
	short := SplitPhaseDates("2026-01-01", "2026-01-02", 4)
	if short[0].Start != "2026-01-01" || short[1].End != "2026-01-02" || short[2].Start != "" || short[3].End != "" {
		t.Fatalf("짧은 기간 %+v", short)
	}
	empty := SplitPhaseDates("", "", 2)
	if empty[0].Start != "" || empty[1].End != "" {
		t.Fatalf("날짜 없음 %+v", empty)
	}
	if !PhaseLate("2026-11-10", PhaseActive, "2026-11-11") {
		t.Fatal("어제 종료는 지연")
	}
	if PhaseLate("2026-11-11", PhaseActive, "2026-11-11") || PhaseLate("", PhaseActive, "2026-11-11") || PhaseLate("2026-11-01", PhaseDone, "2026-11-11") {
		t.Fatal("오늘·빈 날·완료는 지연이 아니다")
	}
	sum := SummarizePhases([]ProjectPhase{
		{Name: "착수", Status: PhaseActive, IsActive: true, PlanEnd: "2026-11-01"},
		{Name: "설계", Status: PhasePlanned, IsActive: true},
		{Name: "숨김", Status: PhasePlanned, IsActive: false},
	}, "2026-11-11")
	if sum.Current != "착수" || !sum.Late || sum.Label() != "완료 0 / 전체 2" {
		t.Fatalf("진행 중 %+v %s", sum, sum.Label())
	}
	next := SummarizePhases([]ProjectPhase{
		{Name: "착수", Status: PhaseDone, IsActive: true},
		{Name: "설계", Status: PhasePlanned, IsActive: true},
	}, "2026-11-11")
	if next.Current != "다음: 설계" || next.Label() != "완료 1 / 전체 2" {
		t.Fatalf("다음 %+v %s", next, next.Label())
	}
	none := SummarizePhases(nil, "2026-11-11")
	if none.Current != "—" || none.Label() != "—" {
		t.Fatalf("없음 %+v", none)
	}
	done := SummarizePhases([]ProjectPhase{{Name: "끝", Status: PhaseDone, IsActive: true}}, "2026-11-11")
	if done.Current != "—" || done.Label() != "완료 1 / 전체 1" {
		t.Fatalf("전부 완료 %+v %s", done, done.Label())
	}
	if _, err := PhaseNameOK("가"); err != nil {
		t.Fatal(err)
	}
	long := ""
	for i := 0; i < 101; i++ {
		long += "가"
	}
	if _, err := PhaseNameOK(long); err == nil {
		t.Fatal("101자는 거절")
	}
	if _, _, _, _, err := PhaseDatesOK("2026-02-02", "2026-02-01", "", ""); err == nil {
		t.Fatal("종료가 시작보다 빠르면 거절")
	}
	if PhaseSectionOpen(ProjectKindBuild) != true || PhaseSectionOpen(ProjectKindMaintenance) {
		t.Fatal("펼침 기준")
	}
	if PhaseEmptyHint(ProjectKindMaintenance) != "사용 안 함" || PhaseEmptyHint(ProjectKindOther) != "설정 안 함" {
		t.Fatal("빈 안내")
	}
	if NormalizePhaseMergeName("설 계(1)") != "설계1" {
		t.Fatal("합치기 이름")
	}
}

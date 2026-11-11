package model

import "testing"

func Test95_175A_FormatCommaAndMatch(t *testing.T) {
	if FormatIntComma(1200) != "1,200" || FormatIntComma(-12) != "-12" {
		t.Fatalf("쉼표 %s %s", FormatIntComma(1200), FormatIntComma(-12))
	}
	if !SiteImportMatch("홍천초등학교", "", "강원 홍천군", "홍천", "") {
		t.Fatal("부분 문자열")
	}
	if !SiteImportMatch("홍천초등학교", "", "", "ㅎㅊ", "") {
		t.Fatal("초성")
	}
	if SiteImportMatch("홍천초등학교", "", "강원", "홍천", "서울") {
		t.Fatal("지역이 다르면 제외")
	}
	if !RegionAuxMatch("강원 홍천군", "홍천") {
		t.Fatal("지역 보조")
	}
	if QtyCredit(SiteDone, 10, 1) != 10 || QtyCredit(SiteInProgress, 10, 12) != 10 || QtyCredit(SitePlanned, 10, 4) != 0 {
		t.Fatal("수량 인식")
	}
	if BothRateText(0, 0, 0, 0, "식") != "— · 수량 —" {
		t.Fatalf("분모 0 %s", BothRateText(0, 0, 0, 0, "식"))
	}
	if EffectiveStatus(PkgAuto, false, true) != StatusCandidate {
		t.Fatal("완료 후보")
	}
	if EffectiveStatus(PhasePlanned, true, false) != PhaseActive {
		t.Fatal("활동이면 진행")
	}
	biz := func(d string) bool { return WeekendBiz(d) }
	if PlanPace("", "2026-11-12", 1, 10, 5, 2, biz) != PaceBefore {
		t.Fatal("종료일 없으면 판정 전")
	}
	if PlanPace("2026-11-01", "2026-11-12", 1, 10, 5, 2, biz) != PaceOver {
		t.Fatal("기한 지남")
	}
}

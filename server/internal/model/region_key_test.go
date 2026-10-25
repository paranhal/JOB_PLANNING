package model

import "testing"

func TestSiteRegionAndMatchRegionKey(t *testing.T) {
	hong := MaintenanceSiteConfig{Region: "충남", AddrSigungu: "홍성군"}
	sido, gu := SiteRegion(hong)
	if sido != "충남" || gu != "홍성군" {
		t.Fatalf("충남 홍성군 got %q %q", sido, gu)
	}
	if !MatchRegionKey(hong, "충남") {
		t.Fatal("시·도 키 불일치")
	}
	if !MatchRegionKey(hong, "충남|홍성군") {
		t.Fatal("시·군·구 키 불일치")
	}
	if MatchRegionKey(hong, "충남|예산군") || MatchRegionKey(hong, "충남|") {
		t.Fatal("다른 시·군·구에 매칭됨")
	}

	fromAddr := MaintenanceSiteConfig{AddrSido: "충청남도", AddrSigungu: "  홍성군  "}
	sido, gu = SiteRegion(fromAddr)
	if sido != "충남" || gu != "홍성군" {
		t.Fatalf("AddrSido 보강 got %q %q", sido, gu)
	}

	emptyGu := MaintenanceSiteConfig{Region: "충남"}
	sido, gu = SiteRegion(emptyGu)
	if sido != "충남" || gu != "" {
		t.Fatalf("빈 시·군·구 %q %q", sido, gu)
	}
	if FormatRegionKey(sido, gu) != "충남|" {
		t.Fatalf("충남| 키 %q", FormatRegionKey(sido, gu))
	}
	if !MatchRegionKey(emptyGu, "충남|") || !MatchRegionKey(emptyGu, "충남") {
		t.Fatal("충남| / 충남 매칭 실패")
	}
	if RegionKeyLabel("충남|") != "충남 (시·군·구 미지정)" {
		t.Fatalf("라벨 %q", RegionKeyLabel("충남|"))
	}

	none := MaintenanceSiteConfig{}
	sido, gu = SiteRegion(none)
	if sido != "" || gu != "" {
		t.Fatalf("둘 다 비어 있음 %q %q", sido, gu)
	}
	if SiteRegionKey(none) != RegionUnspecified {
		t.Fatalf("미지정 키 %q", SiteRegionKey(none))
	}
	if !MatchRegionKey(none, RegionUnspecified) || !MatchRegionKey(none, "") {
		t.Fatal("지역 미지정 매칭 실패")
	}

	spaced := MaintenanceSiteConfig{Region: "충남", AddrSigungu: "수원시   영통구"}
	if _, gu = SiteRegion(spaced); gu != "수원시 영통구" {
		t.Fatalf("공백 정규화 %q", gu)
	}
	if SiteRegionLabel(hong) != "충남 홍성군" {
		t.Fatalf("사이트 라벨 %q", SiteRegionLabel(hong))
	}
}

func TestCollapseMonthTargetRegionKeys(t *testing.T) {
	got := CollapseMonthTargetRegionKeys([]string{"충남|홍성군", "충남", "충남|예산군", "충남"})
	if len(got) != 1 || got[0] != "충남" {
		t.Fatalf("시·도 우선 %+v", got)
	}
	got = CollapseMonthTargetRegionKeys([]string{"충남|홍성군", "충남|예산군"})
	if len(got) != 2 {
		t.Fatalf("자식만 %+v", got)
	}
}

func TestRegionRegisterSummary(t *testing.T) {
	s := RegionRegisterSummary([]string{"충남|홍성군", "충남|예산군"}, 57, 3)
	if s != "충남 홍성군·예산군 — 57곳 등록 / 3곳 건너뜀" {
		t.Fatalf("요약 %q", s)
	}
	s = RegionRegisterSummary([]string{"충남", "충남|홍성군", "세종"}, 10, 0)
	if s != "충남·세종 — 10곳 등록" {
		t.Fatalf("시·도 우선 요약 %q", s)
	}
}

func TestShortRegionNameMoved(t *testing.T) {
	if ShortRegionName("충청남도") != "충남" || ShortRegionName("세종특별자치시") != "세종" {
		t.Fatal("축약 실패")
	}
}

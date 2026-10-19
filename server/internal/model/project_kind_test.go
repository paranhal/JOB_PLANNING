package model

import "testing"

func TestNormalizeProjectKindLegacyMap(t *testing.T) {
	cases := []struct{ in, want string }{
		{"supply", ProjectKindBuild},
		{"network", ProjectKindBuild},
		{"신규구축", ProjectKindBuild},
		{"장비납품", ProjectKindBuild},
		{"consumable", ProjectKindOther},
		{"소모품납품", ProjectKindOther},
		{"기타", ProjectKindOther},
		{"other", ProjectKindOther},
		{"construction", ProjectKindConstruction},
		{"maintenance", ProjectKindMaintenance},
	}
	for _, c := range cases {
		if got := NormalizeProjectKind(c.in); got != c.want {
			t.Fatalf("%s → %s want %s", c.in, got, c.want)
		}
		if !ValidProjectKind(c.in) {
			t.Fatalf("%s 가 유효해야 한다", c.in)
		}
	}
	if ProjectKindLabel("other") != "기타" {
		t.Fatal("기타 라벨")
	}
	if len(ProjectKindOptions()) != 5 {
		t.Fatalf("유형 %d", len(ProjectKindOptions()))
	}
}

func TestEffectiveKindConfirmedInferredUnclassified(t *testing.T) {
	set := WorkProject{ProjectKind: ProjectKindBuild, ProjectKindSet: 1, SalesBizType: SalesBizGoods}
	if got := set.EffectiveKind(); got.Kind != ProjectKindBuild || got.Source != KindSourceConfirmed {
		t.Fatalf("확정 %+v", got)
	}
	inf := WorkProject{ProjectKind: ProjectKindMaintenance, ProjectKindSet: 0, SalesBizType: SalesBizDevelop}
	if got := inf.EffectiveKind(); got.Kind != ProjectKindDev || got.Source != KindSourceInferred {
		t.Fatalf("추정 develop→dev %+v", got)
	}
	goods := WorkProject{SalesBizType: SalesBizGoods}
	if got := goods.EffectiveKind(); got.Kind != ProjectKindOther || got.Source != KindSourceInferred {
		t.Fatalf("물품 기타 %+v", got)
	}
	unk := WorkProject{SalesBizType: "weird"}
	if got := unk.EffectiveKind(); got.Kind != ProjectKindOther || got.Source != KindSourceInferred {
		t.Fatalf("알 수 없는 영업 유형 %+v", got)
	}
	empty := WorkProject{ProjectKind: ProjectKindMaintenance, ProjectKindSet: 0}
	if got := empty.EffectiveKind(); got.Kind != ProjectKindUnclassified || got.Source != KindSourceUnclassified {
		t.Fatalf("연결 없음 %+v", got)
	}
}

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

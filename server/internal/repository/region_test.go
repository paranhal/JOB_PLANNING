package repository

import "testing"

func TestExtractKoreaRegion(t *testing.T) {
	cases := []struct {
		addr string
		want string
	}{
		{"(우)32255 충남 홍성군 홍북읍 선화로 22", "충남"},
		{"(31116) 충청남도 천안시 동남구 단대로 119", "충남"},
		{"(31253충청남도 천안시 동남구 병천면", "충남"},
		{"(30150)세종특별자치시 한누리대로 2023", "세종"},
		{"(우)30004 세종특별자치 전의면 동신길 10", "세종"},
		{"서울특별시 중구 세종대로 110", "서울"},
		{"경기도 성남시 분당구", "경기"},
		{"", ""},
		{"건물 3층", ""},
	}
	for _, tc := range cases {
		got := ExtractKoreaRegion(tc.addr)
		if got != tc.want {
			t.Errorf("ExtractKoreaRegion(%q)=%q, want %q", tc.addr, got, tc.want)
		}
	}
}

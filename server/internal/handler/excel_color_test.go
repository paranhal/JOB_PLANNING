package handler

import "testing"

func TestXlColorStripsARGBAndHash(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"FF1E3A5F", "1E3A5F"},
		{"#FF1E3A5F", "1E3A5F"},
		{"1E3A5F", "1E3A5F"},
		{"#1E3A5F", "1E3A5F"},
		{"ff1e3a5f", "1E3A5F"},
		{"  #ff1e3a5f  ", "1E3A5F"},
		{"FFFFFFFF", "FFFFFF"},
		{"FFFFFF00", "FFFF00"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := xlColor(tc.in); got != tc.want {
			t.Fatalf("xlColor(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

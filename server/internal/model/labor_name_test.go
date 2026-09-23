package model

import "testing"

func TestCleanJobName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"① IT기획자", "IT기획자"},
		{"1 IT기획자", "IT기획자"},
		{"01. IT PM", "IT PM"},
		{"⑦ UI/UX기획/개발자", "UI/UX기획/개발자"},
	}
	for _, tc := range cases {
		if got := CleanJobName(tc.in); got != tc.want {
			t.Errorf("CleanJobName(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

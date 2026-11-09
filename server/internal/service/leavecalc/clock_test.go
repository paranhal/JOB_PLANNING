package leavecalc

import "testing"

func Test95_162_HalfResolve(t *testing.T) {
	cases := []struct {
		in, start, end, part string
	}{
		{"09:00", "09:00", "14:00", "am"},
		{"10:00", "10:00", "15:00", "am"},
		{"13:00", "13:00", "17:00", "pm"},
		{"14:00", "14:00", "18:00", "pm"},
		{"12:30", "13:00", "17:00", "pm"},
		{"12:00", "13:00", "17:00", "pm"},
	}
	for _, tc := range cases {
		s, e, p, err := ResolveHalf(tc.in)
		if err != nil || s != tc.start || e != tc.end || p != tc.part {
			t.Fatalf("%s → %s %s %s err=%v want %s %s %s", tc.in, s, e, p, err, tc.start, tc.end, tc.part)
		}
	}
	if _, _, _, err := ResolveHalf("08:59"); err == nil {
		t.Fatal("before 09:00")
	}
	if _, _, _, err := ResolveHalf("14:01"); err == nil {
		t.Fatal("after 14:00")
	}
}

func Test95_162_ClockWorkMinutesLunch(t *testing.T) {
	got, err := ClockWorkMinutes("11:00", "14:00")
	if err != nil || got != 120 {
		t.Fatalf("11-14 lunch skip %d err=%v", got, err)
	}
	got, err = ClockWorkMinutes("15:00", "18:00")
	if err != nil || got != 180 {
		t.Fatalf("15-18 %d err=%v", got, err)
	}
	got, err = ClockWorkMinutes("09:00", "18:00")
	if err != nil || got != DefaultWorkMinutesPerDay {
		t.Fatalf("full day %d err=%v", got, err)
	}
}

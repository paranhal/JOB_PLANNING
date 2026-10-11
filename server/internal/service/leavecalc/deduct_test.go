package leavecalc

import "testing"

func TestTypesDeductPolicy(t *testing.T) {
	deduct := map[string]bool{
		TypeAnnual: true, TypeHalf: true, TypeMonthly: true, TypeVacation: true,
		TypeEarlyLeave: true, TypeOuting: true,
		TypeAbsence: false, TypeOther: false, TypeSubstitute: false,
	}
	for _, td := range Types() {
		want, ok := deduct[td.Key]
		if !ok {
			t.Fatalf("unexpected type %s", td.Key)
		}
		if td.DeductsBalance != want {
			t.Errorf("%s deduct=%v want %v", td.Key, td.DeductsBalance, want)
		}
	}
}

func TestDeductMinutesTable(t *testing.T) {
	p := DefaultPolicy()
	cases := []struct {
		typ  string
		days int
		clk  int
		want int
	}{
		{TypeAnnual, 2, 0, 960},
		{TypeMonthly, 1, 0, 480},
		{TypeVacation, 3, 0, 1440},
		{TypeHalf, 0, 0, DefaultHalfDayMinutes},
		{TypeEarlyLeave, 0, DefaultEarlyLeaveMinutes, DefaultEarlyLeaveMinutes},
		{TypeEarlyLeave, 0, 45, 45},
		{TypeOuting, 0, 45, 45},
		{TypeAbsence, 2, 0, 0},
		{TypeOther, 1, 120, 0},
		{TypeSubstitute, 1, 0, 0},
	}
	for _, tc := range cases {
		got, err := DeductMinutes(tc.typ, tc.days, tc.clk, p)
		if err != nil {
			t.Fatalf("%s: %v", tc.typ, err)
		}
		if got != tc.want {
			t.Errorf("%s minutes=%d want %d", tc.typ, got, tc.want)
		}
	}
}

func TestHoursAndClockMinutes(t *testing.T) {
	if HoursToMinutes(3) != DefaultEarlyLeaveMinutes {
		t.Fatalf("3h=%d want %d", HoursToMinutes(3), DefaultEarlyLeaveMinutes)
	}
	got, err := ClockRangeMinutes("15:00", "18:00")
	if err != nil || got != 180 {
		t.Fatalf("15-18=%d err=%v", got, err)
	}
	if DefaultPolicy().HalfDayMinutes() != DefaultHalfDayMinutes {
		t.Fatal("half minutes")
	}
}

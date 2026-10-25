package model

import "testing"

func TestPeriodMonthTargetAllowed(t *testing.T) {
	if err := PeriodMonthTargetAllowed("semi", 2); err != nil {
		t.Fatal(err)
	}
	if err := PeriodMonthTargetAllowed("semi", 8); err != nil {
		t.Fatal(err)
	}
	if err := PeriodMonthTargetAllowed("semi", 7); err != nil {
		t.Fatal("하반기 7월은 허용")
	}
	if err := PeriodMonthTargetAllowed("quarterly", 5); err != nil {
		t.Fatal(err)
	}
	if err := PeriodMonthTargetAllowed("yearly", 12); err != nil {
		t.Fatal(err)
	}
	if err := PeriodMonthTargetAllowed("semi", 0); err == nil {
		t.Fatal("0월 허용")
	}
	if err := PeriodMonthTargetAllowed("monthly", 3); err == nil {
		t.Fatal("월 주기 허용")
	}
	if err := PeriodMonthTargetAllowed("odd_bimonthly", 3); err == nil {
		t.Fatal("격월 허용")
	}
}

func TestMonthInCyclePeriod(t *testing.T) {
	if MonthInCyclePeriod("semi", 6) != true || MonthInCyclePeriod("semi", 7) != true {
		t.Fatal("반기 경계")
	}
	if MonthInCyclePeriod("quarterly", 3) != true || MonthInCyclePeriod("quarterly", 4) != true {
		t.Fatal("분기 경계")
	}
}

func TestRegionLabel(t *testing.T) {
	if RegionLabel("") != RegionUnspecified {
		t.Fatal("빈 지역")
	}
	if RegionLabel(" 세종 ") != "세종" {
		t.Fatal("trim")
	}
}

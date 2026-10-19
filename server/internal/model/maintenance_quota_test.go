package model

import "testing"

func TestCalendarQuarterHelpers(t *testing.T) {
	if CalendarQuarter(9) != 3 || CalendarQuarter(10) != 4 || CalendarQuarter(0) != 0 {
		t.Fatalf("CalendarQuarter 9/10/0 = %d/%d/%d", CalendarQuarter(9), CalendarQuarter(10), CalendarQuarter(0))
	}
	s, e := QuarterMonthRange(3)
	if s != 7 || e != 9 {
		t.Fatalf("Q3 months %d-%d", s, e)
	}
	if !IsLastMonthOfQuarter(9) || IsLastMonthOfQuarter(10) {
		t.Fatal("last month of quarter")
	}
	from, to := QuarterDateBounds(2026, 3)
	if from != "2026-07-01" || to != "2026-09-30" {
		t.Fatalf("Q3 bounds %s %s", from, to)
	}
	if !VisitInCalendarQuarter("2026-09-10", 2026, 3) {
		t.Fatal("9월 방문은 Q3")
	}
	if VisitInCalendarQuarter("2026-10-05", 2026, 3) {
		t.Fatal("10월 방문은 Q3가 아님")
	}
	if VisitInCalendarQuarter("", 2026, 3) {
		t.Fatal("빈 날짜는 미확정")
	}
}

func TestInspectionCycleDue(t *testing.T) {
	cases := []struct {
		cycle string
		month int
		want  bool
	}{
		{"monthly", 8, true},
		{"", 3, true},
		{"odd_bimonthly", 8, false},
		{"odd_bimonthly", 7, true},
		{"even_bimonthly", 8, true},
		{"even_bimonthly", 7, false},
		{"quarterly", 8, false},
		{"quarterly", 7, true},
		{"quarterly", 10, true},
		{"semi", 1, true},
		{"semi", 8, false},
		{"yearly", 1, true},
		{"yearly", 8, false},
	}
	for _, tc := range cases {
		got := InspectionCycleDue(tc.cycle, tc.month)
		if got != tc.want {
			t.Errorf("InspectionCycleDue(%q,%d)=%v want %v", tc.cycle, tc.month, got, tc.want)
		}
	}
}

func TestSiteVisitSlotCount(t *testing.T) {
	if n := SiteVisitSlotCount(MaintenanceSiteConfig{HasKlas: true, HasRfid: true}); n != 2 {
		t.Fatalf("both=%d want 2", n)
	}
	if n := SiteVisitSlotCount(MaintenanceSiteConfig{HasKlas: true}); n != 1 {
		t.Fatalf("klas=%d want 1", n)
	}
	if n := SiteVisitSlotCount(MaintenanceSiteConfig{}); n != 1 {
		t.Fatalf("neither=%d want 1", n)
	}
}

func TestSiteDueProductSlots(t *testing.T) {
	cfg := MaintenanceSiteConfig{CustomerID: "c1", ShortName: "가나", HasKlas: true, HasRfid: true, InspectionCycle: "monthly"}
	aug := SiteDueProductSlots(cfg, 8)
	if len(aug) != 2 {
		t.Fatalf("8월 slots=%d want 2", len(aug))
	}
	odd := MaintenanceSiteConfig{CustomerID: "c2", HasKlas: true, InspectionCycle: "odd_bimonthly"}
	if n := SiteDueProductSlots(odd, 8); len(n) != 0 {
		t.Fatalf("짝수월 홀수격월=%d want 0", len(n))
	}
}

func TestSummarizeMonthVisitQuotaDualProduct(t *testing.T) {
	configs := []MaintenanceSiteConfig{
		{CustomerID: "a", HasKlas: true, HasRfid: true, InspectionCycle: "monthly"},
		{CustomerID: "b", HasKlas: true, InspectionCycle: "odd_bimonthly"},
		{CustomerID: "c", HasRfid: true, InspectionCycle: "even_bimonthly"},
	}
	aug := SummarizeMonthVisitQuota(configs, 2026, 8)
	if aug.DueSites != 2 || aug.Slots != 3 {
		t.Fatalf("8월 sites=%d slots=%d want 2/3 (월 2 + 짝수격월 RFID 1, 홀수격월 제외)", aug.DueSites, aug.Slots)
	}
	if aug.KlasSlots != 1 || aug.RfidSlots != 2 {
		t.Fatalf("8월 klas=%d rfid=%d", aug.KlasSlots, aug.RfidSlots)
	}
	jul := SummarizeMonthVisitQuota(configs, 2026, 7)
	if jul.DueSites != 2 || jul.Slots != 3 {
		t.Fatalf("7월 sites=%d slots=%d want 2/3", jul.DueSites, jul.Slots)
	}
}

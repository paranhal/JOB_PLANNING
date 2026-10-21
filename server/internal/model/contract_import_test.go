package model

import (
	"testing"
	"time"
)

func TestSplitContractYearsOracles(t *testing.T) {
	today := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	school := SplitContractYears(time.Date(2024, 12, 30, 0, 0, 0, 0, time.UTC), time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC), 5252622222, today)
	if len(school) != 2 || school[0].StartDate != "2024-12-30" || school[0].EndDate != "2025-12-29" || school[1].StartDate != "2025-12-30" || school[1].EndDate != "2026-02-28" {
		t.Fatalf("school years %+v", school)
	}
	if school[0].Amount != 4500486176 || school[1].Amount != 752136046 {
		t.Fatalf("school amt %d %d", school[0].Amount, school[1].Amount)
	}
	if school[0].Amount+school[1].Amount != 5252622222 {
		t.Fatal("school remainder")
	}

	hotel := SplitContractYears(time.Date(2025, 1, 17, 0, 0, 0, 0, time.UTC), time.Date(2030, 2, 16, 0, 0, 0, 0, time.UTC), 17036250, today)
	if len(hotel) != 5 {
		t.Fatalf("hotel n=%d %+v", len(hotel), hotel)
	}
	want := []struct{ s, e string }{
		{"2025-01-17", "2026-01-16"},
		{"2026-01-17", "2027-01-16"},
		{"2027-01-17", "2028-01-16"},
		{"2028-01-17", "2029-01-16"},
		{"2029-01-17", "2030-02-16"},
	}
	sum := 0
	for i, w := range want {
		if hotel[i].StartDate != w.s || hotel[i].EndDate != w.e {
			t.Fatalf("hotel %d got %s %s want %s %s", i, hotel[i].StartDate, hotel[i].EndDate, w.s, w.e)
		}
		sum += hotel[i].Amount
	}
	if sum != 17036250 {
		t.Fatalf("hotel sum %d", sum)
	}
	if hotel[0].Amount != hotel[1].Amount || hotel[1].Amount != hotel[2].Amount {
		t.Fatalf("같은 일수 연차 금액 %d %d %d", hotel[0].Amount, hotel[1].Amount, hotel[2].Amount)
	}
	if hotel[0].Amount != 3348536 || hotel[4].Amount != 3632932 {
		t.Fatalf("hotel 1·5차 %d %d", hotel[0].Amount, hotel[4].Amount)
	}

	giga := SplitContractYears(time.Date(2025, 3, 18, 0, 0, 0, 0, time.UTC), time.Date(2031, 10, 31, 0, 0, 0, 0, time.UTC), 249678000, today)
	if len(giga) != 7 || giga[6].StartDate != "2031-03-18" || giga[6].EndDate != "2031-10-31" || giga[6].Amount != 23533107 {
		t.Fatalf("giga %+v", giga)
	}

	one := SplitContractYears(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC), 100, today)
	if one != nil {
		t.Fatalf("1년 skip %+v", one)
	}
	merged := SplitContractYears(time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC), 100, today)
	if merged != nil {
		t.Fatalf("31일 합침 skip %+v", merged)
	}
	leap := SplitContractYears(time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC), time.Date(2026, 2, 27, 0, 0, 0, 0, time.UTC), 100, today)
	if len(leap) != 2 || leap[0].EndDate != "2025-02-27" || leap[1].StartDate != "2025-02-28" {
		t.Fatalf("leap %+v", leap)
	}
}

func TestMapContractKindLabel(t *testing.T) {
	k, ok := MapContractKindLabel("용역")
	if !ok || k != ProjectKindDev {
		t.Fatalf("용역 %s %v", k, ok)
	}
	k, ok = MapContractKindLabel("구매")
	if !ok || k != ProjectKindOther {
		t.Fatalf("구매 %s %v", k, ok)
	}
	k, err := InferContractKind("", "네트워크 구축")
	if err != nil || k != ProjectKindBuild {
		t.Fatalf("구축 추론 %s %v", k, err)
	}
	k, err = InferContractKind("", "운영 사업")
	if err != nil || k != ProjectKindOther {
		t.Fatalf("빈 유형 기타 %s %v", k, err)
	}
	m, err := NormalizeContractMethod("수의계약")
	if err != nil || m != "수의" {
		t.Fatalf("수의계약 %s %v", m, err)
	}
	m, err = NormalizeContractMethod("입찰")
	if err != nil || m != "입찰" {
		t.Fatalf("입찰 %s %v", m, err)
	}
	if _, err := NormalizeContractMethod("모름"); err == nil {
		t.Fatal("모르는 계약방식")
	}
}

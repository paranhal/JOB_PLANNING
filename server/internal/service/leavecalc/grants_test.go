package leavecalc

import (
	"reflect"
	"testing"
)

func TestYearlyGrantDaysTable(t *testing.T) {
	cases := []struct {
		y    int
		days int
	}{
		{1, 15},
		{2, 15},
		{3, 16},
		{4, 16},
		{5, 17},
		{21, 25},
		{23, 25},
	}
	for _, tc := range cases {
		if got := YearlyGrantDays(tc.y); got != tc.days {
			t.Errorf("y=%d days=%d want %d", tc.y, got, tc.days)
		}
	}
	if YearlyGrantDays(0) != 0 {
		t.Errorf("y=0 should be 0")
	}
}

func grantKeys(gs []Grant) []string {
	out := make([]string, len(gs))
	for i, g := range gs {
		out[i] = g.GrantKey
	}
	return out
}

func TestComputeGrantsMonthEnd(t *testing.T) {
	p := DefaultPolicy()
	got, err := ComputeGrants("2026-01-31", "2026-03-31", p)
	if err != nil || !got.Measurable {
		t.Fatalf("err=%v measurable=%v", err, got.Measurable)
	}
	if len(got.Grants) != 2 {
		t.Fatalf("grants=%v", got.Grants)
	}
	if got.Grants[0].GrantKey != "M:2026-02" || got.Grants[0].GrantedOn != "2026-02-28" {
		t.Fatalf("first=%+v want M:2026-02 on 2026-02-28", got.Grants[0])
	}
	if got.Grants[1].GrantKey != "M:2026-03" || got.Grants[1].GrantedOn != "2026-03-31" {
		t.Fatalf("second=%+v want M:2026-03 on 2026-03-31", got.Grants[1])
	}
	if got.Grants[0].ExpiresOn != "2027-01-31" || got.Grants[1].ExpiresOn != "2027-01-31" {
		t.Fatalf("monthly expire should be H+1y 2027-01-31, got %s %s", got.Grants[0].ExpiresOn, got.Grants[1].ExpiresOn)
	}
	early, err := ComputeGrants("2026-01-31", "2026-02-27", p)
	if err != nil {
		t.Fatal(err)
	}
	if len(early.Grants) != 0 {
		t.Fatalf("before Feb 28 should have 0 grants, got %v", early.Grants)
	}
}

func TestComputeGrantsLeapYearHire(t *testing.T) {
	p := DefaultPolicy()
	// 2024-02-29 + 1 month → 2024-03-29
	got, err := ComputeGrants("2024-02-29", "2024-03-29", p)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Grants) != 1 || got.Grants[0].GrantedOn != "2024-03-29" {
		t.Fatalf("leap +1m = %+v", got.Grants)
	}
	// non-leap anniversary clamped to Feb 28
	ann, err := ComputeGrants("2024-02-29", "2025-02-28", p)
	if err != nil {
		t.Fatal(err)
	}
	var yearly []Grant
	for _, g := range ann.Grants {
		if g.Source == SourceAutoYearly {
			yearly = append(yearly, g)
		}
	}
	if len(yearly) != 1 || yearly[0].GrantedOn != "2025-02-28" || yearly[0].GrantKey != "Y:2025" {
		t.Fatalf("leap 1y anniversary=%+v", yearly)
	}
	if yearly[0].Minutes != 15*DefaultWorkMinutesPerDay {
		t.Fatalf("y=1 minutes=%d", yearly[0].Minutes)
	}
	// 11 monthly grants expire on H+1y = 2025-02-28 (exclusive that day)
	monthly := 0
	for _, g := range ann.Grants {
		if g.Source == SourceAutoMonthly {
			monthly++
			if g.ExpiresOn != "2025-02-28" {
				t.Fatalf("monthly expire=%s", g.ExpiresOn)
			}
		}
	}
	if monthly != 11 {
		t.Fatalf("monthly count=%d want 11", monthly)
	}
}

func TestComputeGrantsExactlyOneYear(t *testing.T) {
	p := DefaultPolicy()
	dayBefore, err := ComputeGrants("2024-03-15", "2025-03-14", p)
	if err != nil {
		t.Fatal(err)
	}
	yearly := 0
	monthly := 0
	for _, g := range dayBefore.Grants {
		if g.Source == SourceAutoYearly {
			yearly++
		} else {
			monthly++
		}
	}
	if yearly != 0 || monthly != 11 {
		t.Fatalf("day before 1y: yearly=%d monthly=%d", yearly, monthly)
	}

	onDay, err := ComputeGrants("2024-03-15", "2025-03-15", p)
	if err != nil {
		t.Fatal(err)
	}
	yearly, monthly = 0, 0
	var yGrant Grant
	for _, g := range onDay.Grants {
		if g.Source == SourceAutoYearly {
			yearly++
			yGrant = g
		} else {
			monthly++
		}
	}
	if yearly != 1 || monthly != 11 {
		t.Fatalf("on 1y: yearly=%d monthly=%d grants=%v", yearly, monthly, grantKeys(onDay.Grants))
	}
	if yGrant.GrantedOn != "2025-03-15" || yGrant.ExpiresOn != "2026-03-15" || yGrant.Minutes != 15*480 {
		t.Fatalf("yearly grant=%+v", yGrant)
	}
}

func TestComputeGrantsNoHireDate(t *testing.T) {
	got, err := ComputeGrants("", "2026-06-01", DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if got.Measurable {
		t.Fatal("missing hire must be unmeasurable")
	}
	if got.Grants != nil && len(got.Grants) != 0 {
		t.Fatalf("unmeasurable grants=%v", got.Grants)
	}
	bal, err := Evaluate("", "2026-06-01", DefaultPolicy(), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if bal.Measurable || bal.AvailableMinutes != 0 {
		t.Fatalf("unmeasurable balance must not look like 0-days: %+v", bal)
	}
}

func TestComputeGrantsBeforeLedgerStart(t *testing.T) {
	p := DefaultPolicy()
	p.LedgerStartDate = "2026-01-01"
	got, err := ComputeGrants("2020-01-01", "2026-06-01", p)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range got.Grants {
		if g.GrantedOn < p.LedgerStartDate {
			t.Fatalf("grant before ledger: %+v", g)
		}
	}
	// 2026-01-01 is 6th anniversary → y=6, days=15+floor(5/2)=17
	var y2026 *Grant
	for i := range got.Grants {
		if got.Grants[i].GrantKey == "Y:2026" {
			y2026 = &got.Grants[i]
		}
		if got.Grants[i].GrantKey == "Y:2025" {
			t.Fatal("Y:2025 is before ledger_start")
		}
	}
	if y2026 == nil || y2026.Minutes != 17*480 || y2026.GrantedOn != "2026-01-01" {
		t.Fatalf("Y:2026=%+v", y2026)
	}
}

func TestComputeGrantsIdempotentKeys(t *testing.T) {
	p := DefaultPolicy()
	a, err := ComputeGrants("2024-06-10", "2025-08-10", p)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ComputeGrants("2024-06-10", "2025-08-10", p)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(grantKeys(a.Grants), grantKeys(b.Grants)) {
		t.Fatalf("keys differ: %v vs %v", grantKeys(a.Grants), grantKeys(b.Grants))
	}
	seen := map[string]bool{}
	for _, g := range a.Grants {
		if seen[g.GrantKey] {
			t.Fatalf("duplicate key %s", g.GrantKey)
		}
		seen[g.GrantKey] = true
	}
	later, err := ComputeGrants("2024-06-10", "2026-06-10", p)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range grantKeys(a.Grants) {
		if !seenAmong(later.Grants, k) {
			t.Fatalf("later as-of lost key %s", k)
		}
	}
}

func seenAmong(gs []Grant, key string) bool {
	for _, g := range gs {
		if g.GrantKey == key {
			return true
		}
	}
	return false
}

func TestComputeGrantsTenureDaysOnAnniversaries(t *testing.T) {
	p := DefaultPolicy()
	hire := "2000-04-01"
	cases := []struct {
		asOf string
		y    int
		days int
	}{
		{"2001-04-01", 1, 15},
		{"2002-04-01", 2, 15},
		{"2003-04-01", 3, 16},
		{"2004-04-01", 4, 16},
		{"2005-04-01", 5, 17},
		{"2021-04-01", 21, 25},
		{"2023-04-01", 23, 25},
	}
	for _, tc := range cases {
		got, err := ComputeGrants(hire, tc.asOf, p)
		if err != nil {
			t.Fatal(err)
		}
		key := "Y:" + tc.asOf[:4]
		var found *Grant
		for i := range got.Grants {
			if got.Grants[i].GrantKey == key {
				found = &got.Grants[i]
			}
		}
		if found == nil {
			t.Fatalf("missing %s at %s", key, tc.asOf)
		}
		if found.Minutes != tc.days*480 {
			t.Errorf("%s minutes=%d want %d (y=%d days=%d)", key, found.Minutes, tc.days*480, tc.y, tc.days)
		}
	}
}

func TestMonthlyMaxEleven(t *testing.T) {
	got, err := ComputeGrants("2024-01-10", "2024-12-10", DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Grants) != 11 {
		t.Fatalf("want 11 monthly, got %d %v", len(got.Grants), grantKeys(got.Grants))
	}
	if got.Grants[0].GrantedOn != "2024-02-10" || got.Grants[10].GrantedOn != "2024-12-10" {
		t.Fatalf("range %+v … %+v", got.Grants[0], got.Grants[10])
	}
}

func TestNextMonthlyGrantDate(t *testing.T) {
	next, err := NextMonthlyGrantDate("2026-01-31", "2026-02-01", DefaultPolicy())
	if err != nil || next != "2026-02-28" {
		t.Fatalf("next=%q err=%v", next, err)
	}
	none, err := NextMonthlyGrantDate("2024-03-15", "2025-03-15", DefaultPolicy())
	if err != nil || none != "" {
		t.Fatalf("after 1y next=%q err=%v", none, err)
	}
}

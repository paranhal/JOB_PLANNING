package leavecalc

import "testing"

func TestComputeFiscalGrantsNoHire(t *testing.T) {
	got, err := ComputeFiscalGrants("", "2026-06-01", DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if got.Measurable || len(got.Grants) != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestComputeFiscalGrantsHireYearProportional(t *testing.T) {
	got, err := ComputeFiscalGrants("2026-07-01", "2026-08-01", DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if !got.Measurable || len(got.Grants) != 1 {
		t.Fatalf("%+v", got)
	}
	g := got.Grants[0]
	if g.GrantKey != "F:2026" || g.Source != SourceAutoFiscal {
		t.Fatalf("%+v", g)
	}
	if g.GrantedOn != "2026-07-01" || g.ExpiresOn != "2026-12-31" {
		t.Fatalf("dates %+v", g)
	}
	wantDays := roundHalfUp(15*184, 365)
	if g.Minutes != wantDays*480 {
		t.Fatalf("minutes=%d want %d days", g.Minutes, wantDays)
	}
}

func TestComputeFiscalGrantsThirdYear16(t *testing.T) {
	got, err := ComputeFiscalGrants("2023-01-01", "2025-01-01", DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	var y2025 Grant
	for _, g := range got.Grants {
		if g.GrantKey == "F:2025" {
			y2025 = g
		}
	}
	if y2025.Minutes != 16*480 {
		t.Fatalf("3년차=%+v grants=%v", y2025, grantKeys(got.Grants))
	}
}

func TestComputeFiscalGrantsCap25(t *testing.T) {
	got, err := ComputeFiscalGrants("2000-01-01", "2025-01-01", DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	var last Grant
	for _, g := range got.Grants {
		if g.GrantKey == "F:2025" {
			last = g
		}
	}
	if last.Minutes != 25*480 {
		t.Fatalf("cap %+v", last)
	}
}

func TestComputeFiscalGrantsLeapYear(t *testing.T) {
	full, err := ComputeFiscalGrants("2024-01-01", "2024-01-01", DefaultPolicy())
	if err != nil || len(full.Grants) != 1 || full.Grants[0].Minutes != 15*480 {
		t.Fatalf("leap full %+v err=%v", full, err)
	}
	part, err := ComputeFiscalGrants("2024-03-01", "2024-03-01", DefaultPolicy())
	if err != nil || len(part.Grants) != 1 {
		t.Fatalf("%+v %v", part, err)
	}
	want := roundHalfUp(15*306, 366)
	if part.Grants[0].Minutes != want*480 {
		t.Fatalf("leap part days want %d got %d", want, part.Grants[0].Minutes/480)
	}
}

func TestComputeFiscalGrantsIdempotentKeys(t *testing.T) {
	a, err := ComputeFiscalGrants("2024-06-10", "2026-06-10", DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	b, err := ComputeFiscalGrants("2024-06-10", "2026-06-10", DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Grants) != len(b.Grants) {
		t.Fatal("len")
	}
	for i := range a.Grants {
		if a.Grants[i].GrantKey != b.Grants[i].GrantKey {
			t.Fatalf("%s vs %s", a.Grants[i].GrantKey, b.Grants[i].GrantKey)
		}
	}
}

func TestSplitNeedAndPartial(t *testing.T) {
	d, a := SplitNeed(960, 480)
	if d != 480 || a != 480 {
		t.Fatalf("%d %d", d, a)
	}
	grants := []Grant{{GrantKey: "F:2026", Minutes: 480, GrantedOn: "2026-01-01", ExpiresOn: "2026-12-31"}}
	used, left, err := ConsumeFIFOPartial(grants, nil, 960, "2026-06-01")
	if err != nil || left != 480 || len(used) != 1 || used[0].Minutes != 480 {
		t.Fatalf("partial used=%+v left=%d err=%v", used, left, err)
	}
}

package leavecalc

import "testing"

func TestConsumeFIFOEarliestExpiryFirst(t *testing.T) {
	grants := []Grant{
		{GrantKey: "Y:2026", Minutes: 7200, GrantedOn: "2026-03-01", ExpiresOn: "2027-03-01", Source: SourceAutoYearly},
		{GrantKey: "M:2026-02", Minutes: 480, GrantedOn: "2026-02-01", ExpiresOn: "2026-06-01", Source: SourceAutoMonthly},
	}
	used, err := ConsumeFIFO(grants, nil, 720, "2026-03-15")
	if err != nil {
		t.Fatal(err)
	}
	if len(used) != 2 || used[0].GrantKey != "M:2026-02" || used[0].Minutes != 480 {
		t.Fatalf("fifo first=%+v", used)
	}
	if used[1].GrantKey != "Y:2026" || used[1].Minutes != 240 {
		t.Fatalf("fifo second=%+v", used)
	}
	left, err := ValidRemainingMinutes(grants, used, "2026-03-15")
	if err != nil || left != 7200-240 {
		t.Fatalf("left=%d err=%v", left, err)
	}
}

func TestConsumeRestoreExpire(t *testing.T) {
	grants := []Grant{
		{GrantKey: "Y:2026", Minutes: 7200, GrantedOn: "2026-03-01", ExpiresOn: "2027-03-01"},
	}
	used, err := ConsumeFIFO(grants, nil, 480, "2026-06-01")
	if err != nil {
		t.Fatal(err)
	}
	restored := Restore(used)
	if len(restored) != 1 || restored[0].Minutes != -480 || restored[0].Kind != KindRestore {
		t.Fatalf("restore=%+v", restored)
	}
	all := append(append([]Consumption{}, used...), restored...)
	left, err := ValidRemainingMinutes(grants, all, "2026-06-01")
	if err != nil || left != 7200 {
		t.Fatalf("after restore left=%d", left)
	}

	exp, err := Expire(grants, nil, "2027-03-01")
	if err != nil {
		t.Fatal(err)
	}
	if len(exp) != 1 || exp[0].Minutes != 7200 || exp[0].Kind != KindExpire {
		t.Fatalf("expire=%+v", exp)
	}
	again, err := Expire(grants, exp, "2027-03-01")
	if err != nil || len(again) != 0 {
		t.Fatalf("expire idempotent=%v err=%v", again, err)
	}
	leftOnDay, err := ValidRemainingMinutes(grants, nil, "2027-03-01")
	if err != nil || leftOnDay != 0 {
		t.Fatalf("on expire day valid remaining=%d (exclusive expire)", leftOnDay)
	}
}

func TestPendingReservation(t *testing.T) {
	grants := []Grant{
		{GrantKey: "Y:2026", Minutes: 3 * 480, GrantedOn: "2026-03-01", ExpiresOn: "2027-03-01"},
	}
	asOf := "2026-07-01"
	ok1, err := CanReserve(grants, nil, 0, 2*480, asOf)
	if err != nil || !ok1 {
		t.Fatalf("first 2d reserve ok=%v err=%v", ok1, err)
	}
	ok2, err := CanReserve(grants, nil, 2*480, 2*480, asOf)
	if err != nil || ok2 {
		t.Fatalf("second 2d should block, ok=%v err=%v", ok2, err)
	}
	avail, err := AvailableMinutes(grants, nil, 2*480, asOf)
	if err != nil || avail != 480 {
		t.Fatalf("avail after 2d pending=%d", avail)
	}
}

func TestConsumeInsufficient(t *testing.T) {
	grants := []Grant{
		{GrantKey: "Y:2026", Minutes: 480, GrantedOn: "2026-03-01", ExpiresOn: "2027-03-01"},
	}
	_, err := ConsumeFIFO(grants, nil, 960, "2026-04-01")
	if err != ErrInsufficientBalance {
		t.Fatalf("err=%v", err)
	}
}

func TestEvaluateUnmeasurableNotZero(t *testing.T) {
	b, err := Evaluate("   ", "2026-01-01", DefaultPolicy(), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if b.Measurable {
		t.Fatal("want unmeasurable")
	}
}

func TestExactlyOneYearMonthlyExpiredYearlyValid(t *testing.T) {
	p := DefaultPolicy()
	gr, err := ComputeGrants("2024-03-15", "2025-03-15", p)
	if err != nil {
		t.Fatal(err)
	}
	avail, err := AvailableMinutes(gr.Grants, nil, 0, "2025-03-15")
	if err != nil {
		t.Fatal(err)
	}
	if avail != 15*480 {
		t.Fatalf("on 1y anniversary available=%d want 15d (monthly expired exclusive)", avail)
	}
}

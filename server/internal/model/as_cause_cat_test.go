package model

import "testing"

func TestApplyCauseCat1Custom(t *testing.T) {
	as := &ASReceipt{}
	if code := ApplyCauseCat1(as, CauseCatCustom, ""); code != "cause_cat1_custom" {
		t.Fatalf("empty: %s", code)
	}
	if code := ApplyCauseCat1(as, CauseCatCustom, "   "); code != "cause_cat1_custom" {
		t.Fatalf("space: %s", code)
	}
	long := ""
	for i := 0; i < 41; i++ {
		long += "가"
	}
	if code := ApplyCauseCat1(as, CauseCatCustom, long); code != "cause_cat1_custom_len" {
		t.Fatalf("len: %s", code)
	}
	if code := ApplyCauseCat1(as, CauseCatCustom, "  CCTV  "); code != "" {
		t.Fatalf("ok: %s", code)
	}
	if as.CauseCat1 != CauseCatCustom || as.CauseCat1Custom != "CCTV" || as.CauseCat2 != "" {
		t.Fatalf("%+v", as)
	}
	if CauseCat1Label(as.CauseCat1, as.CauseCat1Custom, nil) != "CCTV" {
		t.Fatal(CauseCat1Label(as.CauseCat1, as.CauseCat1Custom, nil))
	}
	if code := ApplyCauseCat1(as, "rfid", "CCTV"); code != "" {
		t.Fatal(code)
	}
	if as.CauseCat1Custom != "" || as.CauseCat1 != "rfid" {
		t.Fatalf("cleared %+v", as)
	}
	if CauseCat1GroupKey(CauseCatCustom, "Cctv") != CauseCat1GroupKey(CauseCatCustom, "cctv") {
		t.Fatal("group key")
	}
}

package model

import "testing"

func TestParseReportSignatureBoxEmptyUsesBase(t *testing.T) {
	base := DefaultReportSignatureBox()
	got := ParseReportSignatureBox("", base)
	if got != base {
		t.Fatalf("%+v", got)
	}
	got = ParseReportSignatureBox("  ", base)
	if got != base {
		t.Fatalf("%+v", got)
	}
}

func TestParseReportSignatureBoxOverridesAndKeepsNegative(t *testing.T) {
	base := DefaultReportSignatureBox()
	got := ParseReportSignatureBox(`{"size":5000,"gap":900,"ny":-300}`, base)
	if got.Size != 5000 || got.Gap != 900 || got.NY != -300 {
		t.Fatalf("%+v", got)
	}
	got = ParseReportSignatureBox(`{"ny":-900}`, base)
	if got.Size != base.Size || got.Gap != base.Gap || got.NY != -900 {
		t.Fatalf("부분만 덮어야 한다 %+v", got)
	}
	got = ParseReportSignatureBox(`{"gap":0,"ny":0}`, base)
	if got.Gap != 0 || got.NY != 0 {
		t.Fatalf("0 보정을 막으면 안 된다 %+v", got)
	}
}

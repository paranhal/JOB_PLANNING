package model

import "testing"

func TestASOpenHref(t *testing.T) {
	if ASOpenHref("R1") != "/as/R1/action" {
		t.Fatal(ASOpenHref("R1"))
	}
	if ASOpenHrefQuery("R1", "err=x") != "/as/R1/action?err=x" {
		t.Fatal(ASOpenHrefQuery("R1", "err=x"))
	}
	if ASLegacyHref("R1") != "/as/R1?legacy=1" {
		t.Fatal(ASLegacyHref("R1"))
	}
	if ASEmbedHref("R1") != "/as/R1/action?embed=1" {
		t.Fatal(ASEmbedHref("R1"))
	}
}

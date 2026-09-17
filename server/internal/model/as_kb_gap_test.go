package model

import "testing"

func TestNormalizeKBQuery(t *testing.T) {
	if NormalizeKBQuery("  RFID   태그 ") != "rfid 태그" {
		t.Fatalf("%q", NormalizeKBQuery("  RFID   태그 "))
	}
	if NormalizeKBQuery("\t") != "" {
		t.Fatal("empty")
	}
}

func TestGapRecentLabel(t *testing.T) {
	if GapRecentLabel("2026-09-12 10:00:00") != "09-12" {
		t.Fatalf("%q", GapRecentLabel("2026-09-12 10:00:00"))
	}
}

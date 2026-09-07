package model

import "testing"

func TestUrgencyFromReason(t *testing.T) {
	if UrgencyFromReason("none") != "normal" {
		t.Fatal("해당 없음은 normal")
	}
	if UrgencyFromReason("") != "normal" {
		t.Fatal("빈값은 normal")
	}
	if UrgencyFromReason("rfid_ops_stop") != "high" {
		t.Fatal("사유가 있으면 high")
	}
	if UrgencyFromReason("other") != "high" {
		t.Fatal("기타도 high")
	}
}

func TestSyncScheduleConfirmed(t *testing.T) {
	if !SyncScheduleConfirmed("2026-09-01") {
		t.Fatal("예정일이 있으면 확정")
	}
	if SyncScheduleConfirmed("") || SyncScheduleConfirmed("  ") {
		t.Fatal("미정은 미확정")
	}
}

func TestAssetUrgencyFamily(t *testing.T) {
	if g := AssetUrgencyFamily("rfid", "", ""); g != "rfid" {
		t.Fatalf("rfid cat: %s", g)
	}
	if g := AssetUrgencyFamily("homepage", "", ""); g != "web" {
		t.Fatalf("homepage: %s", g)
	}
	if g := AssetUrgencyFamily("materials", "", ""); g != "klas" {
		t.Fatalf("materials: %s", g)
	}
	if g := AssetUrgencyFamily("", "자가대출기", ""); g != "rfid" {
		t.Fatalf("자가대출: %s", g)
	}
}

func TestNormalizePartyKind(t *testing.T) {
	if NormalizePartyKind("") != PartyKindCustomer {
		t.Fatal("기본값 customer")
	}
	if NormalizePartyKind("partner") != PartyKindPartner {
		t.Fatal("partner 유지")
	}
	if PartyKindLabel("own") != "자사" {
		t.Fatal(PartyKindLabel("own"))
	}
}

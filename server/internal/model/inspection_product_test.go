package model

import "testing"

func TestSiteDueProductSlotsLegacyUnchanged(t *testing.T) {
	cfg := MaintenanceSiteConfig{CustomerID: "c1", ShortName: "관", HasKlas: true, HasRfid: true, InspectionCycle: "monthly"}
	slots := SiteDueProductSlots(cfg, 8)
	if len(slots) != 2 || slots[0].SlotKey != "klas" || slots[1].SlotKey != "rfid" {
		t.Fatalf("%+v", slots)
	}
}

func TestSiteDueProductSlotsNewMode(t *testing.T) {
	empty := MaintenanceSiteConfig{
		CustomerID: "c3", ShortName: "학교", InspectionCycle: "monthly",
		OrgID: "O03", Products: []InspectionProduct{},
	}
	slots := SiteDueProductSlots(empty, 8)
	if len(slots) != 1 || slots[0].SlotKey != "generic" {
		t.Fatalf("empty %+v", slots)
	}
	two := empty
	two.Products = []InspectionProduct{
		{Code: "NET", Name: "네트워크장비", IsActive: true},
		{Code: "OFF", Name: "비활성", IsActive: false},
		{Code: "SRV", Name: "서버", IsActive: true},
	}
	slots = SiteDueProductSlots(two, 8)
	if len(slots) != 2 || slots[0].ProductType != "네트워크장비" || slots[0].SlotKey != "NET" {
		t.Fatalf("two %+v", slots)
	}
	if VisitProductSlotKeyFor("KLAS", []InspectionProduct{}) != "generic" {
		t.Fatal("신 모드에서 KLAS 레거시 매칭 금지")
	}
	if VisitProductSlotKeyFor("네트워크장비", two.Products) != "NET" {
		t.Fatal("이름 매칭")
	}
}

func TestUseLegacyInspectionSlots(t *testing.T) {
	if !UseLegacyInspectionSlots(OrgIDLibrary, 0) || !UseLegacyInspectionSlots("", 0) {
		t.Fatal("O01 빈 마스터는 레거시")
	}
	if UseLegacyInspectionSlots("O03", 0) || UseLegacyInspectionSlots(OrgIDLibrary, 2) {
		t.Fatal("O03 또는 마스터 있으면 신 모드")
	}
}

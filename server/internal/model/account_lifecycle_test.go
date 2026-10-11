package model

import "testing"

func TestNormalizeUsername(t *testing.T) {
	if NormalizeUsername("  Lee Kim ") != "leekim" {
		t.Fatalf("got %q", NormalizeUsername("  Lee Kim "))
	}
	if NormalizeUsername("Admin") != "admin" {
		t.Fatalf("case")
	}
	if NormalizeUsername("Ａ") == "A" {
		// fullwidth A NFC stays different from ASCII A after lower; just must be stable
	}
	if RequestRoleOK(RoleOrgAdmin) || RequestRoleOK(RoleVisionAdmin) {
		t.Fatal("관리 등급 요청은 안 된다")
	}
	if !RequestRoleOK(RoleTech) {
		t.Fatal("기술은 된다")
	}
}

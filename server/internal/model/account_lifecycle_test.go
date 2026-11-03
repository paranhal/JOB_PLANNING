package model

import (
	"strings"
	"testing"
)

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

func TestUsernameConflictMessage(t *testing.T) {
	if (UsernameConflict{Kind: UsernameConflictLive}).Message() != "이미 사용 중인 아이디입니다" {
		t.Fatal("live")
	}
	if (UsernameConflict{Kind: UsernameConflictRenamed}).PublicMessage() != UsernameInquiryMsg {
		t.Fatal("renamed public")
	}
	if (UsernameConflict{Kind: UsernameConflictDeleted}).Message() != UsernameReservedMsg {
		t.Fatal("deleted")
	}
	if (UsernameConflict{Kind: UsernameConflictReservedOrphan}).PublicMessage() != UsernameInquiryMsg {
		t.Fatal("orphan public")
	}
	if !(UsernameConflict{}).Empty() || (UsernameConflict{}).Message() != "" {
		t.Fatal("empty")
	}
	vis := UsernameConflict{
		Kind: UsernameConflictRenamed, Username: "wsg", ReservedAt: "2026-10-27 12:45:10",
		OwnerName: "경영지원팀관리자", OwnerUsername: "fa_admin",
	}
	got := vis.VisionMessage()
	if !strings.Contains(got, "wsg") || !strings.Contains(got, "2026-10-27") || !strings.Contains(got, "fa_admin") {
		t.Fatalf("vision msg=%s", got)
	}
}

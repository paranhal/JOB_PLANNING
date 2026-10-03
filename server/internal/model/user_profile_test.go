package model

import "testing"

func TestProfileRequiredFieldsByRole(t *testing.T) {
	tech := User{Role: RoleTech}
	if !tech.NeedsProfileFill() || tech.ProfileFieldError() == "" {
		t.Fatal("기술은 핸드폰·조직이 필수여야 한다")
	}
	tech.Mobile = "010-1111-2222"
	tech.OrgID = OrgIDLibrary
	if tech.NeedsProfileFill() {
		t.Fatal("기술 필수 칸을 채웠는데도 막힌다")
	}

	admin := User{Role: RoleOrgAdmin, OrgID: OrgIDLibrary}
	if got := admin.ProfileFieldError(); got != "이메일을 입력하세요." {
		t.Fatalf("조직관리자 이메일: %q", got)
	}
	admin.Email = "a@b.c"
	if admin.NeedsProfileFill() {
		t.Fatal("조직관리자 이메일을 채웠는데도 막힌다")
	}

	vision := User{Role: RoleVisionAdmin, Email: "v@b.c"}
	if vision.NeedsProfileFill() {
		t.Fatal("비젼관리자는 조직 없이 통과해야 한다")
	}
}

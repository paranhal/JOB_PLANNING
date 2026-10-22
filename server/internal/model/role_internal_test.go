package model

import "testing"

func TestIsInternalStaffRoleExcludesPartner(t *testing.T) {
	for _, r := range []string{RoleVisionAdmin, RoleOrgAdmin, RoleCLevel, RoleSupport, RoleTech, RoleSales, RoleObserver, RoleTester} {
		if !IsInternalStaffRole(r) {
			t.Fatalf("%s 내부", r)
		}
	}
	if IsInternalStaffRole("partner") || IsInternalStaffRole("") {
		t.Fatal("partner·빈 값은 내부 직원이 아니다")
	}
}

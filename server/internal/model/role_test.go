package model

import "testing"

func TestNormalizeRoleKnownAliases(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{RoleAdmin, RoleAdmin},
		{"관리자", RoleAdmin},
		{RoleTech, RoleTech},
		{"기술", RoleTech},
		{"기술담당", RoleTech},
		{RoleSales, RoleSales},
		{"영업", RoleSales},
		{"영업담당", RoleSales},
		{RoleOffice, RoleOffice},
		{"행정", RoleOffice},
		{"receipt", RoleOffice},
		{"접수", RoleOffice},
		{"접수담당", RoleOffice},
		{"user", RoleOffice},
		{RoleObserver, RoleObserver},
		{"옵저버", RoleObserver},
		{"viewer", RoleObserver},
		{"열람", RoleObserver},
		{"열람사용자", RoleObserver},
		{" admin ", RoleAdmin},
	}
	for _, tc := range cases {
		if got := NormalizeRole(tc.in); got != tc.want {
			t.Fatalf("NormalizeRole(%q)=%q want %q", tc.in, got, tc.want)
		}
		if !IsKnownRole(tc.in) {
			t.Fatalf("IsKnownRole(%q)=false", tc.in)
		}
	}
}

func TestNormalizeRoleUnauthenticated(t *testing.T) {
	for _, in := range []string{"", "   ", "superuser", "root", "unknown"} {
		if got := NormalizeRole(in); got != "" {
			t.Fatalf("NormalizeRole(%q)=%q want empty", in, got)
		}
		if IsKnownRole(in) {
			t.Fatalf("IsKnownRole(%q)=true", in)
		}
	}
}

func TestRoleLabelUnauthenticated(t *testing.T) {
	if got := RoleLabel(""); got != "미인증" {
		t.Fatalf("RoleLabel(\"\")=%q", got)
	}
}

func TestDefaultPermissionsUnknownNil(t *testing.T) {
	if p := DefaultPermissions(""); p != nil {
		t.Fatalf("DefaultPermissions(\"\")=%v want nil", p)
	}
	if p := DefaultPermissions("unknown"); p != nil {
		t.Fatalf("DefaultPermissions(\"unknown\")=%v want nil", p)
	}
}

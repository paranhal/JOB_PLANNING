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

func TestSalesDefaultHasNoAS(t *testing.T) {
	p := DefaultPermissions(RoleSales)
	if HasPermission(p, PermASReceive) || HasPermission(p, PermASProcess) {
		t.Fatalf("영업 기본값에 AS가 있으면 안 된다: %v", p)
	}
	if !HasPermission(p, PermAnalysis) || !HasPermission(p, PermStats) {
		t.Fatalf("영업 기본값: %v", p)
	}
}

func TestSalesStoredReceiveOnly(t *testing.T) {
	p := EffectivePermissions(RoleSales, "analysis,stats,as_receive")
	if !HasPermission(p, PermASReceive) {
		t.Fatal("저장된 as_receive 가 없다")
	}
	if HasPermission(p, PermASProcess) {
		t.Fatal("접수만 줬는데 as_process 가 있다")
	}
}

func TestAllPermissionsListsReceiveAndProcessSeparately(t *testing.T) {
	var recv, proc bool
	for _, d := range AllPermissions {
		if d.Key == PermASReceive {
			recv = true
		}
		if d.Key == PermASProcess {
			proc = true
		}
	}
	if !recv || !proc {
		t.Fatal("사용자 관리 체크박스에 AS 접수·조치가 따로 있어야 한다")
	}
}

package model

import (
	"strings"
	"testing"
)

func TestNormalizeRoleKnownAliases(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{RoleAdmin, RoleOrgAdmin},
		{"관리자", RoleOrgAdmin},
		{RoleOrgAdmin, RoleOrgAdmin},
		{RoleVisionAdmin, RoleVisionAdmin},
		{"비젼관리자", RoleVisionAdmin},
		{RoleTech, RoleTech},
		{"기술", RoleTech},
		{"기술담당", RoleTech},
		{RoleSales, RoleSales},
		{"영업", RoleSales},
		{"영업담당", RoleSales},
		{RoleOffice, RoleSupport},
		{"행정", RoleSupport},
		{"receipt", RoleSupport},
		{"접수", RoleSupport},
		{"접수담당", RoleSupport},
		{"user", RoleSupport},
		{RoleSupport, RoleSupport},
		{RoleObserver, RoleObserver},
		{"옵저버", RoleObserver},
		{"viewer", RoleObserver},
		{"열람", RoleObserver},
		{"열람사용자", RoleObserver},
		{RoleTester, RoleTester},
		{"테스터", RoleTester},
		{" admin ", RoleOrgAdmin},
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
	if p := DefaultPermissions(RoleTester); p != nil {
		t.Fatalf("테스터 기본값 %v", p)
	}
}

func TestSalesDefaultHasASReceive(t *testing.T) {
	p := DefaultPermissions(RoleSales)
	if !HasPermission(p, PermASCreate) || !HasPermission(p, PermASProcess) {
		t.Fatalf("영업 기본값에 AS 접수·조치가 없다: %v", p)
	}
	if HasPermission(p, PermASDelete) {
		t.Fatalf("영업이 AS 삭제 전권: %v", p)
	}
	if !HasPermission(p, PermSalesView) || !HasPermission(p, PermStatsView) {
		t.Fatalf("영업 기본값: %v", p)
	}
}

func TestSalesStoredReceiveOnly(t *testing.T) {
	p := EffectivePermissions(RoleSales, "analysis,stats,as_receive")
	if !HasPermission(p, PermASCreate) {
		t.Fatal("저장된 as_receive 가 없다")
	}
	if HasPermission(p, PermASProcess) {
		t.Fatal("접수만 줬는데 as.process 가 있다")
	}
	if PermissionSourceLabel(RoleSales, "") != "직급 기본값" {
		t.Fatal("빈 값 출처")
	}
	got := PermissionSourceLabel(RoleSales, "stats.view")
	if got == "직급 기본값" || !strings.HasPrefix(got, "개별 지정(") {
		t.Fatalf("출처 %q", got)
	}
}

func TestCompactStoredPermissionsMatchesDefaultEmpty(t *testing.T) {
	def := DefaultPermissions(RoleSales)
	if CompactStoredPermissions(RoleSales, def) != "" {
		t.Fatal("기본값과 같으면 비어야 한다")
	}
	if CompactStoredPermissions(RoleSales, []string{PermASCreate}) == "" {
		t.Fatal("예외는 남겨야 한다")
	}
	if CompactStoredPermissions(RoleVisionAdmin, def) != "" {
		t.Fatal("비젼관리자는 저장하지 않는다")
	}
}

func TestPermissionGroupsCoverAllKeys(t *testing.T) {
	seen := map[string]bool{}
	n := 0
	for _, g := range PermissionGroups() {
		if g.Label == "" || len(g.Items) == 0 {
			t.Fatalf("%+v", g)
		}
		for _, d := range g.Items {
			if seen[d.Key] {
				t.Fatalf("중복 %s", d.Key)
			}
			seen[d.Key] = true
			n++
		}
	}
	if n != len(AllPermissions) {
		t.Fatalf("그룹 %d All %d", n, len(AllPermissions))
	}
}

func TestAllPermissionsListsReceiveAndProcessSeparately(t *testing.T) {
	var recv, proc bool
	for _, d := range AllPermissions {
		if d.Key == PermASCreate {
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

func TestCanonicalPermLegacyKeys(t *testing.T) {
	if CanonicalPerm(PermASReceive) != PermASCreate {
		t.Fatal("as_receive")
	}
	if CanonicalPerm(PermAnalysis) != PermSalesView {
		t.Fatal("analysis")
	}
}

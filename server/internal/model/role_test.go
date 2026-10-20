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
		{RoleCLevel, RoleCLevel},
		{"C-Level", RoleCLevel},
		{"c-level", RoleCLevel},
		{"경영진", RoleCLevel},
		{"대표", RoleCLevel},
		{"임원", RoleCLevel},
		{" admin ", RoleOrgAdmin},
	}
	known := map[string]bool{
		RoleAdmin: true, "관리자": true, RoleOrgAdmin: true, RoleVisionAdmin: true, "비젼관리자": true,
		RoleCLevel: true, "C-Level": true, "c-level": true, "경영진": true, "대표": true, "임원": true,
		RoleTech: true, "기술": true, "기술담당": true, RoleSales: true, "영업": true, "영업담당": true,
		RoleOffice: true, "행정": true, "receipt": true, "접수": true, "접수담당": true, "user": true,
		RoleSupport: true, " admin ": true,
	}
	for _, tc := range cases {
		if got := NormalizeRole(tc.in); got != tc.want {
			t.Fatalf("NormalizeRole(%q)=%q want %q", tc.in, got, tc.want)
		}
		if known[tc.in] && !IsKnownRole(tc.in) {
			t.Fatalf("IsKnownRole(%q)=false", tc.in)
		}
		if !known[tc.in] && IsKnownRole(tc.in) {
			t.Fatalf("IsKnownRole(%q)=true (깃발 옛 값)", tc.in)
		}
	}
	if CanSeeMargin(RoleObserver) {
		t.Fatal("observer 호환값은 마진을 보지 않는다")
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

func TestEffectiveRoleTesterUsesBase(t *testing.T) {
	if got := EffectiveRole(RoleTester, RoleSales); got != RoleSales {
		t.Fatalf("tester sales=%s", got)
	}
	if got := EffectiveRole(RoleTech, RoleSales); got != RoleTech {
		t.Fatalf("tech stays=%s", got)
	}
	u := User{Role: RoleSales, IsTest: true}
	if !HasPermission(u.PermList(), PermSalesCreate) {
		t.Fatal("영업 테스터가 사업 등록 권한이 없다")
	}
	if PermAccess(RoleTester, PermSalesCreate) != AccessNone {
		t.Fatal("테스터 자체 권한표가 생겼다")
	}
}

func TestCLevelAccessNeverAboveView(t *testing.T) {
	for _, d := range AllPermissions {
		got := PermAccess(RoleCLevel, d.Key)
		want := ClampCLevelAccess(d.Key, got)
		if got != want || got > AccessView {
			t.Fatalf("%s = %d", d.Key, got)
		}
		if IsAdminSectionPerm(d.Key) && got != AccessNone {
			t.Fatalf("관리 권한 %s = %d", d.Key, got)
		}
	}
	if ClampCLevelAccess(PermASCreate, AccessFull) != AccessView {
		t.Fatal("쓰기 권한은 조회로 내린다")
	}
	if ClampCLevelAccess(PermUsersEdit, AccessFull) != AccessNone {
		t.Fatal("관리 권한은 없음")
	}
}

func TestCLevelDefaultPermissionsViewOnly(t *testing.T) {
	p := DefaultPermissions(RoleCLevel)
	if !HasPermission(p, PermASView) || !HasPermission(p, PermSalesView) || !HasPermission(p, PermStatsView) {
		t.Fatalf("조회 권한 없음: %v", p)
	}
	if HasPermission(p, PermASCreate) || HasPermission(p, PermUsersEdit) || HasPermission(p, PermDataDelete) {
		t.Fatalf("쓰기·관리 권한이 있다: %v", p)
	}
	if AdminGrade(RoleCLevel) != "exec" || IsAdminGrade(RoleCLevel) {
		t.Fatal("C-Level 은 exec 등급이고 관리자가 아니다")
	}
	if RoleFromAdminGradeMust(t, "exec", "") != RoleCLevel {
		t.Fatal("exec 등급")
	}
}

func RoleFromAdminGradeMust(t *testing.T, grade, job string) string {
	t.Helper()
	r, err := RoleFromAdminGrade(grade, job)
	if err != nil {
		t.Fatal(err)
	}
	return r
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

func TestSelfLeaveOnAllRoles(t *testing.T) {
	for _, role := range []string{RoleVisionAdmin, RoleOrgAdmin, RoleCLevel, RoleSupport, RoleTech, RoleSales, RoleObserver} {
		if PermAccess(role, PermSelfLeave) < AccessOwn {
			t.Fatalf("%s self.leave", role)
		}
		if PermAccess(role, PermSelfApproval) < AccessOwn {
			t.Fatalf("%s self.approval", role)
		}
	}
	if PermAccess(RoleTester, PermSelfLeave) != AccessNone {
		t.Fatal("tester")
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

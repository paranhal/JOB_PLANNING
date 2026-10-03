package model

import "testing"

func TestRoleMatrixRules(t *testing.T) {
	workView := []string{
		PermASView, PermMntView, PermAdminView, PermSalesView, PermSalesActView, PermMasterView, PermStatsView,
	}
	adminSection := []string{
		PermOrgView, PermOrgCreate, PermUsersView, PermCodesView, PermDataView,
	}

	t.Run("1_모든직급이_업무화면을_본다", func(t *testing.T) {
		for _, role := range []string{RoleVisionAdmin, RoleOrgAdmin, RoleSupport, RoleTech, RoleSales, RoleObserver} {
			for _, key := range workView {
				if PermAccess(role, key) < AccessView {
					t.Fatalf("%s %s = %d", role, key, PermAccess(role, key))
				}
			}
		}
	})

	t.Run("2_옵저버는_쓰기가_없다", func(t *testing.T) {
		for _, d := range AllPermissions {
			a := PermAccess(RoleObserver, d.Key)
			if a > AccessView {
				t.Fatalf("옵저버 %s = %d", d.Key, a)
			}
			if a == AccessView && !hasSuffixView(d.Key) {
				t.Fatalf("옵저버 조회가 아닌 키 %s", d.Key)
			}
		}
	})

	t.Run("3_기술은_영업사업을_못_만든다", func(t *testing.T) {
		if PermAccess(RoleTech, PermSalesCreate) != AccessNone {
			t.Fatal(PermAccess(RoleTech, PermSalesCreate))
		}
	})

	t.Run("4_기술은_영업활동을_기록한다", func(t *testing.T) {
		if PermAccess(RoleTech, PermSalesActCreate) != AccessFull {
			t.Fatal(PermAccess(RoleTech, PermSalesActCreate))
		}
	})

	t.Run("5_영업의_AS수정은_자기건만", func(t *testing.T) {
		if PermAccess(RoleSales, PermASEdit) != AccessOwn {
			t.Fatal(PermAccess(RoleSales, PermASEdit))
		}
	})

	t.Run("6_영업은_AS접수_조치를_한다", func(t *testing.T) {
		if PermAccess(RoleSales, PermASCreate) != AccessFull || PermAccess(RoleSales, PermASProcess) != AccessFull {
			t.Fatal("영업 AS 접수·조치")
		}
	})

	t.Run("7_지원은_업무_전권", func(t *testing.T) {
		for _, key := range []string{
			PermASCreate, PermASEdit, PermASDelete, PermMntDelete, PermAdminCreate, PermSalesCreate, PermMasterEdit, PermStatsView,
		} {
			if PermAccess(RoleSupport, key) != AccessFull {
				t.Fatalf("지원 %s = %d", key, PermAccess(RoleSupport, key))
			}
		}
	})

	t.Run("8_기술_영업_지원은_관리섹션_불가", func(t *testing.T) {
		for _, role := range []string{RoleTech, RoleSales, RoleSupport} {
			for _, key := range adminSection {
				if PermAccess(role, key) != AccessNone {
					t.Fatalf("%s %s = %d", role, key, PermAccess(role, key))
				}
			}
		}
	})

	t.Run("9_삭제는_자기영역에서만_전권", func(t *testing.T) {
		if PermAccess(RoleTech, PermASDelete) != AccessFull || PermAccess(RoleTech, PermMntDelete) != AccessFull {
			t.Fatal("기술 AS·점검 삭제")
		}
		if PermAccess(RoleTech, PermAdminDelete) != AccessOwn {
			t.Fatal("기술 행정 삭제는 자기 건")
		}
		if PermAccess(RoleTech, PermSalesDelete) != AccessNone {
			t.Fatal("기술 영업 사업 삭제")
		}
		if PermAccess(RoleSales, PermSalesDelete) != AccessFull {
			t.Fatal("영업 사업 삭제")
		}
		if PermAccess(RoleSales, PermASDelete) != AccessNone {
			t.Fatal("영업 AS 삭제")
		}
		if PermAccess(RoleSupport, PermASDelete) != AccessFull || PermAccess(RoleSupport, PermSalesDelete) != AccessFull {
			t.Fatal("지원 삭제 전권")
		}
	})

	t.Run("10_기준정보_삭제는_지원이상", func(t *testing.T) {
		if PermAccess(RoleSupport, PermMasterDelete) != AccessFull {
			t.Fatal("지원")
		}
		if PermAccess(RoleOrgAdmin, PermMasterDelete) != AccessFull {
			t.Fatal("조직관리자")
		}
		if PermAccess(RoleTech, PermMasterDelete) != AccessNone || PermAccess(RoleSales, PermMasterDelete) != AccessNone {
			t.Fatal("기술·영업")
		}
	})
}

func hasSuffixView(key string) bool {
	return len(key) >= 5 && key[len(key)-5:] == ".view"
}

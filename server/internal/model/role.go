package model

import (
	"log"
	"strings"
)

const (
	RoleVisionAdmin = "vision_admin"
	RoleOrgAdmin    = "org_admin"
	RoleSupport     = "support"
	RoleTech        = "tech"
	RoleSales       = "sales"
	RoleObserver    = "observer"
	RoleTester      = "tester"

	// 옛 코드 이름. NormalizeRole 이 org_admin·support 로 옮긴다.
	RoleAdmin  = "admin"
	RoleOffice = "office"
)

const (
	PermASView       = "as.view"
	PermASCreate     = "as.create"
	PermASProcess    = "as.process"
	PermASEdit       = "as.edit"
	PermASDelete     = "as.delete"
	PermMntView      = "mnt.view"
	PermMntCreate    = "mnt.create"
	PermMntEdit      = "mnt.edit"
	PermMntProcess   = "mnt.process"
	PermMntDelete    = "mnt.delete"
	PermAdminView    = "admin.view"
	PermAdminCreate  = "admin.create"
	PermAdminEdit    = "admin.edit"
	PermAdminProcess = "admin.process"
	PermAdminDelete  = "admin.delete"
	PermSalesView    = "sales.view"
	PermSalesCreate  = "sales.create"
	PermSalesEdit    = "sales.edit"
	PermSalesDelete  = "sales.delete"
	PermSalesActView   = "sales_act.view"
	PermSalesActCreate = "sales_act.create"
	PermSalesActEdit   = "sales_act.edit"
	PermSalesActDelete = "sales_act.delete"
	PermMasterView   = "master.view"
	PermMasterCreate = "master.create"
	PermMasterEdit   = "master.edit"
	PermMasterDelete = "master.delete"
	PermStatsView    = "stats.view"
	PermOrgView      = "org.view"
	PermOrgCreate    = "org.create"
	PermOrgEdit      = "org.edit"
	PermOrgDelete    = "org.delete"
	PermUsersView    = "users.view"
	PermUsersCreate  = "users.create"
	PermUsersEdit    = "users.edit"
	PermUsersDelete  = "users.delete"
	PermCodesView    = "codes.view"
	PermCodesCreate  = "codes.create"
	PermCodesEdit    = "codes.edit"
	PermCodesDelete  = "codes.delete"
	PermDataView     = "data.view"
	PermDataCreate   = "data.create"
	PermDataEdit     = "data.edit"
	PermDataDelete   = "data.delete"

	// 옛 9키. hasPerm·화면 호환. 새 키로 읽는다.
	PermASReceive       = "as_receive"
	PermASProcessLegacy = "as_process"
	PermWorkboard       = "workboard"
	PermMaintenance     = "maintenance"
	PermMaintenanceEdit = "maintenance_edit"
	PermMasterWrite     = "master_write"
	PermCodesUsers      = "codes_users"
	PermAnalysis        = "analysis"
	PermStats           = "stats"
)

type Access int

const (
	AccessNone Access = iota
	AccessView
	AccessOwn
	AccessFull
)

type PermDef struct {
	Key   string
	Label string
}

var AllPermissions = []PermDef{
	{PermASView, "AS 조회"}, {PermASCreate, "AS 접수"}, {PermASProcess, "AS 조치"}, {PermASEdit, "AS 수정"}, {PermASDelete, "AS 삭제"},
	{PermMntView, "정기점검 조회"}, {PermMntCreate, "정기점검 등록"}, {PermMntEdit, "정기점검 수정"}, {PermMntProcess, "정기점검 조치"}, {PermMntDelete, "정기점검 삭제"},
	{PermAdminView, "행정 조회"}, {PermAdminCreate, "행정 등록"}, {PermAdminEdit, "행정 수정"}, {PermAdminProcess, "행정 조치"}, {PermAdminDelete, "행정 삭제"},
	{PermSalesView, "영업사업 조회"}, {PermSalesCreate, "영업사업 등록"}, {PermSalesEdit, "영업사업 수정"}, {PermSalesDelete, "영업사업 삭제"},
	{PermSalesActView, "영업활동 조회"}, {PermSalesActCreate, "영업활동 등록"}, {PermSalesActEdit, "영업활동 수정"}, {PermSalesActDelete, "영업활동 삭제"},
	{PermMasterView, "기준정보 조회"}, {PermMasterCreate, "기준정보 등록"}, {PermMasterEdit, "기준정보 수정"}, {PermMasterDelete, "기준정보 삭제"},
	{PermStatsView, "통계 조회"},
	{PermOrgView, "조직 조회"}, {PermOrgCreate, "조직 생성"}, {PermOrgEdit, "조직 수정"}, {PermOrgDelete, "조직 삭제"},
	{PermUsersView, "계정 조회"}, {PermUsersCreate, "계정 생성"}, {PermUsersEdit, "계정 수정"}, {PermUsersDelete, "계정 삭제"},
	{PermCodesView, "코드 조회"}, {PermCodesCreate, "코드 등록"}, {PermCodesEdit, "코드 수정"}, {PermCodesDelete, "코드 삭제"},
	{PermDataView, "데이터 조회"}, {PermDataCreate, "데이터 등록"}, {PermDataEdit, "데이터 수정"}, {PermDataDelete, "데이터 삭제"},
}

func CanonicalPerm(key string) string {
	switch strings.TrimSpace(key) {
	case PermASReceive:
		return PermASCreate
	case PermASProcessLegacy:
		return PermASProcess
	case PermWorkboard:
		return PermAdminCreate
	case PermMaintenance:
		return PermMntView
	case PermMaintenanceEdit:
		return PermMntEdit
	case PermMasterWrite:
		return PermMasterEdit
	case PermCodesUsers:
		return PermCodesEdit
	case PermAnalysis:
		return PermSalesView
	case PermStats:
		return PermStatsView
	default:
		return strings.TrimSpace(key)
	}
}

func NormalizeRole(role string) string {
	switch strings.TrimSpace(role) {
	case RoleVisionAdmin, "비젼관리자", "비전관리자":
		return RoleVisionAdmin
	case RoleOrgAdmin, RoleAdmin, "관리자":
		return RoleOrgAdmin
	case RoleSupport, RoleOffice, "행정", "receipt", "접수", "접수담당", "user", "지원":
		return RoleSupport
	case RoleTech, "기술", "기술담당":
		return RoleTech
	case RoleSales, "영업", "영업담당":
		return RoleSales
	case RoleObserver, "옵저버", "viewer", "열람", "열람사용자":
		return RoleObserver
	case RoleTester, "테스터":
		return RoleTester
	default:
		if trimmed := strings.TrimSpace(role); trimmed != "" {
			log.Printf("[auth] 알 수 없는 역할 %q", role)
		}
		return ""
	}
}

func IsKnownRole(role string) bool {
	switch NormalizeRole(role) {
	case RoleVisionAdmin, RoleOrgAdmin, RoleSupport, RoleTech, RoleSales, RoleObserver, RoleTester:
		return true
	default:
		return false
	}
}

func IsAdminGrade(role string) bool {
	r := NormalizeRole(role)
	return r == RoleVisionAdmin || r == RoleOrgAdmin
}

func RoleLabel(role string) string {
	switch NormalizeRole(role) {
	case RoleVisionAdmin:
		return "비젼관리자"
	case RoleOrgAdmin:
		return "조직관리자"
	case RoleSupport:
		return "지원"
	case RoleTech:
		return "기술"
	case RoleSales:
		return "영업"
	case RoleObserver:
		return "옵저버"
	case RoleTester:
		return "테스터"
	default:
		if strings.TrimSpace(role) == "" {
			return "미인증"
		}
		return role
	}
}

func ParsePermissions(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, p := range parts {
		p = CanonicalPerm(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

func FormatPermissions(perms []string) string {
	seen := map[string]bool{}
	var out []string
	for _, p := range perms {
		p = CanonicalPerm(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return strings.Join(out, ",")
}

func HasPermission(perms []string, key string) bool {
	key = CanonicalPerm(key)
	for _, p := range perms {
		if CanonicalPerm(p) == key {
			return true
		}
	}
	return false
}

func EffectivePermissions(role, stored string) []string {
	role = NormalizeRole(role)
	if role == RoleVisionAdmin {
		return DefaultPermissions(RoleVisionAdmin)
	}
	if p := ParsePermissions(stored); len(p) > 0 {
		return p
	}
	return DefaultPermissions(role)
}

func PermissionSourceLabel(role, stored string) string {
	role = NormalizeRole(role)
	if role == RoleVisionAdmin {
		return "직급 기본값"
	}
	if len(ParsePermissions(stored)) > 0 {
		return "개별 지정"
	}
	return "직급 기본값"
}

func ObserverViewPermissions() []string {
	return DefaultPermissions(RoleObserver)
}

func permAtLeast(a Access, need Access) bool {
	return a >= need
}

func HasAccess(role, key string, need Access) bool {
	return permAtLeast(PermAccess(role, key), need)
}

func PermAccess(role, key string) Access {
	role = NormalizeRole(role)
	key = CanonicalPerm(key)
	if role == RoleTester {
		return AccessNone
	}
	if role == RoleVisionAdmin {
		if key == "" {
			return AccessNone
		}
		return AccessFull
	}
	return defaultAccess(role, key)
}

func DefaultPermissions(role string) []string {
	role = NormalizeRole(role)
	if role == "" || role == RoleTester {
		return nil
	}
	var out []string
	for _, d := range AllPermissions {
		if PermAccess(role, d.Key) >= AccessOwn {
			out = append(out, d.Key)
		} else if PermAccess(role, d.Key) == AccessView {
			out = append(out, d.Key)
		}
	}
	return out
}

func defaultAccess(role, key string) Access {
	switch role {
	case RoleOrgAdmin:
		switch {
		case strings.HasPrefix(key, "org."):
			if key == PermOrgView {
				return AccessView
			}
			return AccessNone
		default:
			return AccessFull
		}
	case RoleSupport:
		if isAdminSectionPerm(key) {
			return AccessNone
		}
		return AccessFull
	case RoleTech:
		return techAccess(key)
	case RoleSales:
		return salesAccess(key)
	case RoleObserver:
		if strings.HasSuffix(key, ".view") {
			return AccessView
		}
		return AccessNone
	default:
		return AccessNone
	}
}

func isAdminSectionPerm(key string) bool {
	return strings.HasPrefix(key, "org.") || strings.HasPrefix(key, "users.") ||
		strings.HasPrefix(key, "codes.") || strings.HasPrefix(key, "data.")
}

func techAccess(key string) Access {
	switch key {
	case PermASView, PermASCreate, PermASProcess, PermASEdit, PermASDelete,
		PermMntView, PermMntCreate, PermMntEdit, PermMntProcess, PermMntDelete,
		PermAdminView, PermAdminCreate, PermAdminProcess, PermAdminEdit,
		PermSalesView, PermSalesActView, PermSalesActCreate, PermSalesActEdit,
		PermMasterView, PermMasterCreate, PermMasterEdit, PermStatsView:
		return AccessFull
	case PermAdminDelete, PermSalesActDelete:
		return AccessOwn
	default:
		return AccessNone
	}
}

func salesAccess(key string) Access {
	switch key {
	case PermASView, PermASCreate, PermASProcess,
		PermMntView, PermAdminView, PermAdminCreate, PermAdminProcess, PermAdminEdit,
		PermSalesView, PermSalesCreate, PermSalesEdit, PermSalesDelete,
		PermSalesActView, PermSalesActCreate, PermSalesActEdit, PermSalesActDelete,
		PermMasterView, PermMasterCreate, PermMasterEdit, PermStatsView:
		return AccessFull
	case PermASEdit, PermAdminDelete:
		return AccessOwn
	default:
		return AccessNone
	}
}

func LegacyDefaultPermissions(role string) []string {
	switch strings.TrimSpace(role) {
	case RoleAdmin, "관리자":
		return []string{PermASReceive, PermASProcessLegacy, PermWorkboard, PermMaintenance, PermMaintenanceEdit, PermMasterWrite, PermCodesUsers, PermAnalysis, PermStats}
	case RoleTech, "기술", "기술담당":
		return []string{PermASReceive, PermASProcessLegacy, PermWorkboard, PermMaintenance, PermMaintenanceEdit, PermStats}
	case RoleSales, "영업", "영업담당":
		return []string{PermAnalysis, PermStats}
	case RoleOffice, "행정", "receipt", "접수", "접수담당", "user":
		return []string{PermASReceive, PermWorkboard, PermStats}
	case RoleObserver, "옵저버", "viewer", "열람", "열람사용자":
		return []string{PermWorkboard, PermMaintenance, PermAnalysis, PermStats}
	default:
		return nil
	}
}

func SamePermSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	mb := map[string]bool{}
	for _, x := range b {
		mb[x] = true
	}
	for _, x := range a {
		if !mb[x] {
			return false
		}
	}
	return true
}

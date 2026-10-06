package model

import (
	"fmt"
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
	PermASView         = "as.view"
	PermASCreate       = "as.create"
	PermASProcess      = "as.process"
	PermASEdit         = "as.edit"
	PermASDelete       = "as.delete"
	PermMntView        = "mnt.view"
	PermMntCreate      = "mnt.create"
	PermMntEdit        = "mnt.edit"
	PermMntProcess     = "mnt.process"
	PermMntDelete      = "mnt.delete"
	PermAdminView      = "admin.view"
	PermAdminCreate    = "admin.create"
	PermAdminEdit      = "admin.edit"
	PermAdminProcess   = "admin.process"
	PermAdminDelete    = "admin.delete"
	PermSalesView      = "sales.view"
	PermSalesCreate    = "sales.create"
	PermSalesEdit      = "sales.edit"
	PermSalesDelete    = "sales.delete"
	PermSalesActView   = "sales_act.view"
	PermSalesActCreate = "sales_act.create"
	PermSalesActEdit   = "sales_act.edit"
	PermSalesActDelete = "sales_act.delete"
	PermMasterView     = "master.view"
	PermMasterCreate   = "master.create"
	PermMasterEdit     = "master.edit"
	PermMasterDelete   = "master.delete"
	PermStatsView      = "stats.view"
	PermOrgView        = "org.view"
	PermOrgCreate      = "org.create"
	PermOrgEdit        = "org.edit"
	PermOrgDelete      = "org.delete"
	PermUsersView      = "users.view"
	PermUsersCreate    = "users.create"
	PermUsersEdit      = "users.edit"
	PermUsersDelete    = "users.delete"
	PermCodesView      = "codes.view"
	PermCodesCreate    = "codes.create"
	PermCodesEdit      = "codes.edit"
	PermCodesDelete    = "codes.delete"
	PermDataView       = "data.view"
	PermDataCreate     = "data.create"
	PermDataEdit       = "data.edit"
	PermDataDelete     = "data.delete"

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
	case RoleVisionAdmin, RoleOrgAdmin, RoleSupport, RoleTech, RoleSales:
		return true
	default:
		return false
	}
}

func IsAdminGrade(role string) bool {
	r := NormalizeRole(role)
	return r == RoleVisionAdmin || r == RoleOrgAdmin
}

// AdminGrade vision | org | none  (§66.3)
func AdminGrade(role string) string {
	switch NormalizeRole(role) {
	case RoleVisionAdmin:
		return "vision"
	case RoleOrgAdmin:
		return "org"
	default:
		return "none"
	}
}

// JobRole 일반일 때만 업무. 관리자면 "".
func JobRole(role string) string {
	switch r := NormalizeRole(role); r {
	case RoleSales, RoleTech, RoleSupport:
		return r
	default:
		return ""
	}
}

// RoleFromAdminGrade 화면의 등급+업무를 role 한 칸으로 접는다 (§67.6).
func RoleFromAdminGrade(grade, job string) (string, error) {
	switch strings.TrimSpace(grade) {
	case "vision", RoleVisionAdmin:
		return RoleVisionAdmin, nil
	case "org", RoleOrgAdmin:
		return RoleOrgAdmin, nil
	case "none", "user":
		j := NormalizeRole(job)
		if j != RoleSales && j != RoleTech && j != RoleSupport {
			return "", fmt.Errorf("일반은 업무를 하나 고르세요.")
		}
		return j, nil
	default:
		if r := NormalizeRole(grade); IsKnownRole(r) {
			return r, nil
		}
		if r := NormalizeRole(job); r == RoleSales || r == RoleTech || r == RoleSupport {
			return r, nil
		}
		return "", fmt.Errorf("관리 등급을 고르세요.")
	}
}

// EffectiveRole 테스터는 base_role 권한표를 쓴다. 호출은 걷어냈다. 함수는 남긴다 (§66).
func EffectiveRole(role, baseRole string) string {
	role = NormalizeRole(role)
	if role != RoleTester {
		return role
	}
	b := NormalizeRole(baseRole)
	if b == "" || b == RoleTester {
		return RoleTester
	}
	return b
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

type PermGroup struct {
	Label string
	Items []PermDef
}

func PermissionGroups() []PermGroup {
	type g = PermGroup
	order := []struct {
		prefix, label string
	}{
		{"sales_act.", "영업활동"},
		{"as.", "AS"},
		{"mnt.", "정기점검"},
		{"admin.", "행정"},
		{"sales.", "영업사업"},
		{"master.", "기준정보"},
		{"stats.", "통계"},
		{"org.", "조직"},
		{"users.", "계정"},
		{"codes.", "코드"},
		{"data.", "데이터"},
	}
	idx := make([]g, len(order))
	for i, o := range order {
		idx[i].Label = o.label
	}
	for _, d := range AllPermissions {
		for i, o := range order {
			if strings.HasPrefix(d.Key, o.prefix) {
				idx[i].Items = append(idx[i].Items, d)
				break
			}
		}
	}
	out := make([]g, 0, len(idx))
	for _, x := range idx {
		if len(x.Items) > 0 {
			out = append(out, x)
		}
	}
	return out
}

func CompactStoredPermissions(role string, selected []string) string {
	role = NormalizeRole(role)
	if role == RoleVisionAdmin {
		return ""
	}
	got := ParsePermissions(FormatPermissions(selected))
	if SamePermSet(got, DefaultPermissions(role)) {
		return ""
	}
	return FormatPermissions(got)
}

func PermissionDiffCounts(role, stored string) (add, del int) {
	role = NormalizeRole(role)
	got := ParsePermissions(stored)
	if len(got) == 0 {
		return 0, 0
	}
	def := DefaultPermissions(role)
	dm := map[string]bool{}
	for _, k := range def {
		dm[CanonicalPerm(k)] = true
	}
	gm := map[string]bool{}
	for _, k := range got {
		k = CanonicalPerm(k)
		gm[k] = true
		if !dm[k] {
			add++
		}
	}
	for _, k := range def {
		if !gm[CanonicalPerm(k)] {
			del++
		}
	}
	return add, del
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
	if len(ParsePermissions(stored)) == 0 {
		return "직급 기본값"
	}
	add, del := PermissionDiffCounts(role, stored)
	if add == 0 && del == 0 {
		return "직급 기본값"
	}
	return fmt.Sprintf("개별 지정(+%d −%d)", add, del)
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

var accessOverrides map[string]map[string]Access

func SetAccessOverrides(m map[string]map[string]Access) {
	accessOverrides = m
}

func AccessLabel(a Access) string {
	switch a {
	case AccessView:
		return "조회"
	case AccessOwn:
		return "본인 것만"
	case AccessFull:
		return "전부"
	default:
		return "없음"
	}
}

func BuiltinAccess(role, key string) Access {
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

func PermAccess(role, key string) Access {
	role = NormalizeRole(role)
	key = CanonicalPerm(key)
	if role == RoleTester {
		return AccessNone
	}
	if m, ok := accessOverrides[role]; ok {
		if a, ok := m[key]; ok {
			return a
		}
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

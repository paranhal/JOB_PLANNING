package model

import "strings"

// 등급(역할)
const (
	RoleAdmin    = "admin"    // 관리자
	RoleTech     = "tech"     // 기술
	RoleSales    = "sales"    // 영업
	RoleOffice   = "office"   // 행정
	RoleObserver = "observer" // 옵저버
)

// 권한 키 (체크박스)
const (
	PermASReceive        = "as_receive"        // AS 접수
	PermASProcess        = "as_process"        // AS 조치
	PermWorkboard        = "workboard"         // 일일업무
	PermMaintenance      = "maintenance"       // 정기점검 조회
	PermMaintenanceEdit  = "maintenance_edit"  // 정기점검 수정
	PermMasterWrite      = "master_write"      // 기준정보 쓰기
	PermCodesUsers       = "codes_users"       // 코드·사용자 관리
	PermAnalysis         = "analysis"          // 분석/영업
	PermStats            = "stats"             // 통계
)

// AllPermissions 사용자 관리 화면 체크박스 순서
var AllPermissions = []struct {
	Key   string
	Label string
}{
	{PermASReceive, "AS 접수"},
	{PermASProcess, "AS 조치"},
	{PermWorkboard, "일일업무"},
	{PermMaintenance, "정기점검 조회"},
	{PermMaintenanceEdit, "정기점검 수정"},
	{PermMasterWrite, "기준정보 쓰기"},
	{PermCodesUsers, "코드·사용자 관리"},
	{PermAnalysis, "분석/영업"},
	{PermStats, "통계"},
}

// NormalizeRole 레거시 역할 → 신규 등급
func NormalizeRole(role string) string {
	switch strings.TrimSpace(role) {
	case RoleAdmin, "관리자":
		return RoleAdmin
	case RoleTech, "기술", "기술담당":
		return RoleTech
	case RoleSales, "영업", "영업담당":
		return RoleSales
	case RoleOffice, "행정", "receipt", "접수", "접수담당", "user":
		return RoleOffice
	case RoleObserver, "옵저버", "viewer", "열람", "열람사용자":
		return RoleObserver
	default:
		if role == "" {
			return RoleObserver
		}
		return role
	}
}

// RoleLabel 화면 표시명
func RoleLabel(role string) string {
	switch NormalizeRole(role) {
	case RoleAdmin:
		return "관리자"
	case RoleTech:
		return "기술"
	case RoleSales:
		return "영업"
	case RoleOffice:
		return "행정"
	case RoleObserver:
		return "옵저버"
	default:
		return role
	}
}

// DefaultPermissions 등급별 기본 권한 (체크박스 초기값)
func DefaultPermissions(role string) []string {
	switch NormalizeRole(role) {
	case RoleAdmin:
		return []string{
			PermASReceive, PermASProcess, PermWorkboard,
			PermMaintenance, PermMaintenanceEdit, PermMasterWrite,
			PermCodesUsers, PermAnalysis, PermStats,
		}
	case RoleTech:
		return []string{
			PermASReceive, PermASProcess, PermWorkboard,
			PermMaintenance, PermMaintenanceEdit, PermStats,
		}
	case RoleSales:
		return []string{PermAnalysis, PermStats}
	case RoleOffice:
		return []string{PermASReceive, PermWorkboard, PermStats}
	case RoleObserver:
		// 전체 메뉴 조회용(쓰기는 RequireActiveRole에서 차단)
		return []string{
			PermWorkboard, PermMaintenance, PermAnalysis, PermStats,
		}
	default:
		return nil
	}
}

// ObserverViewPermissions 옵저버 기본 조회 권한(마이그레이션·시드용)
func ObserverViewPermissions() []string {
	return DefaultPermissions(RoleObserver)
}

// ParsePermissions CSV → 슬라이스
func ParsePermissions(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

// FormatPermissions 슬라이스 → CSV
func FormatPermissions(perms []string) string {
	seen := map[string]bool{}
	var out []string
	for _, p := range perms {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return strings.Join(out, ",")
}

// HasPermission 권한 보유 여부
func HasPermission(perms []string, key string) bool {
	for _, p := range perms {
		if p == key {
			return true
		}
	}
	return false
}

// EffectivePermissions 저장된 권한이 비어 있으면 등급 기본값
func EffectivePermissions(role, stored string) []string {
	role = NormalizeRole(role)
	if role == RoleAdmin {
		return DefaultPermissions(RoleAdmin)
	}
	if p := ParsePermissions(stored); len(p) > 0 {
		return p
	}
	return DefaultPermissions(role)
}

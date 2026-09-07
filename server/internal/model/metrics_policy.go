package model

import "strings"

// MetricsPolicy app_settings 에서 읽은 지표 하한·실행률 대상 (§4.5).
type MetricsPolicy struct {
	BaseDate      string // YYYY-MM-DD. 비면 하한 없음
	ProgressScope string // 예: as,maintenance
}

// ProgressScopeRevertHint 실행률에서 행정·지원을 빼 둔 이유와 되돌릴 조건 (§4.5.4).
const ProgressScopeRevertHint = "행정·지원의 예정일 입력률이 4주 연속 90% 이상이면 실행률 대상에 다시 넣는다 (progress_scope 에 admin 추가)."

// ProgressScopeIncludes 실행률 분모·분자에 이 업무 유형을 넣을지.
// scope 가 비어 있으면 전 유형(설정을 지워 되돌린 상태).
func ProgressScopeIncludes(scope, kind string) bool {
	kind = strings.ToLower(strings.TrimSpace(kind))
	scope = strings.ToLower(strings.TrimSpace(scope))
	if scope == "" {
		return true
	}
	if kind == "support" {
		kind = "admin"
	}
	if kind == "mnt" {
		kind = "maintenance"
	}
	for _, p := range strings.Split(scope, ",") {
		p = strings.TrimSpace(p)
		if p == "support" {
			p = "admin"
		}
		if p == "mnt" {
			p = "maintenance"
		}
		if p == kind {
			return true
		}
	}
	return false
}

// ProgressScopeLabel 실행률 카드 안내. 예: 「AS · 정기점검 기준」
func ProgressScopeLabel(scope string) string {
	scope = strings.ToLower(strings.TrimSpace(scope))
	if scope == "" {
		return ""
	}
	var parts []string
	if ProgressScopeIncludes(scope, "as") {
		parts = append(parts, "AS")
	}
	if ProgressScopeIncludes(scope, "maintenance") {
		parts = append(parts, "정기점검")
	}
	if ProgressScopeIncludes(scope, "admin") {
		parts = append(parts, "행정·지원")
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " · ") + " 기준"
}

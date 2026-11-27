package model

import (
	"strings"
	"sync"
)

// WeeklyActivityKind 업무 구분(사업 외 업무) 한 줄. 사업에 연결되지 않은 업무를 묶는 블록이다. (95-197 → 95-224)
// 업무(work_tasks.activity_kind)에 이 키를 저장하면 주간보고서의 같은 블록에 모인다.
// 구분 목록은 코드 관리(codes, code_group='work_category')가 원본이다. 코드가 없으면 아래 기본값을 쓴다.
type WeeklyActivityKind struct {
	Key, Label string
	Inactive   bool // 비활성: 신규 선택 불가, 이미 붙은 업무의 표시는 유지
}

// WorkCategoryCodeGroup 업무 구분을 담는 codes 그룹.
const WorkCategoryCodeGroup = "work_category"

// defaultWeeklyActivityKinds 표시 순서(95-224 확정). 키는 기존 activity_kind 값과 같다.
var defaultWeeklyActivityKinds = []WeeklyActivityKind{
	{"mgmt", "경영지원", false},
	{"asset", "자산관리", false},
	{"tech", "영업", false},
	{"strategy", "전략및혁신", false},
	{"service", "사업 부문", false},
	{"biz_support", "업무지원", false},
	{"growth", "역량강화", false},
}

var (
	activityMu    sync.RWMutex
	activityKinds = append([]WeeklyActivityKind(nil), defaultWeeklyActivityKinds...)
)

// DefaultWeeklyActivityKinds 코드 시드용 기본 7종(복사본).
func DefaultWeeklyActivityKinds() []WeeklyActivityKind {
	return append([]WeeklyActivityKind(nil), defaultWeeklyActivityKinds...)
}

// SetWeeklyActivityKinds 코드 관리에서 읽은 목록으로 바꾼다. 빈 목록이면 기본값으로 되돌린다.
func SetWeeklyActivityKinds(list []WeeklyActivityKind) {
	activityMu.Lock()
	defer activityMu.Unlock()
	if len(list) == 0 {
		activityKinds = append([]WeeklyActivityKind(nil), defaultWeeklyActivityKinds...)
		return
	}
	activityKinds = append([]WeeklyActivityKind(nil), list...)
}

// WeeklyActivityKindList 비활성을 포함한 전체(표시·집계용, 정렬 순서).
func WeeklyActivityKindList() []WeeklyActivityKind {
	activityMu.RLock()
	defer activityMu.RUnlock()
	return append([]WeeklyActivityKind(nil), activityKinds...)
}

// SelectableWeeklyActivityKinds 신규 선택 가능한(활성) 구분만.
func SelectableWeeklyActivityKinds() []WeeklyActivityKind {
	var out []WeeklyActivityKind
	for _, k := range WeeklyActivityKindList() {
		if !k.Inactive {
			out = append(out, k)
		}
	}
	return out
}

// IsWeeklyActivityKind 아는 구분 키인지(비활성 포함).
func IsWeeklyActivityKind(key string) bool {
	key = strings.TrimSpace(key)
	for _, k := range WeeklyActivityKindList() {
		if k.Key == key {
			return true
		}
	}
	return false
}

// IsWeeklyActivitySelectable 신규로 고를 수 있는 구분 키인지(활성만).
func IsWeeklyActivitySelectable(key string) bool {
	key = strings.TrimSpace(key)
	for _, k := range WeeklyActivityKindList() {
		if k.Key == key {
			return !k.Inactive
		}
	}
	return false
}

// WeeklyActivityLabel 분류 키의 표시 이름(없으면 빈 문자열).
func WeeklyActivityLabel(key string) string {
	key = strings.TrimSpace(key)
	for _, k := range WeeklyActivityKindList() {
		if k.Key == key {
			return k.Label
		}
	}
	return ""
}

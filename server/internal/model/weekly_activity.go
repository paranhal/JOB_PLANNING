package model

import "strings"

// WeeklyActivityKind 주간업무보고의 활동 분류. 사업에 연결되지 않은 업무를 묶는 블록이다. (95-197)
// 업무(work_tasks.activity_kind)에 이 키를 저장하면 보고서의 같은 활동 블록에 모인다.
type WeeklyActivityKind struct {
	Key, Label string
}

// WeeklyActivityKinds 화면 표시 순서. tech·asset 은 기존 활동 행 키와 같다.
var WeeklyActivityKinds = []WeeklyActivityKind{
	{"mgmt", "경영지원"},
	{"tech", "기술영업"},
	{"strategy", "전략및혁신"},
	{"asset", "자산관리"},
	{"biz_support", "업무지원"},
	{"service", "고객서비스"},
	{"growth", "역량강화"},
}

// IsWeeklyActivityKind 7종 분류 키인지.
func IsWeeklyActivityKind(key string) bool {
	key = strings.TrimSpace(key)
	for _, k := range WeeklyActivityKinds {
		if k.Key == key {
			return true
		}
	}
	return false
}

// WeeklyActivityLabel 분류 키의 표시 이름(없으면 빈 문자열).
func WeeklyActivityLabel(key string) string {
	key = strings.TrimSpace(key)
	for _, k := range WeeklyActivityKinds {
		if k.Key == key {
			return k.Label
		}
	}
	return ""
}

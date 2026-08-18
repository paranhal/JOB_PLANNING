package model

import (
	"fmt"
	"strings"
)

const (
	HolidayKindPublic     = "public"
	HolidayKindSubstitute = "substitute"
	HolidayKindCompany    = "company"

	HolidaySourceAPI    = "api"
	HolidaySourceManual = "manual"
)

// Holiday 공휴일·휴무일 1건. holidays 테이블 (§23.13 · 부록 B.2).
type Holiday struct {
	Date      string // YYYY-MM-DD, PK
	Year      int
	Name      string
	Kind      string // public | substitute | company
	Source    string // api | manual
	SyncedAt  string
	CreatedAt string
}

func ValidHolidayKind(kind string) bool {
	switch strings.TrimSpace(kind) {
	case HolidayKindPublic, HolidayKindSubstitute, HolidayKindCompany:
		return true
	default:
		return false
	}
}

func ValidHolidaySource(source string) bool {
	switch strings.TrimSpace(source) {
	case HolidaySourceAPI, HolidaySourceManual:
		return true
	default:
		return false
	}
}

func HolidayKindLabel(kind string) string {
	switch strings.TrimSpace(kind) {
	case HolidayKindPublic:
		return "공휴일"
	case HolidayKindSubstitute:
		return "대체공휴일"
	case HolidayKindCompany:
		return "회사 휴무일"
	default:
		return kind
	}
}

// HolidayMissingBanner 해당 연도 행이 없을 때 화면 배너. §23.13
func HolidayMissingBanner(year int) string {
	if year <= 0 {
		return ""
	}
	return fmt.Sprintf("%d년 휴무일 미등록 — 주말만 제외됩니다", year)
}

// HolidaySyncFailBanner API 동기화 실패. 테이블은 비우지 않는다. §23.13.2
func HolidaySyncFailBanner(year int, lastSync string) string {
	lastSync = strings.TrimSpace(lastSync)
	if lastSync == "" {
		lastSync = "없음"
	} else if len(lastSync) >= 10 {
		lastSync = lastSync[:10]
	}
	return fmt.Sprintf("%d 공휴일 동기화 실패 — 마지막 동기화 %s", year, lastSync)
}

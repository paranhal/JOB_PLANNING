package model

import (
	"fmt"
	"strings"
)

// FormatASWorkTitle AS 업무명 — "[AS]고객명_접수번호".
func FormatASWorkTitle(orgName, asNumber string) string {
	name := strings.TrimSpace(orgName)
	if name == "" {
		name = "고객"
	}
	num := strings.TrimSpace(asNumber)
	if num == "" {
		return "[AS]" + name
	}
	return "[AS]" + name + "_" + num
}

// FormatSalesWorkTitle 영업 활동 일정표 제목 — "[영업]제목". 이미 접두어가 있으면 그대로 둔다.
func FormatSalesWorkTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return "[영업]"
	}
	if strings.HasPrefix(title, "[영업]") {
		return title
	}
	return "[영업]" + title
}

// FormatMaintenanceWorkTitle 정기점검 업무명 — "[점검]고객명_정기점검번호".
func FormatMaintenanceWorkTitle(orgName, visitNumber string) string {
	name := strings.TrimSpace(orgName)
	if name == "" {
		name = "사이트"
	}
	num := strings.TrimSpace(visitNumber)
	if num == "" {
		return "[점검]" + name
	}
	return "[점검]" + name + "_" + num
}

// FormatMaintenanceVisitNumber 정기점검 번호 — "방문일 · 점검대상".
func FormatMaintenanceVisitNumber(visitDate, productType string) string {
	num := strings.TrimSpace(visitDate)
	p := strings.TrimSpace(productType)
	if p == "" {
		return num
	}
	if num == "" {
		return p
	}
	return num + " · " + p
}

// FormatMaintenanceDescription 정기점검 설명 — "[제품]고객명 정기점검".
func FormatMaintenanceDescription(productType, orgName string) string {
	p := strings.TrimSpace(productType)
	if p == "" {
		p = "점검"
	}
	name := strings.TrimSpace(orgName)
	if name == "" {
		name = "사이트"
	}
	return "[" + p + "]" + name + " 정기점검"
}

// NormalizeDurationMin 소요 예상 시간(분). 비어 있거나 잘못되면 30분. 1분 단위(화면 칸은 15분 유지).
func NormalizeDurationMin(min int) int {
	if min <= 0 {
		return 30
	}
	if min > 13*60 {
		min = 13 * 60 // 07:00~20:00
	}
	return min
}

// DurationFromTimes 시작·종료시각으로 분 단위 소요시간을 구한다.
func DurationFromTimes(start, end string) int {
	sm := ParseHHMMMinutes(start)
	em := ParseHHMMMinutes(end)
	if sm < 0 || em < 0 || em <= sm {
		return 30
	}
	return NormalizeDurationMin(em - sm)
}

// ParseHHMMMinutes HH:MM → 자정 기준 분. 실패 시 -1.
func ParseHHMMMinutes(hhmm string) int {
	var hh, mm int
	if _, err := fmt.Sscanf(strings.TrimSpace(hhmm), "%d:%d", &hh, &mm); err != nil {
		return -1
	}
	if hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return -1
	}
	return hh*60 + mm
}

// FormatHHMMMinutes 분 → HH:MM
func FormatHHMMMinutes(total int) string {
	if total < 0 {
		total = 0
	}
	return fmt.Sprintf("%02d:%02d", total/60, total%60)
}

package model

import (
	"fmt"
	"strings"
)

const (
	MonthTargetSourceSite   = "site"
	MonthTargetSourceRegion = "region"
	MonthTargetSourceBulk   = "bulk"
	MonthTargetGenericSlot  = "generic"
)

// MaintenanceMonthTarget 기간 주기 사이트를 특정 달에 점검 대상으로 올린 기록. 95-45.
type MaintenanceMonthTarget struct {
	TargetID        string
	PlanID          string
	OrgID           string
	CustomerID      string
	SlotKey         string
	Year            int
	Month           int
	RoundNo         int
	Source          string
	Region          string
	CreatedByUserID string
	CreatedByName   string
	CreatedAt       string
	CancelledAt     string
}

func (t MaintenanceMonthTarget) Active() bool {
	return strings.TrimSpace(t.CancelledAt) == ""
}

func NormalizeMonthTargetSlot(slot string) string {
	slot = strings.TrimSpace(slot)
	if slot == "" {
		return MonthTargetGenericSlot
	}
	return slot
}

func NormalizeMonthTargetSource(src string) string {
	switch strings.TrimSpace(src) {
	case MonthTargetSourceRegion, MonthTargetSourceBulk:
		return strings.TrimSpace(src)
	default:
		return MonthTargetSourceSite
	}
}

func ValidateMonthTargetMonth(year, month int) error {
	if year < 1 || month < 1 || month > 12 {
		return fmt.Errorf("점검 월을 확인하세요")
	}
	return nil
}

func PeriodMonthTargetAllowed(cycle string, month int) error {
	if month < 1 || month > 12 {
		return fmt.Errorf("점검 월을 확인하세요")
	}
	if !IsPeriodCycle(cycle) {
		return fmt.Errorf("월·격월 사이트는 달 등록을 쓰지 않습니다")
	}
	if !MonthInCyclePeriod(cycle, month) {
		return fmt.Errorf("이 주기의 기간 밖 달입니다")
	}
	return nil
}

type MonthTargetSkip struct {
	CustomerID string
	Name       string
	Reason     string
	Month      int
}

type MonthTargetRegisterResult struct {
	Added   int
	Skipped []MonthTargetSkip
}

package repository

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"customer-support/internal/model"
)

const (
	WeeklyProgressModeAuto   = "auto"
	WeeklyProgressModeManual = "manual"

	weeklyProgressModeKeyPrefix  = "weekly_progress_mode."
	weeklyProgressCycleKeyPrefix = "weekly_progress_cycle."
)

func weeklyProgressModeKey(orgID string) string {
	return weeklyProgressModeKeyPrefix + strings.TrimSpace(orgID)
}

func weeklyProgressCycleKey(orgID string) string {
	return weeklyProgressCycleKeyPrefix + strings.TrimSpace(orgID)
}

func (r *StatsRepo) WeeklyProgressMode(orgID string) string {
	if r == nil || r.db == nil {
		return WeeklyProgressModeManual
	}
	var v string
	_ = r.db.QueryRow(`SELECT setting_value FROM app_settings WHERE setting_key=?`, weeklyProgressModeKey(orgID)).Scan(&v)
	if strings.TrimSpace(v) == WeeklyProgressModeAuto {
		return WeeklyProgressModeAuto
	}
	return WeeklyProgressModeManual
}

func (r *StatsRepo) WeeklyProgressCycle(orgID string) string {
	if r == nil || r.db == nil {
		return "반기"
	}
	var v string
	_ = r.db.QueryRow(`SELECT setting_value FROM app_settings WHERE setting_key=?`, weeklyProgressCycleKey(orgID)).Scan(&v)
	switch strings.TrimSpace(v) {
	case "월", "분기", "반기", "연":
		return strings.TrimSpace(v)
	default:
		return "반기"
	}
}

func (r *StatsRepo) SetWeeklyProgressMode(orgID, mode string) error {
	mode = strings.TrimSpace(mode)
	if mode != WeeklyProgressModeAuto && mode != WeeklyProgressModeManual {
		return fmt.Errorf("진행율 방식이 올바르지 않습니다")
	}
	return NewSettingsRepo(r.db).Set(weeklyProgressModeKey(orgID), mode)
}

func (r *StatsRepo) SetWeeklyProgressCycle(orgID, cycle string) error {
	switch strings.TrimSpace(cycle) {
	case "월", "분기", "반기", "연":
	default:
		return fmt.Errorf("주기가 올바르지 않습니다")
	}
	return NewSettingsRepo(r.db).Set(weeklyProgressCycleKey(orgID), strings.TrimSpace(cycle))
}

// CompanyMntMonthProgress 도서관 대시보드와 같은 월 목표·누적(CountMonthMntQuota + 완료 구간).
func (r *StatsRepo) CompanyMntMonthProgress(orgID string, weekMon time.Time) (quota, cum int, month int, err error) {
	wi := WeekInfoOf(weekMon)
	f := model.StatsMeetingFilter{OrgID: orgID, DateBasis: model.BasisComplete}
	quota, err = r.CountMonthMntQuota(wi.Year, wi.Month, f)
	if err != nil {
		return 0, 0, wi.Month, err
	}
	loc := weekMon.Location()
	monthStart := time.Date(wi.Year, time.Month(wi.Month), 1, 0, 0, 0, 0, loc)
	monthEnd := monthStart.AddDate(0, 1, 0)
	cumEnd := WeekInfoOf(weekMon).Monday.AddDate(0, 0, 7)
	if cumEnd.After(monthEnd) {
		cumEnd = monthEnd
	}
	if cumEnd.Before(monthStart) {
		cumEnd = monthStart
	}
	cum, err = r.countMntCompletedRange(monthStart.Format("2006-01-02"), cumEnd.Format("2006-01-02"), f)
	return quota, cum, wi.Month, err
}

func ProgressPct(done, total int) float64 {
	if total <= 0 {
		return 0
	}
	return math.Round(float64(done)/float64(total)*1000) / 10
}

func ParseProgressNumber(s string) (*int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return nil, fmt.Errorf("숫자를 입력하세요")
	}
	if n < 0 {
		return nil, fmt.Errorf("0 이상이어야 합니다")
	}
	return &n, nil
}

func ParseProgressPct(s string) (*float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, fmt.Errorf("진행율은 숫자여야 합니다")
	}
	if v < 0 || v > 100 {
		return nil, fmt.Errorf("진행율은 0부터 100까지입니다")
	}
	rounded := math.Round(v*10) / 10
	return &rounded, nil
}

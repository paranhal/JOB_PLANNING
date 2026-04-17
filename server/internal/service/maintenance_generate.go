package service

import (
	"fmt"
	"sort"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

const fixedRuleLastMonday = "LAST_MONDAY_OF_MONTH"

// AutoGenerateMaintenance 고정 규칙 사이트 배치 후, 나머지를 지역·가나다 순으로 평일(25일 이전 우선)에 나누어 넣는다.
func AutoGenerateMaintenance(repo *repository.MaintenanceRepo, planID string) error {
	plan, err := repo.GetPlan(planID)
	if err != nil || plan == nil {
		return fmt.Errorf("계획을 찾을 수 없습니다")
	}
	configs, err := repo.ListSiteConfigs()
	if err != nil {
		return err
	}
	if len(configs) == 0 {
		return fmt.Errorf("정기점검 사이트 설정이 없습니다. 먼저 「점검 사이트 설정」에서 등록하세요")
	}

	if err := repo.DeleteAutoVisits(planID); err != nil {
		return err
	}

	y := plan.PlanYear
	loc := time.Local

	var fixed, flex []model.MaintenanceSiteConfig
	for _, c := range configs {
		if c.FixedRule == fixedRuleLastMonday {
			fixed = append(fixed, c)
		} else {
			flex = append(flex, c)
		}
	}
	sort.Slice(flex, func(i, j int) bool {
		if flex[i].Region != flex[j].Region {
			return flex[i].Region < flex[j].Region
		}
		return flex[i].ShortName < flex[j].ShortName
	})

	for m := 1; m <= 12; m++ {
		placed := map[string]struct{}{}
		for _, c := range fixed {
			day := lastWeekdayOfMonth(y, m, time.Monday, loc)
			if day == 0 {
				continue
			}
			ds := fmt.Sprintf("%04d-%02d-%02d", y, m, day)
			t := time.Date(y, time.Month(m), day, 0, 0, 0, 0, loc)
			if IsKRHoliday(y, t) {
				continue
			}
			cat := c.EntryCategory
			if cat == "" {
				cat = "fixed"
			}
			if err := repo.InsertVisit(planID, ds, c.CustomerID, 0, 1, cat, ""); err != nil {
				return err
			}
			placed[c.CustomerID] = struct{}{}
		}

		workdays := workdaysForMonth(y, m, loc)
		perDay := 4
		dayIdx, slot := 0, 0
		for _, c := range flex {
			if _, ok := placed[c.CustomerID]; ok {
				continue
			}
			if dayIdx >= len(workdays) {
				return fmt.Errorf("%d년 %d월: 자동 배정 가능한 평일이 부족합니다. 수동으로 일정을 추가하거나 사이트를 나누세요", y, m)
			}
			ds := workdays[dayIdx]
			cat := c.EntryCategory
			if cat == "" {
				cat = "normal"
			}
			if err := repo.InsertVisit(planID, ds, c.CustomerID, slot, 1, cat, ""); err != nil {
				return err
			}
			slot++
			if slot >= perDay {
				slot = 0
				dayIdx++
			}
		}
	}

	repo.TouchPlanUpdated(planID)
	return nil
}

func lastWeekdayOfMonth(year, month int, wd time.Weekday, loc *time.Location) int {
	last := time.Date(year, time.Month(month+1), 0, 0, 0, 0, 0, loc).Day()
	for d := last; d >= 1; d-- {
		t := time.Date(year, time.Month(month), d, 0, 0, 0, 0, loc)
		if t.Weekday() == wd {
			return d
		}
	}
	return 0
}

// 25일 이전 평일을 앞에 두고, 부족 시 같은 달의 이후 평일을 이어 붙인다.
func workdaysForMonth(year, month int, loc *time.Location) []string {
	last := time.Date(year, time.Month(month+1), 0, 0, 0, 0, 0, loc).Day()
	var primary, extra []string
	for d := 1; d <= last; d++ {
		t := time.Date(year, time.Month(month), d, 0, 0, 0, 0, loc)
		if t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
			continue
		}
		if IsKRHoliday(year, t) {
			continue
		}
		ds := t.Format("2006-01-02")
		if d <= 24 {
			primary = append(primary, ds)
		} else {
			extra = append(extra, ds)
		}
	}
	out := append([]string{}, primary...)
	out = append(out, extra...)
	return out
}

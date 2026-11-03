package model

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	PlanYearMin = 2000
	PlanYearMax = 2100
)

func ValidPlanYear(y int) bool {
	return y >= PlanYearMin && y <= PlanYearMax
}

func YearFromDatePrefix(s string) int {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "/", "-")
	if len(s) < 4 || s[0] < '0' || s[0] > '9' {
		return 0
	}
	if len(s) >= 5 && s[4] != '-' {
		return 0
	}
	y, err := strconv.Atoi(s[:4])
	if err != nil || !ValidPlanYear(y) {
		return 0
	}
	return y
}

func DefaultPlanYearFromSales(s *SalesProject) int {
	if s != nil {
		if y := YearFromDatePrefix(s.ExpectedYM); y > 0 {
			return y
		}
		if y := YearFromDatePrefix(s.RevenueFrom); y > 0 {
			return y
		}
		if y := YearFromDatePrefix(s.ContractedAt); y > 0 {
			return y
		}
		if ValidPlanYear(s.BudgetYear) {
			return s.BudgetYear
		}
	}
	return time.Now().Year()
}

func ApplyPlanYearDefault(p *WorkProject, sales *SalesProject) {
	if p == nil {
		return
	}
	if ValidPlanYear(p.PlanYear) {
		return
	}
	if sales != nil {
		p.PlanYear = DefaultPlanYearFromSales(sales)
		return
	}
	if y := YearFromDatePrefix(p.StartDate); y > 0 {
		p.PlanYear = y
		return
	}
	p.PlanYear = time.Now().Year()
}

func ValidatePlanYearDates(p *WorkProject) error {
	if p == nil {
		return nil
	}
	if p.PlanYear != 0 && !ValidPlanYear(p.PlanYear) {
		return fmt.Errorf("사업 연도는 %d~%d 사이여야 합니다", PlanYearMin, PlanYearMax)
	}
	start := strings.TrimSpace(p.StartDate)
	end := strings.TrimSpace(p.EndDate)
	if start != "" && end != "" && end < start {
		return fmt.Errorf("계약기간 마감은 시작일보다 빠를 수 없습니다")
	}
	return nil
}

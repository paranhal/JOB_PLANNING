package repository

import (
	"fmt"
	"strconv"
	"strings"

	"customer-support/internal/model"
)

func (r *MaintenanceRepo) siteRounds(customerID string) int {
	cfg, _ := r.GetSiteConfig(customerID)
	if cfg == nil {
		return 1
	}
	return cfg.SiteRounds()
}

func (r *MaintenanceRepo) visitPeriodFilter(customerID, visitDate string) func(model.MaintenanceVisit) bool {
	cfg, _ := r.GetSiteConfig(customerID)
	cycle := "monthly"
	if cfg != nil && strings.TrimSpace(cfg.InspectionCycle) != "" {
		cycle = cfg.InspectionCycle
	}
	visitDate = strings.TrimSpace(visitDate)
	if len(visitDate) < 7 {
		return func(model.MaintenanceVisit) bool { return false }
	}
	year, _ := strconv.Atoi(visitDate[:4])
	month, _ := strconv.Atoi(visitDate[5:7])
	if p, ok := model.CyclePeriodOf(cycle, month); ok {
		return func(v model.MaintenanceVisit) bool {
			return model.VisitInCyclePeriod(v.VisitDate, year, p)
		}
	}
	ym := visitDate[:7]
	return func(v model.MaintenanceVisit) bool {
		d := strings.TrimSpace(v.VisitDate)
		return len(d) >= 7 && d[:7] == ym
	}
}

func (r *MaintenanceRepo) CountPeriodRounds(planID, customerID, productType, visitDate string) int {
	n := 0
	inPeriod := r.visitPeriodFilter(customerID, visitDate)
	catalog := r.slotCatalogForCustomer(customerID)
	want := model.VisitProductSlotKeyFor(productType, catalog)
	visits, err := r.ListVisits(planID)
	if err != nil {
		return 0
	}
	seen := map[int]bool{}
	for _, v := range visits {
		if v.CustomerID != customerID {
			continue
		}
		if model.VisitProductSlotKeyFor(v.ProductType, catalog) != want {
			continue
		}
		if !inPeriod(v) {
			continue
		}
		rn := v.VisitRound()
		if seen[rn] {
			continue
		}
		seen[rn] = true
		n++
	}
	return n
}

func (r *MaintenanceRepo) nextRoundNo(planID, customerID, productType, visitDate, exceptID string) int {
	used := map[int]bool{}
	inPeriod := r.visitPeriodFilter(customerID, visitDate)
	catalog := r.slotCatalogForCustomer(customerID)
	want := model.VisitProductSlotKeyFor(productType, catalog)
	visits, err := r.ListVisits(planID)
	if err != nil {
		return 1
	}
	except := strings.TrimSpace(exceptID)
	for _, v := range visits {
		if v.VisitID == except || v.CustomerID != customerID {
			continue
		}
		if model.VisitProductSlotKeyFor(v.ProductType, catalog) != want {
			continue
		}
		if !inPeriod(v) {
			continue
		}
		used[v.VisitRound()] = true
	}
	for i := 1; i < 100; i++ {
		if !used[i] {
			return i
		}
	}
	return 1
}

func (r *MaintenanceRepo) guardVisitRound(v model.MaintenanceVisit, exceptID string, autoRound, allowExtra bool) error {
	if strings.TrimSpace(v.DupReason) != "" {
		return nil
	}
	round := v.VisitRound()
	needed := r.siteRounds(v.CustomerID)
	if round > needed {
		if autoRound && allowExtra {
			return nil
		}
		if autoRound {
			if dup, err := r.PeriodRoundDuplicate(v.PlanID, v.CustomerID, v.ProductType, v.VisitDate, exceptID, 1); err != nil {
				return err
			} else if dup != nil {
				return ErrMonthDuplicate(dup)
			}
		}
		return fmt.Errorf("주기당 회차 %d를 넘습니다. 추가하려면 사유를 입력하세요", needed)
	}
	dup, err := r.PeriodRoundDuplicate(v.PlanID, v.CustomerID, v.ProductType, v.VisitDate, exceptID, round)
	if err != nil {
		return err
	}
	if dup != nil {
		return ErrMonthDuplicate(dup)
	}
	return nil
}

// PeriodRoundDuplicate 같은 주기·같은 제품군·같은 회차가 있으면 알려 준다.
func (r *MaintenanceRepo) PeriodRoundDuplicate(planID, customerID, productType, visitDate, exceptVisitID string, roundNo int) (*model.MaintenanceVisit, error) {
	planID, customerID, visitDate = strings.TrimSpace(planID), strings.TrimSpace(customerID), strings.TrimSpace(visitDate)
	if planID == "" || customerID == "" || len(visitDate) < 7 {
		return nil, nil
	}
	if roundNo < 1 {
		roundNo = 1
	}
	visits, err := r.ListVisits(planID)
	if err != nil {
		return nil, err
	}
	catalog := r.slotCatalogForCustomer(customerID)
	want := model.VisitProductSlotKeyFor(productType, catalog)
	except := strings.TrimSpace(exceptVisitID)
	inPeriod := r.visitPeriodFilter(customerID, visitDate)
	for i := range visits {
		v := visits[i]
		if v.VisitID == except || v.CustomerID != customerID {
			continue
		}
		if model.VisitProductSlotKeyFor(v.ProductType, catalog) != want {
			continue
		}
		if !inPeriod(v) {
			continue
		}
		if v.VisitRound() != roundNo {
			continue
		}
		return &v, nil
	}
	return nil, nil
}

func (r *MaintenanceRepo) PeriodClosed(planID, customerID, productType string, year, month int) bool {
	if month < 1 {
		month = 1
	}
	date := fmt.Sprintf("%04d-%02d-15", year, month)
	needed := r.siteRounds(customerID)
	inPeriod := r.visitPeriodFilter(customerID, date)
	catalog := r.slotCatalogForCustomer(customerID)
	want := model.VisitProductSlotKeyFor(productType, catalog)
	visits, err := r.ListVisits(planID)
	if err != nil {
		return false
	}
	done := map[int]bool{}
	for _, v := range visits {
		if v.CustomerID != customerID || !v.Completed {
			continue
		}
		if model.VisitProductSlotKeyFor(v.ProductType, catalog) != want {
			continue
		}
		if !inPeriod(v) {
			continue
		}
		done[v.VisitRound()] = true
	}
	return len(done) >= needed
}

func parseRoundForm(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

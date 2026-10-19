package repository

import (
	"strings"

	"customer-support/internal/model"
)

// QuarterObligations 분기 주기 사이트의 점검대상 중, 그 달력 분기에
// 방문(완료 또는 일정 확정)이 아직 없는 슬롯. 월·격월·반기·년1회는 넣지 않는다.
// 세 호출처(계획 화면·관리 미배정·미계획함)가 공유한다(D-70).
func (r *MaintenanceRepo) QuarterObligations(planID string, year, quarter int) ([]model.UnassignedMonthSlot, error) {
	planID = strings.TrimSpace(planID)
	if planID == "" || year < 1 || quarter < 1 || quarter > 4 {
		return nil, nil
	}
	configs, err := r.ListSiteConfigs()
	if err != nil {
		return nil, err
	}
	visits, err := r.ListVisits(planID)
	if err != nil {
		return nil, err
	}
	covered := map[string]map[string]bool{}
	for _, v := range visits {
		if !model.VisitInCalendarQuarter(v.VisitDate, year, quarter) {
			continue
		}
		if covered[v.CustomerID] == nil {
			covered[v.CustomerID] = map[string]bool{}
		}
		covered[v.CustomerID][model.VisitProductSlotKey(v.ProductType)] = true
	}
	startMonth, _ := model.QuarterMonthRange(quarter)
	var out []model.UnassignedMonthSlot
	for _, cfg := range configs {
		if !model.IsQuarterlyCycle(cfg.InspectionCycle) {
			continue
		}
		slots := model.SiteDueProductSlots(cfg, startMonth)
		have := covered[cfg.CustomerID]
		genericUsed := have["generic"]
		for _, s := range slots {
			if have[s.SlotKey] {
				continue
			}
			if genericUsed && s.SlotKey != "generic" {
				genericUsed = false
				continue
			}
			out = append(out, s)
		}
	}
	return out, nil
}

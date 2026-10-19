package repository

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

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

// ListOpenUnassignedSlots 미배정·미계획함·배지가 공유하는 목록(D-71).
// 분기 슬롯은 분기 마지막 달에만, 확인·건너뜀이 없는 건을 「분기 점검 미확정」으로 넣는다.
func (r *MaintenanceRepo) ListOpenUnassignedSlots(planID string, year, month int) ([]model.UnassignedMonthSlot, error) {
	slots, err := r.ListUnassignedMonthSlots(planID, year, month)
	if err != nil {
		return nil, err
	}
	var out []model.UnassignedMonthSlot
	for _, s := range slots {
		if model.IsQuarterlyCycle(s.Cycle) {
			continue
		}
		out = append(out, s)
	}
	if model.IsLastMonthOfQuarter(month) {
		qSlots, err := r.PendingQuarterSlots(planID, year, model.CalendarQuarter(month))
		if err != nil {
			return nil, err
		}
		for _, s := range qSlots {
			s.UnconfirmedQuarter = true
			out = append(out, s)
		}
	}
	return out, nil
}

// PendingQuarterSlots 그 분기에 아직 방문도 건너뜀도 없는 분기 주기 슬롯.
func (r *MaintenanceRepo) PendingQuarterSlots(planID string, year, quarter int) ([]model.UnassignedMonthSlot, error) {
	slots, err := r.QuarterObligations(planID, year, quarter)
	if err != nil {
		return nil, err
	}
	skip, err := r.quarterSkipSet(planID, year, quarter)
	if err != nil {
		return nil, err
	}
	var out []model.UnassignedMonthSlot
	for _, s := range slots {
		if skip[s.CustomerID+"|"+s.SlotKey] {
			continue
		}
		s.LastAssignee = r.lookupAssignee(planID, s.CustomerID, s.ProductType)
		out = append(out, s)
	}
	return out, nil
}

func (r *MaintenanceRepo) quarterSkipSet(planID string, year, quarter int) (map[string]bool, error) {
	out := map[string]bool{}
	rows, err := r.db.Query(`
		SELECT customer_id, slot_key FROM maintenance_quarter_decisions
		WHERE plan_id=? AND year=? AND quarter=? AND action='skip'`,
		planID, year, quarter)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return out, nil
		}
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var cust, slot string
		if err := rows.Scan(&cust, &slot); err != nil {
			return nil, err
		}
		out[cust+"|"+slot] = true
	}
	return out, rows.Err()
}

// SkipQuarterSlot 이번 분기 이 슬롯을 건너뛴다. 같은 키면 사유를 덮어쓴다.
func (r *MaintenanceRepo) SkipQuarterSlot(planID, customerID, slotKey, reason, createdBy string, year, quarter int) error {
	planID, customerID, slotKey = strings.TrimSpace(planID), strings.TrimSpace(customerID), strings.TrimSpace(slotKey)
	reason = strings.TrimSpace(reason)
	if planID == "" || customerID == "" || slotKey == "" {
		return fmt.Errorf("사이트와 점검대상을 선택하세요")
	}
	if reason == "" {
		return fmt.Errorf("건너뛰려면 사유를 적으세요")
	}
	if year < 1 || quarter < 1 || quarter > 4 {
		return fmt.Errorf("분기를 확인하세요")
	}
	id := randomID("mqd_")
	now := time.Now().Format("2006-01-02 15:04:05")
	if r.Now != nil {
		now = r.Now().Format("2006-01-02 15:04:05")
	}
	_, err := r.db.Exec(`
		INSERT INTO maintenance_quarter_decisions
		(decision_id, plan_id, customer_id, slot_key, year, quarter, action, reason, created_by, created_at)
		VALUES (?,?,?,?,?,?, 'skip', ?, ?, ?)
		ON CONFLICT(plan_id, customer_id, slot_key, year, quarter) DO UPDATE SET
			action='skip', reason=excluded.reason, created_by=excluded.created_by, created_at=excluded.created_at`,
		id, planID, customerID, slotKey, year, quarter, reason, strings.TrimSpace(createdBy), now)
	return err
}

// FilterUnassignedSlotsByOrg 빈 org_id 행은 레거시로 두고, 값이 있으면 조직만 남긴다.
func FilterUnassignedSlotsByOrg(slots []model.UnassignedMonthSlot, orgID string) []model.UnassignedMonthSlot {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" || orgID == OrgAll {
		return slots
	}
	var out []model.UnassignedMonthSlot
	for _, s := range slots {
		if strings.TrimSpace(s.OrgID) == "" || s.OrgID == orgID {
			out = append(out, s)
		}
	}
	return out
}

func (r *MaintenanceRepo) CustomerOrgID(customerID string) string {
	if r == nil || r.db == nil {
		return ""
	}
	var org string
	err := r.db.QueryRow(`SELECT COALESCE(org_id,'') FROM customers WHERE customer_id=?`, strings.TrimSpace(customerID)).Scan(&org)
	if err != nil {
		return ""
	}
	return org
}

func applyMaintenanceQuarterDecisionsV76(db *sql.DB) {
	if db == nil {
		return
	}
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS maintenance_quarter_decisions (
			decision_id TEXT PRIMARY KEY,
			plan_id TEXT NOT NULL,
			customer_id TEXT NOT NULL,
			slot_key TEXT NOT NULL,
			year INTEGER NOT NULL,
			quarter INTEGER NOT NULL,
			action TEXT NOT NULL DEFAULT 'skip',
			reason TEXT NOT NULL DEFAULT '',
			created_by TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT (datetime('now','localtime'))
		);
		CREATE UNIQUE INDEX IF NOT EXISTS uq_mnt_quarter_decisions
			ON maintenance_quarter_decisions(plan_id, customer_id, slot_key, year, quarter);`)
	if err != nil {
		log.Printf("062 quarter decisions: %v", err)
	}
}

package repository

import (
	"database/sql"
	"log"
	"strings"
)

// applyMaintenancePlanOrgYearUnique 연도 단독 UNIQUE 를 (org_id, plan_year) 로 바꾼다.
// 기존 행의 org_id 는 채우지 않는다.
func applyMaintenancePlanOrgYearUnique(db *sql.DB) {
	if db == nil {
		return
	}
	if _, err := db.Exec(`DROP INDEX IF EXISTS idx_maintenance_plans_year`); err != nil {
		log.Printf("066 drop idx_maintenance_plans_year: %v", err)
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_maintenance_plans_org_year ON maintenance_plans(org_id, plan_year)`); err != nil {
		log.Printf("066 idx_maintenance_plans_org_year: %v", err)
	}
}

// CountEmptyOrgRows 조직 값이 비어 있는 행 수. 임의로 도서관에 넣지 않고 보고용으로만 센다.
func CountEmptyOrgRows(db *sql.DB, table string) int {
	if db == nil {
		return 0
	}
	switch table {
	case "maintenance_plans", "as_receipts":
	default:
		return 0
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM ` + table + ` WHERE TRIM(COALESCE(org_id,''))=''`).Scan(&n)
	return n
}

func orgIDForFilter(orgID string) string {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return OrgAll
	}
	return orgID
}

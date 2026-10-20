package repository

import (
	"database/sql"
	"fmt"
	"log"
	"os"
)

func applyProjectYearsSitesEquipment(db *sql.DB) {
	if db == nil {
		return
	}
	up, err := projectEquipMigrationSQL()
	if err != nil {
		log.Printf("070 project years/sites/equipment: %v", err)
		return
	}
	if err := execSQLIgnoreDupCol(db, up); err != nil {
		log.Printf("070 project years/sites/equipment apply: %v", err)
	}
}

func projectEquipMigrationSQL() (string, error) {
	for _, p := range workGroupSQLPaths("070_project_years_sites_equipment.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("070_project_years_sites_equipment.sql 없음")
}

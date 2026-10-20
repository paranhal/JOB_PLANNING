package repository

import (
	"database/sql"
	"fmt"
	"log"
	"os"
)

func applyEquipmentCategoryImportBatches(db *sql.DB) {
	if db == nil {
		return
	}
	up, err := equipCatImportMigrationSQL()
	if err != nil {
		log.Printf("071 equipment_category_import_batches: %v", err)
		return
	}
	if err := execSQLIgnoreDupCol(db, up); err != nil {
		log.Printf("071 equipment_category_import_batches apply: %v", err)
	}
}

func equipCatImportMigrationSQL() (string, error) {
	for _, p := range workGroupSQLPaths("071_equipment_category_import_batches.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("071_equipment_category_import_batches.sql 없음")
}

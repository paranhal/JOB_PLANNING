package repository

import (
	"database/sql"
	"fmt"
	"log"
	"os"
)

func applyInspectionProducts(db *sql.DB) {
	if db == nil {
		return
	}
	up, err := inspectionProductMigrationSQL()
	if err != nil {
		log.Printf("072 inspection_products: %v", err)
		return
	}
	if err := execSQLIgnoreDupCol(db, up); err != nil {
		log.Printf("072 inspection_products apply: %v", err)
	}
}

func inspectionProductMigrationSQL() (string, error) {
	for _, p := range workGroupSQLPaths("072_inspection_products.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("072_inspection_products.sql 없음")
}

func inspectionProductDownSQL() (string, error) {
	for _, p := range workGroupSQLPaths("072_inspection_products.down.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("072_inspection_products.down.sql 없음")
}

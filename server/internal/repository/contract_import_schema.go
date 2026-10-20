package repository

import (
	"database/sql"
	"fmt"
	"log"
	"os"
)

func applyContractImport(db *sql.DB) {
	if db == nil {
		return
	}
	up, err := contractImportMigrationSQL()
	if err != nil {
		log.Printf("073 contract_import: %v", err)
		return
	}
	if err := execSQLIgnoreDupCol(db, up); err != nil {
		log.Printf("073 contract_import apply: %v", err)
	}
}

func contractImportMigrationSQL() (string, error) {
	for _, p := range workGroupSQLPaths("073_contract_import.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("073_contract_import.sql 없음")
}

func contractImportDownSQL() (string, error) {
	for _, p := range workGroupSQLPaths("073_contract_import.down.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("073_contract_import.down.sql 없음")
}

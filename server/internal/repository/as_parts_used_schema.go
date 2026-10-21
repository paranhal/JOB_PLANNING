package repository

import (
	"database/sql"
	"fmt"
	"log"
	"os"
)

func applyASPartsUsed(db *sql.DB) {
	if db == nil {
		return
	}
	up, err := asPartsUsedMigrationSQL()
	if err != nil {
		log.Printf("074 as_parts_used: %v", err)
		return
	}
	if err := execSQLIgnoreDupCol(db, up); err != nil {
		log.Printf("074 as_parts_used apply: %v", err)
	}
}

func asPartsUsedMigrationSQL() (string, error) {
	for _, p := range workGroupSQLPaths("074_as_parts_used.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("074_as_parts_used.sql 없음")
}

func asPartsUsedDownSQL() (string, error) {
	for _, p := range workGroupSQLPaths("074_as_parts_used.down.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("074_as_parts_used.down.sql 없음")
}

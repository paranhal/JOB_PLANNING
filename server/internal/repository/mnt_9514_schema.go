package repository

import (
	"database/sql"
	"fmt"
	"log"
	"os"
)

func applyMnt9514(db *sql.DB) {
	if db == nil {
		return
	}
	up, err := mnt9514MigrationSQL()
	if err != nil {
		log.Printf("082 mnt rounds: %v", err)
		return
	}
	if err := execSQLIgnoreDupCol(db, up); err != nil {
		log.Printf("082 mnt rounds apply: %v", err)
	}
}

func mnt9514MigrationSQL() (string, error) {
	for _, p := range workGroupSQLPaths("082_mnt_rounds_assignees.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("082_mnt_rounds_assignees.sql 없음")
}

func mnt9514DownSQL() (string, error) {
	for _, p := range workGroupSQLPaths("082_mnt_rounds_assignees.down.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("082_mnt_rounds_assignees.down.sql 없음")
}

package repository

import (
	"database/sql"
	"fmt"
	"log"
	"os"
)

func applyWorkTaskGroupV81(db *sql.DB) {
	if db == nil {
		return
	}
	up, err := workTaskGroupMigrationSQL()
	if err != nil {
		log.Printf("081 work_tasks group: %v", err)
		return
	}
	if err := execSQLIgnoreDupCol(db, up); err != nil {
		log.Printf("081 work_tasks group apply: %v", err)
	}
}

func workTaskGroupMigrationSQL() (string, error) {
	for _, p := range workGroupSQLPaths("081_work_tasks_group.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("081_work_tasks_group.sql 없음")
}

func workTaskGroupDownSQL() (string, error) {
	for _, p := range workGroupSQLPaths("081_work_tasks_group.down.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("081_work_tasks_group.down.sql 없음")
}

package repository

import (
	"database/sql"
	"fmt"
	"log"
	"os"
)

func applyWorkTaskProjectWork(db *sql.DB) {
	if db == nil {
		return
	}
	if pragmaHasColumn(db, "work_task_sites", "customer_id") && !pragmaHasColumn(db, "work_task_sites", "site_id") {
		return
	}
	up, err := workTaskProjectWorkMigrationSQL()
	if err != nil {
		log.Printf("076 work_task_project_work: %v", err)
		return
	}
	if err := execSQLIgnoreDupCol(db, up); err != nil {
		log.Printf("076 work_task_project_work apply: %v", err)
	}
}

func workTaskProjectWorkMigrationSQL() (string, error) {
	for _, p := range workGroupSQLPaths("076_work_task_project_work.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("076_work_task_project_work.sql 없음")
}

func workTaskProjectWorkDownSQL() (string, error) {
	for _, p := range workGroupSQLPaths("076_work_task_project_work.down.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("076_work_task_project_work.down.sql 없음")
}

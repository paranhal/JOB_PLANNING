package repository

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
)

func applyWorkTaskActionV79(db *sql.DB) {
	if db == nil {
		return
	}
	up, err := workTaskActionMigrationSQL()
	if err != nil {
		log.Printf("079 work_task_action: %v", err)
		return
	}
	if err := execSQLIgnoreDupCol(db, up); err != nil {
		log.Printf("079 work_task_action apply: %v", err)
	}
}

func workTaskActionMigrationSQL() (string, error) {
	for _, p := range workGroupSQLPaths("079_work_task_action.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("079_work_task_action.sql 없음")
}

func workTaskActionDownSQL() (string, error) {
	for _, p := range workGroupSQLPaths("079_work_task_action.down.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("079_work_task_action.down.sql 없음")
}

func workTaskActionSQLHasSeedInsert(sqlText string) bool {
	for _, line := range strings.Split(sqlText, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "--") {
			continue
		}
		u := strings.ToUpper(trim)
		if strings.Contains(u, "INSERT INTO WORK_TASK_PROCESSES") ||
			strings.Contains(u, "INSERT INTO WORK_TASK_PARTS_USED") ||
			strings.Contains(u, "INSERT INTO WORK_TASKS") {
			return true
		}
	}
	return false
}

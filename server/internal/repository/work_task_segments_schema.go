package repository

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
)

func applyWorkTaskSegmentsV95(db *sql.DB) {
	if db == nil {
		return
	}
	up, err := workTaskSegmentsMigrationSQL()
	if err != nil {
		log.Printf("095 work_task_segments: %v", err)
		return
	}
	if err := execWorkGroupSQL(db, up); err != nil {
		log.Printf("095 work_task_segments apply: %v", err)
	}
}

func workTaskSegmentsMigrationSQL() (string, error) {
	for _, p := range workGroupSQLPaths("095_work_task_segments.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("095_work_task_segments.sql 없음")
}

func workTaskSegmentsDownSQL() (string, error) {
	for _, p := range workGroupSQLPaths("095_work_task_segments.down.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("095_work_task_segments.down.sql 없음")
}

func workTaskSegmentSQLHasSeedInsert(sqlText string) bool {
	for _, line := range strings.Split(sqlText, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "--") {
			continue
		}
		u := strings.ToUpper(trim)
		if strings.Contains(u, "INSERT INTO") {
			return true
		}
	}
	return false
}

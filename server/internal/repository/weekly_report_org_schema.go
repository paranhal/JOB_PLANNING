package repository

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
)

func applyWeeklyReportOrgV78(db *sql.DB) {
	if db == nil {
		return
	}
	up, err := weeklyReportOrgMigrationSQL()
	if err != nil {
		log.Printf("078 weekly_report_org: %v", err)
		return
	}
	if err := execSQLIgnoreDupCol(db, up); err != nil {
		log.Printf("078 weekly_report_org apply: %v", err)
	}
}

func weeklyReportOrgMigrationSQL() (string, error) {
	for _, p := range workGroupSQLPaths("078_weekly_report_org.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("078_weekly_report_org.sql 없음")
}

func weeklyReportOrgDownSQL() (string, error) {
	for _, p := range workGroupSQLPaths("078_weekly_report_org.down.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("078_weekly_report_org.down.sql 없음")
}

func weeklyReportSQLHasSeedInsert(sqlText string) bool {
	for _, line := range strings.Split(sqlText, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "--") {
			continue
		}
		u := strings.ToUpper(trim)
		if strings.Contains(u, "INSERT INTO WEEKLY_REPORT_ROWS") ||
			strings.Contains(u, "INSERT INTO COMPANY_WEEKLY_REPORTS") {
			return true
		}
	}
	return false
}

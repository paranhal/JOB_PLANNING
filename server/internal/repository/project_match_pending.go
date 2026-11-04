package repository

import (
	"database/sql"
	"fmt"
	"log"
	"os"
)

func applyProjectMatchPendingV104(db *sql.DB) {
	if db == nil {
		return
	}
	up, err := projectMatchPendingSQL()
	if err != nil {
		log.Printf("104 project_match_pending: %v", err)
		return
	}
	if err := execWorkGroupSQLIgnoreDupCol(db, up); err != nil {
		log.Printf("104 project_match_pending apply: %v", err)
	}
}

func projectMatchPendingSQL() (string, error) {
	for _, p := range workGroupSQLPaths("104_project_match_pending.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("104_project_match_pending.sql 없음")
}

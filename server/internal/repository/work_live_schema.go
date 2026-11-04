package repository

import (
	"database/sql"
	"fmt"
	"log"
	"os"
)

func applyWorkLiveV103(db *sql.DB) {
	if db == nil {
		return
	}
	up, err := workLiveMigrationSQL()
	if err != nil {
		log.Printf("103 work_live: %v", err)
		return
	}
	if err := execWorkGroupSQLIgnoreDupCol(db, up); err != nil {
		log.Printf("103 work_live apply: %v", err)
	}
}

func workLiveMigrationSQL() (string, error) {
	for _, p := range workGroupSQLPaths("103_work_live.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("103_work_live.sql 없음")
}

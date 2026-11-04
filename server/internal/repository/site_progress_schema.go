package repository

import (
	"database/sql"
	"fmt"
	"log"
	"os"
)

func applySiteProgressV105(db *sql.DB) {
	if db == nil {
		return
	}
	up, err := siteProgressSQL()
	if err != nil {
		log.Printf("105 site_progress: %v", err)
		return
	}
	if err := execWorkGroupSQLIgnoreDupCol(db, up); err != nil {
		log.Printf("105 site_progress apply: %v", err)
	}
}

func siteProgressSQL() (string, error) {
	for _, p := range workGroupSQLPaths("105_site_progress.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("105_site_progress.sql 없음")
}

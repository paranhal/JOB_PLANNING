package repository

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
)

func applyMnt9515A(db *sql.DB) {
	if db == nil {
		return
	}
	up, err := mnt9515AMigrationSQL()
	if err != nil {
		log.Printf("083 mnt bulk batch: %v", err)
		return
	}
	if err := execSQLIgnoreDupCol(db, up); err != nil {
		log.Printf("083 mnt bulk batch apply: %v", err)
	}
}

func mnt9515AMigrationSQL() (string, error) {
	for _, p := range workGroupSQLPaths("083_mnt_site_bulk_batch.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("083_mnt_site_bulk_batch.sql 없음")
}

func mnt9515ADownSQL() (string, error) {
	for _, p := range workGroupSQLPaths("083_mnt_site_bulk_batch.down.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("083_mnt_site_bulk_batch.down.sql 없음")
}

func mnt9515AHasSeedInsert(up string) bool {
	for _, line := range strings.Split(up, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "--") {
			continue
		}
		if strings.Contains(strings.ToUpper(trim), "INSERT") {
			return true
		}
	}
	return false
}

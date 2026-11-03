package repository

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
)

func applyWorkTaskItemExtV97(db *sql.DB) {
	if db == nil {
		return
	}
	up, err := workTaskItemExtMigrationSQL()
	if err != nil {
		log.Printf("097 work_task_item_ext: %v", err)
		return
	}
	if err := execWorkGroupSQLIgnoreDupCol(db, up); err != nil {
		log.Printf("097 work_task_item_ext apply: %v", err)
	}
}

func workTaskItemExtMigrationSQL() (string, error) {
	for _, p := range workGroupSQLPaths("097_work_task_item_ext.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("097_work_task_item_ext.sql 없음")
}

func workTaskItemExtDownSQL() (string, error) {
	for _, p := range workGroupSQLPaths("097_work_task_item_ext.down.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("097_work_task_item_ext.down.sql 없음")
}

func applyWorkTaskDelaysV96(db *sql.DB) {
	if db == nil {
		return
	}
	up, err := workTaskDelaysMigrationSQL()
	if err != nil {
		log.Printf("096 work_task_delays: %v", err)
		return
	}
	if err := execWorkGroupSQLIgnoreDupCol(db, up); err != nil {
		log.Printf("096 work_task_delays apply: %v", err)
	}
}

func workTaskDelaysMigrationSQL() (string, error) {
	for _, p := range workGroupSQLPaths("096_work_task_delays.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("096_work_task_delays.sql 없음")
}

func workTaskDelaysDownSQL() (string, error) {
	for _, p := range workGroupSQLPaths("096_work_task_delays.down.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("096_work_task_delays.down.sql 없음")
}

func execWorkGroupSQLIgnoreDupCol(db *sql.DB, script string) error {
	var b strings.Builder
	flush := func() error {
		stmt := strings.TrimSpace(b.String())
		b.Reset()
		if stmt == "" {
			return nil
		}
		if _, err := db.Exec(stmt); err != nil {
			low := strings.ToLower(err.Error())
			if strings.Contains(low, "duplicate column") {
				return nil
			}
			return fmt.Errorf("%s: %w", firstSQLLine(stmt), err)
		}
		return nil
	}
	for _, line := range strings.Split(script, "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "--") {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
		if strings.HasSuffix(trim, ";") {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	return flush()
}

func workTaskDelaySQLHasSeedInsert(sqlText string) bool {
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

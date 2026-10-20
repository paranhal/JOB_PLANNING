package repository

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
)

func applyWorkCooperations(db *sql.DB) {
	if db == nil {
		return
	}
	up, err := coopMigrationSQL()
	if err != nil {
		log.Printf("069 work_cooperations: %v", err)
		return
	}
	if err := execSQLIgnoreDupCol(db, up); err != nil {
		log.Printf("069 work_cooperations apply: %v", err)
	}
}

func coopMigrationSQL() (string, error) {
	for _, p := range workGroupSQLPaths("069_work_cooperations.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("069_work_cooperations.sql 없음")
}

func coopDownSQL() (string, error) {
	for _, p := range workGroupSQLPaths("069_work_cooperations.down.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("069_work_cooperations.down.sql 없음")
}

func execSQLIgnoreDupCol(db *sql.DB, script string) error {
	var b strings.Builder
	depth := 0
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
		up := strings.ToUpper(trim)
		b.WriteString(line)
		b.WriteByte('\n')
		if strings.Contains(up, " BEGIN") || strings.HasSuffix(up, "BEGIN") {
			depth++
		}
		if strings.HasPrefix(up, "END;") || up == "END" || strings.HasPrefix(up, "END ") {
			if depth > 0 {
				depth--
			}
		}
		if depth == 0 && strings.HasSuffix(trim, ";") {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	return flush()
}

package repository

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func applyWorkGroups(db *sql.DB) {
	if db == nil {
		return
	}
	up, err := workGroupMigrationSQL()
	if err != nil {
		log.Printf("068 work_groups: %v", err)
		return
	}
	if err := execWorkGroupSQL(db, up); err != nil {
		log.Printf("068 work_groups apply: %v", err)
	}
}

func workGroupSQLPaths(name string) []string {
	out := []string{
		filepath.Join("migrations", name),
		filepath.Join("..", "..", "migrations", name),
		filepath.Join("..", "..", "..", "migrations", name),
	}
	if _, file, _, ok := runtime.Caller(0); ok {
		out = append(out, filepath.Join(filepath.Dir(file), "..", "..", "migrations", name))
	}
	return out
}

func workGroupMigrationSQL() (string, error) {
	for _, p := range workGroupSQLPaths("068_work_groups.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("068_work_groups.sql 없음")
}

func workGroupDownSQL() (string, error) {
	for _, p := range workGroupSQLPaths("068_work_groups.down.sql") {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("068_work_groups.down.sql 없음")
}

func execWorkGroupSQL(db *sql.DB, script string) error {
	var b strings.Builder
	depth := 0
	flush := func() error {
		stmt := strings.TrimSpace(b.String())
		b.Reset()
		if stmt == "" {
			return nil
		}
		if _, err := db.Exec(stmt); err != nil {
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

func firstSQLLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			if len(line) > 80 {
				return line[:80]
			}
			return line
		}
	}
	return ""
}

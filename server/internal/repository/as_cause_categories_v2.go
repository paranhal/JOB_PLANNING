package repository

import (
	"database/sql"
	_ "embed"
	"log"
	"strings"
)

//go:embed as_cause_categories_v2.sql
var causeCategoriesV2SQL string

// applyASCauseCategoriesV2 마이그레이션 030. §34.3.3 엑셀 2026-09-02 판(55행).
func applyASCauseCategoriesV2(db *sql.DB) {
	if _, err := db.Exec(`ALTER TABLE as_cause_categories ADD COLUMN cause_type_map TEXT NOT NULL DEFAULT ''`); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		log.Printf("030 as_cause_categories.cause_type_map: %v", err)
	}
	if _, err := db.Exec(`ALTER TABLE as_cause_categories ADD COLUMN is_fault INTEGER NOT NULL DEFAULT 1`); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		log.Printf("030 as_cause_categories.is_fault: %v", err)
	}
	for _, stmt := range causeCategorySeedStatements(causeCategoriesV2SQL) {
		if _, err := db.Exec(stmt); err != nil {
			log.Printf("030 cause seed: %v", err)
			return
		}
	}
}

func causeCategorySeedStatements(raw string) []string {
	var stmts []string
	var b strings.Builder
	skipCreate := false
	for _, line := range strings.Split(raw, "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "--") {
			continue
		}
		upper := strings.ToUpper(trim)
		if strings.HasPrefix(upper, "CREATE TABLE") || strings.HasPrefix(upper, "CREATE INDEX") {
			skipCreate = true
		}
		if skipCreate {
			if strings.Contains(trim, ";") {
				skipCreate = false
			}
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
		if strings.Contains(trim, ";") {
			s := strings.TrimSpace(b.String())
			b.Reset()
			s = strings.TrimSuffix(s, ";")
			s = strings.TrimSpace(s)
			if s != "" {
				stmts = append(stmts, s)
			}
		}
	}
	rest := strings.TrimSpace(b.String())
	if rest != "" {
		stmts = append(stmts, strings.TrimSuffix(rest, ";"))
	}
	return stmts
}

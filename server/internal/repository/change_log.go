package repository

import (
	"database/sql"
	"encoding/json"
	"strings"

	"customer-support/internal/audit"
)

func rowJSON(db *sql.DB, table, pkCol, id string) string {
	if db == nil || id == "" || !auditIdent(table) || !auditIdent(pkCol) {
		return "{}"
	}
	rows, err := db.Query(`SELECT * FROM `+table+` WHERE `+pkCol+`=? LIMIT 1`, id)
	if err != nil {
		return "{}"
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil || !rows.Next() {
		return "{}"
	}
	raw := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range raw {
		ptrs[i] = &raw[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return "{}"
	}
	m := map[string]any{}
	for i, c := range cols {
		if c == "password_hash" {
			m[c] = "***"
			continue
		}
		m[c] = sqlVal(raw[i])
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func sqlVal(v any) any {
	switch t := v.(type) {
	case nil:
		return nil
	case []byte:
		return string(t)
	default:
		return t
	}
}

func auditIdent(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func logCreate(db *sql.DB, table, pk, id, label string) {
	logCreateWithReason(db, table, pk, id, label, "")
}

func logCreateWithReason(db *sql.DB, table, pk, id, label, reason string) {
	audit.Use(db)
	audit.LogWithReason(audit.ActionCreate, table, pk, id, label, "", rowJSON(db, table, pk, id), reason)
}

func logUpdate(db *sql.DB, table, pk, id, label, before string) {
	logUpdateWithReason(db, table, pk, id, label, before, "")
}

func logUpdateWithReason(db *sql.DB, table, pk, id, label, before, reason string) {
	audit.Use(db)
	audit.LogWithReason(audit.ActionUpdate, table, pk, id, label, before, rowJSON(db, table, pk, id), reason)
}

func logDelete(db *sql.DB, table, pk, id, label, before string) {
	audit.Use(db)
	audit.Log(audit.ActionDelete, table, pk, id, label, before, "")
}

func touchUpdate(db *sql.DB, table, pk, id, label string, fn func() error) error {
	before := rowJSON(db, table, pk, id)
	if err := fn(); err != nil {
		return err
	}
	if strings.TrimSpace(before) == "{}" && strings.TrimSpace(rowJSON(db, table, pk, id)) == "{}" {
		return nil
	}
	logUpdate(db, table, pk, id, label, before)
	return nil
}

func touchDelete(db *sql.DB, table, pk, id, label string, fn func() error) error {
	before := rowJSON(db, table, pk, id)
	if err := fn(); err != nil {
		return err
	}
	logDelete(db, table, pk, id, label, before)
	return nil
}

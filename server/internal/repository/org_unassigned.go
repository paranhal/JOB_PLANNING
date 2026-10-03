package repository

import (
	"database/sql"
	"fmt"
)

type OrgEmptyCount struct {
	Table string
	Count int
}

func ListEmptyOrgRows(db *sql.DB) ([]OrgEmptyCount, error) {
	if db == nil {
		return nil, fmt.Errorf("db")
	}
	out := make([]OrgEmptyCount, 0, len(orgRootTables)+1)
	for _, t := range orgRootTables {
		if !tableHasColumn(db, t, "org_id") {
			continue
		}
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + t + ` WHERE org_id IS NULL OR TRIM(org_id)=''`).Scan(&n); err != nil {
			return out, err
		}
		out = append(out, OrgEmptyCount{Table: t, Count: n})
	}
	if tableHasColumn(db, "users", "org_id") {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM users
			WHERE role <> 'vision_admin' AND (org_id IS NULL OR TRIM(org_id)='')`).Scan(&n); err != nil {
			return out, err
		}
		out = append(out, OrgEmptyCount{Table: "users", Count: n})
	}
	return out, nil
}

func AssignEmptyOrgToO01(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("db")
	}
	for _, t := range orgRootTables {
		if !tableHasColumn(db, t, "org_id") {
			continue
		}
		if _, err := db.Exec(`UPDATE ` + t + ` SET org_id='O01' WHERE org_id IS NULL OR TRIM(org_id)=''`); err != nil {
			return err
		}
	}
	if tableHasColumn(db, "users", "org_id") {
		if _, err := db.Exec(`UPDATE users SET org_id='O01'
			WHERE role <> 'vision_admin' AND (org_id IS NULL OR TRIM(org_id)='')`); err != nil {
			return err
		}
	}
	return nil
}

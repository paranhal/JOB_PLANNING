package repository

import (
	"database/sql"
	"fmt"

	"customer-support/internal/audit"
)

func applyIsTestColumns(db *sql.DB) {
	if db == nil {
		return
	}
	tables := append([]string{}, orgRootTables...)
	tables = append(tables, "maintenance_visits", "as_work_items")
	for _, t := range tables {
		addNamedColumn(db, t, "is_test",
			fmt.Sprintf(`ALTER TABLE %s ADD COLUMN is_test INTEGER DEFAULT 0`, t))
	}
	addNamedColumn(db, "users", "base_role",
		`ALTER TABLE users ADD COLUMN base_role TEXT DEFAULT ''`)
}

func applyUserProfileColumns(db *sql.DB) {
	if db == nil {
		return
	}
	addNamedColumn(db, "users", "org_id",
		`ALTER TABLE users ADD COLUMN org_id TEXT DEFAULT ''`)
	addNamedColumn(db, "users", "base_role",
		`ALTER TABLE users ADD COLUMN base_role TEXT DEFAULT ''`)
	addNamedColumn(db, "users", "mobile",
		`ALTER TABLE users ADD COLUMN mobile TEXT DEFAULT ''`)
	addNamedColumn(db, "users", "tel",
		`ALTER TABLE users ADD COLUMN tel TEXT DEFAULT ''`)
	addNamedColumn(db, "users", "email",
		`ALTER TABLE users ADD COLUMN email TEXT DEFAULT ''`)
	addNamedColumn(db, "users", "signature_path",
		`ALTER TABLE users ADD COLUMN signature_path TEXT DEFAULT ''`)
	addNamedColumn(db, "users", "profile_done",
		`ALTER TABLE users ADD COLUMN profile_done INTEGER DEFAULT 0`)
	addNamedColumn(db, "users", "username_changed_at",
		`ALTER TABLE users ADD COLUMN username_changed_at DATETIME`)
	addNamedColumn(db, "users", "is_test",
		`ALTER TABLE users ADD COLUMN is_test INTEGER DEFAULT 0`)
}

func stampIsTest() int {
	if audit.Current().IsTest {
		return 1
	}
	return 0
}

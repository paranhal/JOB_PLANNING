package repository

import (
	"database/sql"
	"fmt"

	"customer-support/internal/audit"
	"customer-support/internal/model"
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

func stampIsTest() int {
	if model.NormalizeRole(audit.Current().Role) == model.RoleTester {
		return 1
	}
	return 0
}

package repository

import (
	"database/sql"
	"strings"

	"customer-support/internal/audit"
)

var createdByTables = []string{
	"as_receipts", "as_processes", "work_tasks", "work_other",
	"sales_projects", "sales_activities", "sales_quotes", "customers", "assets",
}

func applyCreatedByColumns(db *sql.DB) {
	if db == nil {
		return
	}
	for _, t := range createdByTables {
		addNamedColumn(db, t, "created_by_user_id",
			"ALTER TABLE "+t+" ADD COLUMN created_by_user_id TEXT DEFAULT ''")
		addNamedColumn(db, t, "created_by_name",
			"ALTER TABLE "+t+" ADD COLUMN created_by_name TEXT DEFAULT ''")
	}
}

func stampCreatedBy() (userID, name string) {
	a := audit.Current()
	userID = strings.TrimSpace(a.UserID)
	name = strings.TrimSpace(a.Name)
	if name == "" {
		name = strings.TrimSpace(a.Username)
	}
	if userID == "" && name == "" {
		return "", "시스템"
	}
	if name == "" {
		name = "시스템"
	}
	return userID, name
}

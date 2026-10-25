package repository

import "database/sql"

func applySalesGroupChildrenV92(db *sql.DB) {
	if db == nil {
		return
	}
	addTextColumnDefault(db, "sales_groups", "child_name_template", "")
	addTextColumnDefault(db, "sales_groups", "child_defaults_json", "{}")
	addIntColumnDefault(db, "sales_groups", "auto_close", 1)
}

func ApplySalesGroupChildrenV92ForTest(db *sql.DB) { applySalesGroupChildrenV92(db) }

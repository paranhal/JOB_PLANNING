package repository

import (
	"database/sql"
	"fmt"
	"log"

	"customer-support/internal/model"
)

// orgRootTables 뿌리 17개. 여기에만 org_id 를 붙인다 (§52.3).
var orgRootTables = []string{
	"customers",
	"assets",
	"work_projects",
	"work_tasks",
	"work_other",
	"as_receipts",
	"maintenance_plans",
	"maintenance_site_config",
	"sales_projects",
	"sales_quotes",
	"sales_orders",
	"sales_items",
	"sales_groups",
	"holidays",
	"staff_leaves",
	"weekly_report_rows",
	"labor_rates",
}

// orgSharedTables 전사 공용. org_id 를 붙이지 않는다 (§52.3).
var orgSharedTables = []string{
	"codes",
	"as_cause_categories",
	"as_keywords",
	"app_settings",
	"app_versions",
	"schema_migrations",
	"id_sequences",
}

func applyOrgs(db *sql.DB) {
	if db == nil {
		return
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS orgs (
			org_id      TEXT PRIMARY KEY,
			org_no      TEXT UNIQUE,
			org_name    TEXT NOT NULL,
			short_name  TEXT DEFAULT '',
			is_active   INTEGER NOT NULL DEFAULT 1,
			sort_order  INTEGER DEFAULT 0,
			note        TEXT DEFAULT '',
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
			created_by  TEXT DEFAULT ''
		)`); err != nil {
		log.Printf("orgs: %v", err)
		return
	}
	for _, t := range orgRootTables {
		addNamedColumn(db, t, "org_id",
			fmt.Sprintf(`ALTER TABLE %s ADD COLUMN org_id TEXT DEFAULT ''`, t))
	}
	addNamedColumn(db, "users", "org_id",
		`ALTER TABLE users ADD COLUMN org_id TEXT DEFAULT ''`)
	applyOrgSplitV266(db)
}

func applyOrgSplitV266(db *sql.DB) {
	if metaDone(db, orgSplitMetaKey) {
		return
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,sort_order)
		VALUES(?,?,?,1)`, model.OrgIDLibrary, model.OrgNoLibrary, model.OrgNameLibrary); err != nil {
		log.Printf("org_split seed: %v", err)
		return
	}
	for _, t := range orgRootTables {
		if !tableHasColumn(db, t, "org_id") {
			log.Printf("org_split: %s.org_id 없음", t)
			return
		}
		if _, err := db.Exec(`UPDATE ` + t + ` SET org_id='O01'
			WHERE org_id IS NULL OR TRIM(org_id)=''`); err != nil {
			log.Printf("org_split %s: %v", t, err)
			return
		}
	}
	if _, err := db.Exec(`UPDATE users SET org_id='O01'
		WHERE role <> 'vision_admin' AND (org_id IS NULL OR TRIM(org_id)='')`); err != nil {
		log.Printf("org_split users: %v", err)
		return
	}
	markMetaDone(db, orgSplitMetaKey)
}

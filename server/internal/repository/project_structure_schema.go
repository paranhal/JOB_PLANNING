package repository

import (
	"database/sql"
	"log"
)

func applyPackagePlansV109(db *sql.DB) {
	if db == nil {
		return
	}
	up, err := leaveLedgerSQL("109_package_plans.sql")
	if err != nil {
		log.Printf("109 package_plans: %v", err)
		return
	}
	if err := execWorkGroupSQLIgnoreDupCol(db, up); err != nil {
		log.Printf("109 package_plans apply: %v", err)
	}
	addNamedColumn(db, "work_tasks", "package_id",
		`ALTER TABLE work_tasks ADD COLUMN package_id TEXT NOT NULL DEFAULT ''`)
	addNamedColumn(db, "work_tasks", "plan_id",
		`ALTER TABLE work_tasks ADD COLUMN plan_id TEXT NOT NULL DEFAULT ''`)
	if tableHasColumn(db, "work_tasks", "plan_id") {
		if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_work_tasks_plan ON work_tasks(plan_id)`); err != nil {
			log.Printf("109 idx_work_tasks_plan: %v", err)
		}
	}
	if tableHasColumn(db, "work_tasks", "package_id") {
		if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_work_tasks_package ON work_tasks(package_id)`); err != nil {
			log.Printf("109 idx_work_tasks_package: %v", err)
		}
	}
}

func applyPackageEventsV110(db *sql.DB) {
	if db == nil {
		return
	}
	up, err := leaveLedgerSQL("110_package_events.sql")
	if err != nil {
		log.Printf("110 package_events: %v", err)
		return
	}
	if err := execWorkGroupSQLIgnoreDupCol(db, up); err != nil {
		log.Printf("110 package_events apply: %v", err)
	}
}

func applyStructureTemplatesV112(db *sql.DB) {
	if db == nil {
		return
	}
	up, err := leaveLedgerSQL("112_structure_templates.sql")
	if err != nil {
		log.Printf("112 structure_templates: %v", err)
		return
	}
	if err := execWorkGroupSQLIgnoreDupCol(db, up); err != nil {
		log.Printf("112 structure_templates apply: %v", err)
	}
}

func applyPackageMilestonesV111(db *sql.DB) {
	if db == nil {
		return
	}
	up, err := leaveLedgerSQL("111_package_milestones.sql")
	if err != nil {
		log.Printf("111 package_milestones: %v", err)
		return
	}
	if err := execWorkGroupSQLIgnoreDupCol(db, up); err != nil {
		log.Printf("111 package_milestones apply: %v", err)
	}
}

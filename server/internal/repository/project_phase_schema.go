package repository

import (
	"database/sql"
	"log"
	"strings"
)

func applyProjectPhasesV108(db *sql.DB) {
	if db == nil {
		return
	}
	up, err := leaveLedgerSQL("108_project_phases.sql")
	if err != nil {
		log.Printf("108 project_phases: %v", err)
		return
	}
	if err := execWorkGroupSQLIgnoreDupCol(db, up); err != nil {
		log.Printf("108 project_phases apply: %v", err)
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='project_merge_moves'`).Scan(&n)
	if n > 0 {
		addNamedColumn(db, "project_merge_moves", "payload",
			`ALTER TABLE project_merge_moves ADD COLUMN payload TEXT NOT NULL DEFAULT ''`)
	}
}

func projectPhaseSQL(name string) (string, error) {
	return leaveLedgerSQL(name)
}

func execSQLIgnoreMissing(db *sql.DB, script string) error {
	if err := execWorkGroupSQLIgnoreDupCol(db, script); err != nil {
		low := strings.ToLower(err.Error())
		if strings.Contains(low, "no such table") || strings.Contains(low, "no such column") {
			return nil
		}
		return err
	}
	return nil
}

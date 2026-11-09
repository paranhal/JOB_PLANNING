package repository

import (
	"database/sql"
	"fmt"
	"log"
	"os"
)

func applyLeaveLedgerV107(db *sql.DB) {
	if db == nil {
		return
	}
	up, err := leaveLedgerSQL("107_leave_ledger.sql")
	if err != nil {
		log.Printf("107 leave_ledger: %v", err)
		return
	}
	if err := execWorkGroupSQLIgnoreDupCol(db, up); err != nil {
		log.Printf("107 leave_ledger apply: %v", err)
	}
	addNamedColumn(db, "employee_profiles", "leave_auto_enabled",
		`ALTER TABLE employee_profiles ADD COLUMN leave_auto_enabled INTEGER NOT NULL DEFAULT 0`)
	addNamedColumn(db, "leave_requests", "absence_consent",
		`ALTER TABLE leave_requests ADD COLUMN absence_consent INTEGER NOT NULL DEFAULT 0`)
	addNamedColumn(db, "leave_requests", "deduct_minutes",
		`ALTER TABLE leave_requests ADD COLUMN deduct_minutes INTEGER NOT NULL DEFAULT 0`)
	addNamedColumn(db, "leave_requests", "absence_minutes",
		`ALTER TABLE leave_requests ADD COLUMN absence_minutes INTEGER NOT NULL DEFAULT 0`)
}

func leaveLedgerSQL(name string) (string, error) {
	for _, p := range workGroupSQLPaths(name) {
		b, err := os.ReadFile(p)
		if err == nil && len(b) > 0 {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("%s 없음", name)
}

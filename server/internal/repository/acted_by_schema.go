package repository

import (
	"database/sql"
	"strings"
)

func ApplyActedByV85ForTest(db *sql.DB) { applyActedByV85(db) }

func applyActedByV85(db *sql.DB) {
	if db == nil {
		return
	}
	for _, table := range []string{"work_tasks", "sales_activities", "maintenance_visits"} {
		addTextColumn(db, table, "acted_by_user_id")
		addTextColumn(db, table, "acted_by_name")
		if _, err := db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN on_behalf INTEGER NOT NULL DEFAULT 0`); err != nil &&
			!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			// 기존 DB는 열 있음
		}
	}
}

func stampActedByRow(db *sql.DB, table, idCol, id, uid, name string, onBehalf bool) error {
	if db == nil || strings.TrimSpace(id) == "" {
		return nil
	}
	flag := 0
	if onBehalf {
		flag = 1
	}
	_, err := db.Exec(`UPDATE `+table+` SET acted_by_user_id=?, acted_by_name=?, on_behalf=? WHERE `+idCol+`=?`,
		uid, name, flag, id)
	return err
}

func (r *WBRepo) StampActedBy(taskID, uid, name string, onBehalf bool) error {
	if r == nil {
		return nil
	}
	return stampActedByRow(r.db, "work_tasks", "task_id", taskID, uid, name, onBehalf)
}

func (r *SalesRepo) StampActivityActedBy(activityID, uid, name string, onBehalf bool) error {
	if r == nil {
		return nil
	}
	return stampActedByRow(r.db, "sales_activities", "activity_id", activityID, uid, name, onBehalf)
}

func (r *MaintenanceRepo) StampVisitActedBy(visitID, uid, name string, onBehalf bool) error {
	if r == nil {
		return nil
	}
	return stampActedByRow(r.db, "maintenance_visits", "visit_id", visitID, uid, name, onBehalf)
}

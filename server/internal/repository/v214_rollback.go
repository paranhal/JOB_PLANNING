package repository

import (
	"database/sql"
	"log"
	"strings"
)

const v214ProjectKindRollbackMeta = "__meta:v214_project_kind_rollback_v1"

// applyV214ProjectKindRollback §22.1.1 v2.15 — work_projects.project_kind 와
// 영업 확장 컬럼을 떼고, 코드를 지운다. 이미 없는 컬럼은 건너뛴다.
func applyV214ProjectKindRollback(db *sql.DB) {
	if db == nil || metaDone(db, v214ProjectKindRollbackMeta) {
		return
	}
	if _, err := db.Exec(`DELETE FROM codes WHERE code_group IN (
		'project_kind','weekly_ref_project_kind','sales_stage','sales_lead_source')`); err != nil {
		log.Printf("v2.14 rollback codes: %v", err)
	}
	cols := []string{
		"project_kind",
		"sales_stage", "expected_ym", "expected_precision", "expected_note", "expected_undated_reason",
		"prospect_name", "prospect_region",
		"prospect_contact_name", "prospect_contact_title", "prospect_contact_phone", "prospect_contact_email",
		"sales_owner", "sales_owner_id", "expected_amount", "win_probability", "competitor", "lead_source",
	}
	for _, col := range cols {
		if !workProjectHasColumn(db, col) {
			continue
		}
		if _, err := db.Exec(`ALTER TABLE work_projects DROP COLUMN ` + col); err != nil &&
			!strings.Contains(strings.ToLower(err.Error()), "no such column") {
			log.Printf("v2.14 rollback drop %s: %v", col, err)
		}
	}
	if _, err := db.Exec(`DELETE FROM work_actions WHERE task_id IN (
		SELECT task_id FROM work_tasks WHERE source_type='project')`); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "no such table") {
		log.Printf("v2.14 rollback work_actions: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM work_activities WHERE task_id IN (
		SELECT task_id FROM work_tasks WHERE source_type='project')`); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "no such table") {
		log.Printf("v2.14 rollback work_activities: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM work_tasks WHERE source_type='project'`); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "no such table") {
		log.Printf("v2.14 rollback work_tasks: %v", err)
	}
	markMetaDone(db, v214ProjectKindRollbackMeta)
}

func workProjectHasColumn(db *sql.DB, name string) bool {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('work_projects') WHERE name=?`, name).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

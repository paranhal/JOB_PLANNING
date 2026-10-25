package repository

import (
	"database/sql"
	"log"
	"strings"
)

func applyAssigneePickerV91(db *sql.DB) {
	if db == nil {
		return
	}
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS as_receipt_assignees (
			as_id TEXT NOT NULL,
			user_id TEXT NOT NULL DEFAULT '',
			assignee_name TEXT NOT NULL DEFAULT '',
			role TEXT NOT NULL DEFAULT 'owner',
			work_group_id TEXT,
			sort_order INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT (datetime('now','localtime'))
		);
		CREATE UNIQUE INDEX IF NOT EXISTS uq_as_receipt_assignees_user
			ON as_receipt_assignees(as_id, user_id) WHERE user_id <> '';
		CREATE INDEX IF NOT EXISTS idx_as_receipt_assignees_user ON as_receipt_assignees(user_id);
		CREATE INDEX IF NOT EXISTS idx_as_receipt_assignees_as ON as_receipt_assignees(as_id, sort_order);

		CREATE TABLE IF NOT EXISTS as_work_item_assignees (
			work_id TEXT NOT NULL,
			user_id TEXT NOT NULL DEFAULT '',
			assignee_name TEXT NOT NULL DEFAULT '',
			role TEXT NOT NULL DEFAULT 'owner',
			work_group_id TEXT,
			sort_order INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT (datetime('now','localtime'))
		);
		CREATE UNIQUE INDEX IF NOT EXISTS uq_as_work_item_assignees_user
			ON as_work_item_assignees(work_id, user_id) WHERE user_id <> '';
		CREATE INDEX IF NOT EXISTS idx_as_work_item_assignees_user ON as_work_item_assignees(user_id);
		CREATE INDEX IF NOT EXISTS idx_as_work_item_assignees_work ON as_work_item_assignees(work_id, sort_order);
	`)
	if err != nil {
		log.Printf("091 assignee picker tables: %v", err)
	}
	addTextColumnDefault(db, "work_task_members", "work_group_id", "")
	backfillASReceiptAssignees(db)
	backfillASWorkItemAssignees(db)
}

func ApplyAssigneePickerV91ForTest(db *sql.DB) { applyAssigneePickerV91(db) }

func backfillASReceiptAssignees(db *sql.DB) {
	if db == nil {
		return
	}
	_, err := db.Exec(`
		INSERT INTO as_receipt_assignees (as_id, user_id, assignee_name, role, sort_order)
		SELECT as_id, COALESCE(assigned_user_id,''), COALESCE(assigned_to,''), 'owner', 0
		  FROM as_receipts
		 WHERE (COALESCE(assigned_to,'') <> '' OR COALESCE(assigned_user_id,'') <> '')
		   AND NOT EXISTS (SELECT 1 FROM as_receipt_assignees a WHERE a.as_id = as_receipts.as_id)`)
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "no such table") {
		log.Printf("091 backfill as_receipt_assignees: %v", err)
	}
}

func backfillASWorkItemAssignees(db *sql.DB) {
	if db == nil {
		return
	}
	_, err := db.Exec(`
		INSERT INTO as_work_item_assignees (work_id, user_id, assignee_name, role, sort_order)
		SELECT work_id, COALESCE(assigned_user_id,''), COALESCE(assigned_to,''), 'owner', 0
		  FROM as_work_items
		 WHERE (COALESCE(assigned_to,'') <> '' OR COALESCE(assigned_user_id,'') <> '')
		   AND NOT EXISTS (SELECT 1 FROM as_work_item_assignees a WHERE a.work_id = as_work_items.work_id)`)
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "no such table") {
		log.Printf("091 backfill as_work_item_assignees: %v", err)
	}
}

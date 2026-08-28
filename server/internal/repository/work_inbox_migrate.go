package repository

import (
	"database/sql"
	"log"
	"strings"
)

const inboxToWaitingMetaKey = "__meta:inbox_to_waiting_v1"

// applyInboxToWaiting 마이그레이션 024. §33.5.1 — inbox 상태를 할 일(waiting)로 옮긴다.
func applyInboxToWaiting(db *sql.DB) {
	if db == nil {
		return
	}
	if _, err := db.Exec(`UPDATE work_tasks SET status='waiting' WHERE status='inbox'`); err != nil &&
		!strings.Contains(err.Error(), "no such table") {
		log.Printf("024 inbox→waiting: %v", err)
		return
	}
	markMetaDone(db, inboxToWaitingMetaKey)
}

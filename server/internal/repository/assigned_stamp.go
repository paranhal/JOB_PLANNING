package repository

import (
	"database/sql"
	"strings"
	"time"

	"customer-support/internal/audit"
)

func assigneeIdentityChanged(oldName, oldUID, newName, newUID string) bool {
	oldName, oldUID = strings.TrimSpace(oldName), strings.TrimSpace(oldUID)
	newName, newUID = strings.TrimSpace(newName), strings.TrimSpace(newUID)
	if newName == "" && newUID == "" {
		return false
	}
	if oldName == "" && oldUID == "" {
		return true
	}
	if oldUID != "" && newUID != "" {
		return oldUID != newUID
	}
	if oldName != "" && newName != "" && oldName != newName {
		return true
	}
	if oldUID != newUID && (oldUID == "" || newUID == "") {
		return oldName != newName || oldUID != newUID
	}
	return false
}

func stampAssigned(db *sql.DB, table, idCol, id string) {
	if db == nil || strings.TrimSpace(table) == "" || strings.TrimSpace(id) == "" {
		return
	}
	by := strings.TrimSpace(audit.Current().Name)
	_, _ = db.Exec(`UPDATE "`+table+`" SET assigned_at=?, assigned_by=? WHERE "`+idCol+`"=?`,
		time.Now().Format("2006-01-02 15:04:05"), by, id)
}

func stampAssignedIfChanged(db *sql.DB, table, idCol, id, oldName, oldUID, newName, newUID string) {
	if assigneeIdentityChanged(oldName, oldUID, newName, newUID) {
		stampAssigned(db, table, idCol, id)
	}
}

func readAssigneePair(db *sql.DB, table, idCol, id, nameCol, uidCol string) (name, uid string) {
	if db == nil || id == "" {
		return "", ""
	}
	_ = db.QueryRow(`SELECT TRIM(COALESCE("`+nameCol+`",'')), TRIM(COALESCE("`+uidCol+`",'')) FROM "`+table+`" WHERE "`+idCol+`"=?`, id).Scan(&name, &uid)
	return name, uid
}

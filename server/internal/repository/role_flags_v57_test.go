package repository

import (
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/audit"
)

func TestMigrateRoleFlagsV57WritesSystemLog(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "v57-log.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO users (user_id, username, password_hash, full_name, role, is_active, created_at)
		VALUES ('USRobs','legacyobs','x','옛옵저버','observer',1,'2026-01-01 00:00:00')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM id_sequences WHERE seq_key=?`, roleFlagsV57MetaKey); err != nil {
		t.Fatal(err)
	}
	if err := migrateRoleFlagsV57(db); err != nil {
		t.Fatal(err)
	}
	var role string
	var ro int
	if err := db.QueryRow(`SELECT role, is_readonly FROM users WHERE username='legacyobs'`).Scan(&role, &ro); err != nil {
		t.Fatal(err)
	}
	if role != "org_admin" || ro != 1 {
		t.Fatalf("role=%s ro=%d", role, ro)
	}
	logs, err := audit.ListLogs(20)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, l := range logs {
		if l.UserName == "시스템" && strings.Contains(l.Summary, "옵저버") {
			found = true
		}
	}
	if !found {
		t.Fatal("시스템 전환 로그가 없다")
	}
}

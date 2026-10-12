package repository

import (
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func TestApprovalMigrationRoundTripAndTriggers(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "apr.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='approval_requests'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("table n=%d err=%v", n, err)
	}
	if _, err := db.Exec(`INSERT INTO approval_events(event_id, request_id, event_type, actor_id) VALUES ('e1','r1','submit','u1')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE approval_events SET event_type='x' WHERE event_id='e1'`); err == nil {
		t.Fatal("append-only update")
	}
	if _, err := db.Exec(`DELETE FROM approval_events WHERE event_id='e1'`); err == nil {
		t.Fatal("append-only delete")
	}
	users := NewUserRepo(db)
	if err := users.EnsureAdmin("x"); err != nil {
		t.Fatal(err)
	}
	var adminID string
	_ = db.QueryRow(`SELECT user_id FROM users WHERE username='admin'`).Scan(&adminID)
	if _, err := db.Exec(`INSERT INTO employee_profiles(user_id, org_id, is_org_head) VALUES (?,?,1)`, adminID, "O01"); err != nil {
		t.Fatal(err)
	}
	tech := &struct{ ID string }{}
	if err := users.Create(&model.User{Username: "tech1", PasswordHash: "x", FullName: "기술", Role: model.RoleTech, IsActive: true, OrgID: "O01"}); err != nil {
		t.Fatal(err)
	}
	_ = db.QueryRow(`SELECT user_id FROM users WHERE username='tech1'`).Scan(&tech.ID)
	if _, err := db.Exec(`INSERT INTO employee_profiles(user_id, org_id, is_org_head) VALUES (?,?,1)`, tech.ID, "O01"); err == nil {
		t.Fatal("tech cannot be head")
	}
	if _, err := db.Exec(`INSERT INTO approval_delegations(delegation_id, org_id, from_user_id, to_user_id, start_date, end_date, source, created_by)
		VALUES ('d1','O01',?,?, '2026-01-01','2026-01-10','head','u')`, adminID, tech.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO approval_delegations(delegation_id, org_id, from_user_id, to_user_id, start_date, end_date, source, created_by)
		VALUES ('d2','O01',?,?, '2026-01-05','2026-01-12','admin','u')`, adminID, tech.ID); err == nil {
		t.Fatal("overlap")
	}
	_, down, err := approvalMigrationFiles()
	if err != nil {
		t.Fatal(err)
	}
	if err := execSQLScript(db, down); err != nil {
		t.Fatal(err)
	}
	applyApprovalEngineV71(db)
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='approval_policies'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("after roundtrip n=%d err=%v", n, err)
	}
}

func TestApprovalSQLFilesExist(t *testing.T) {
	up, down, err := approvalMigrationFiles()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(up, "approval_events") || !strings.Contains(down, "DROP TABLE IF EXISTS approval_events") {
		t.Fatal("058 files")
	}
}

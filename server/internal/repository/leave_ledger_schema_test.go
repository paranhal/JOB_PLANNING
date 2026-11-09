package repository

import (
	"path/filepath"
	"strings"
	"testing"
)

func Test95_161_LeaveLedgerMigrationRoundTrip(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "ll.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='leave_grants'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("grants n=%d err=%v", n, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='leave_consumptions'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("cons")
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='leave_absence_records'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("abs")
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM leave_types WHERE type_key='military_training'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("military %d", n)
	}
	if !tableHasColumn(db, "employee_profiles", "leave_auto_enabled") {
		t.Fatal("leave_auto_enabled")
	}
	if _, err := db.Exec(`INSERT INTO leave_grants(grant_id,user_id,grant_key,minutes,granted_on,expires_on,source)
		VALUES ('g1','u1','F:2026',480,'2026-01-01','2026-12-31','auto_fiscal')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE leave_grants SET minutes=1 WHERE grant_id='g1'`); err == nil {
		t.Fatal("grants update")
	}
	if _, err := db.Exec(`DELETE FROM leave_grants WHERE grant_id='g1'`); err == nil {
		t.Fatal("grants delete")
	}
	if _, err := db.Exec(`INSERT INTO leave_consumptions(cons_id,user_id,grant_id,minutes,kind) VALUES ('c1','u1','g1',480,'use')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO leave_consumptions(cons_id,user_id,grant_id,minutes,kind) VALUES ('c2','u1','g1',1,'use')`); err == nil {
		t.Fatal("over-consume")
	}
	down, err := leaveLedgerSQL("107_leave_ledger.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := execWorkGroupSQL(db, down); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='leave_grants'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("after down n=%d", n)
	}
	applyLeaveLedgerV107(db)
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='leave_grants'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("reup n=%d", n)
	}
	up, err := leaveLedgerSQL("107_leave_ledger.sql")
	if err != nil || !strings.Contains(up, "leave_absence_records") {
		t.Fatal("up file")
	}
}

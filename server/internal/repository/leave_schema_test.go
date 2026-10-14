package repository

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLeaveMigrationRoundTrip(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "lv.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='leave_requests'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("table n=%d err=%v", n, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM leave_types`).Scan(&n); err != nil || n < 8 {
		t.Fatalf("types %d", n)
	}
	if _, err := db.Exec(`INSERT INTO leave_requests(leave_req_id,org_id,user_id,doc_no,type_key,period_mode,start_date,end_date,work_minutes,reason,status)
		VALUES ('LR1','O01','u1','VI-근태-260101-001','annual','days','2026-01-02','2026-01-02',480,'사유','pending')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE leave_requests SET reason='바꿈' WHERE leave_req_id='LR1'`); err == nil {
		t.Fatal("frozen")
	}
	_, down, err := leaveMigrationFiles()
	if err != nil {
		t.Fatal(err)
	}
	if err := execSQLScript(db, down); err != nil {
		t.Fatal(err)
	}
	applyLeaveRequestsV72(db)
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='leave_policies'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("after n=%d err=%v", n, err)
	}
}

func TestLeaveSQLFilesExist(t *testing.T) {
	up, down, err := leaveMigrationFiles()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(up, "leave_requests") || !strings.Contains(down, "DROP TABLE IF EXISTS leave_requests") {
		t.Fatal("059 files")
	}
}

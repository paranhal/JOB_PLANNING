package audit_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/audit"
	"customer-support/internal/repository"
)

func TestRollbackUpdateAndDelete(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	audit.Init(db)
	pop := audit.Push(audit.Actor{UserID: "U1", Username: "admin", Name: "관리자"})
	defer pop()

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('C1','원래이름','원래이름',1)`); err != nil {
		t.Fatal(err)
	}
	audit.Log(audit.ActionUpdate, "customers", "customer_id", "C1", "고객",
		`{"customer_id":"C1","org_name":"원래이름","official_name":"원래이름","is_active":1}`,
		`{"customer_id":"C1","org_name":"바꾼이름","official_name":"바꾼이름","is_active":1}`)
	if _, err := db.Exec(`UPDATE customers SET org_name='바꾼이름', official_name='바꾼이름' WHERE customer_id='C1'`); err != nil {
		t.Fatal(err)
	}

	logs, err := audit.ListLogs(10)
	if err != nil || len(logs) == 0 {
		t.Fatalf("이력: %+v err=%v", logs, err)
	}
	if err := audit.Rollback(logs[0].LogID); err != nil {
		t.Fatal(err)
	}
	var name string
	if err := db.QueryRow(`SELECT org_name FROM customers WHERE customer_id='C1'`).Scan(&name); err != nil || name != "원래이름" {
		t.Fatalf("롤백 후 이름: %q err=%v", name, err)
	}

	audit.Log(audit.ActionDelete, "customers", "customer_id", "C1", "고객",
		`{"customer_id":"C1","org_name":"원래이름","official_name":"원래이름","is_active":1}`, "")
	if _, err := db.Exec(`DELETE FROM customers WHERE customer_id='C1'`); err != nil {
		t.Fatal(err)
	}
	logs, _ = audit.ListLogs(10)
	var delID string
	for _, l := range logs {
		if l.Action == audit.ActionDelete && !l.RolledBack {
			delID = l.LogID
			break
		}
	}
	if delID == "" {
		t.Fatal("삭제 이력 없음")
	}
	if err := audit.Rollback(delID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM customers WHERE customer_id='C1'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("삭제 롤백: n=%d err=%v", n, err)
	}
}

func TestArchiveLogsAndAnalyze(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	db, err := repository.InitDB(filepath.Join(dataDir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	audit.Init(db)
	pop := audit.Push(audit.Actor{Name: "관리자", Username: "admin"})
	defer pop()

	var before int
	_ = db.QueryRow(`SELECT COUNT(*) FROM data_change_logs`).Scan(&before)
	audit.Log(audit.ActionCreate, "customers", "customer_id", "C9", "테스트기관", "", `{"customer_id":"C9"}`)
	name, n, err := audit.ArchiveLogs(dataDir)
	if err != nil || n != before+1 {
		t.Fatalf("아카이브: name=%s n=%d before=%d err=%v", name, n, before, err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "backups", name, "change_logs.db")); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := db.QueryRow(`SELECT COUNT(*) FROM data_change_logs`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("초기화되지 않음: %d %v", left, err)
	}
	st := audit.AnalyzeBackup(dataDir, name)
	if st.Error != "" || !st.IsLogArchive || st.ChangeLogs != before+1 {
		t.Fatalf("분석: %+v before=%d", st, before)
	}
}

func TestDueForArchive(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	audit.Init(db)
	if audit.DueForArchive() {
		t.Fatal("빈 테이블인데 대상이면 안 됨")
	}
	old := time.Now().AddDate(0, -4, 0).Format("2006-01-02 15:04:05")
	if _, err := db.Exec(`INSERT INTO data_change_logs (log_id, occurred_at, action, table_name, entity_id, before_json, after_json, rolled_back)
		VALUES ('Lold', ?, 'update', 'customers', 'C1', '{}', '{}', 0)`, old); err != nil {
		t.Fatal(err)
	}
	if !audit.DueForArchive() {
		t.Fatal("4개월 전 이력이면 대상이어야 함")
	}
}

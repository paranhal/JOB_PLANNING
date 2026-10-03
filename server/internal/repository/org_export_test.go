package repository

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOrgExportPurgeRestoreAndSplit(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "org.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r := NewOrgRepo(db)
	if _, err := r.Create("다른팀", "이팀", "", "t"); err != nil {
		t.Fatal(err)
	}
	must := func(q string, args ...interface{}) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	must(`INSERT INTO customers (customer_id, org_name, official_name, org_id) VALUES ('C-KEEP','남길고객','남길고객','O01')`)
	must(`INSERT INTO customers (customer_id, org_name, official_name, org_id) VALUES ('C-MOVE','옮길고객','옮길고객','O02')`)
	must(`INSERT INTO as_receipts (as_id, as_number, customer_id, symptom, org_id) VALUES ('AS1','AS-1','C-MOVE','장애','O02')`)
	must(`INSERT INTO as_processes (process_id, as_id, work_content, time_spent) VALUES ('P1','AS1','조치',30)`)
	must(`INSERT INTO sales_projects (sales_id, name, org_id, customer_id) VALUES ('S1','사업','O02','C-MOVE')`)
	must(`INSERT INTO work_tasks (task_id, title, org_id, customer_id) VALUES ('T1','업무','O02','C-MOVE')`)

	snap := filepath.Join(dir, "snap")
	if err := os.MkdirAll(snap, 0755); err != nil {
		t.Fatal(err)
	}
	meta, err := r.WriteOrgSnapshot(snap, "O02", "org_delete")
	if err != nil {
		t.Fatal(err)
	}
	if meta.Counts.Customers != 1 || meta.Counts.AS != 1 {
		t.Fatalf("counts %+v", meta.Counts)
	}
	if _, err := os.Stat(filepath.Join(snap, "org.db")); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(filepath.Join(snap, "org.json"))
	if err != nil {
		t.Fatal(err)
	}
	var jm OrgBackupMeta
	if err := json.Unmarshal(src, &jm); err != nil || jm.OrgID != "O02" {
		t.Fatalf("json %s err=%v", src, err)
	}

	if err := r.RestoreOrg(filepath.Join(snap, "org.db")); !errors.Is(err, ErrOrgAlive) {
		t.Fatalf("restore while alive: %v", err)
	}

	if err := r.Update("O02", "다른팀", "이팀", "", false); err != nil {
		t.Fatal(err)
	}
	deleted, err := r.PurgeOrg("O02")
	if err != nil {
		t.Fatal(err)
	}
	if deleted["customers"] < 1 || deleted["as_receipts"] < 1 {
		t.Fatalf("purge counts %+v", deleted)
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM customers WHERE customer_id='C-MOVE'`).Scan(&n)
	if n != 0 {
		t.Fatal("옮긴 고객이 남아 있다")
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM as_processes WHERE process_id='P1'`).Scan(&n)
	if n != 0 {
		t.Fatal("자식 조치가 남아 있다")
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM customers WHERE customer_id='C-KEEP'`).Scan(&n)
	if n != 1 {
		t.Fatal("O01 고객이 지워졌다")
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM data_change_logs WHERE table_name='orgs' AND entity_id='O02' AND action='delete'`).Scan(&n)
	if n < 1 {
		t.Fatal("완전삭제 로그가 없다")
	}

	if err := r.RestoreOrg(filepath.Join(snap, "org.db")); err != nil {
		t.Fatal(err)
	}
	got, err := r.Get("O02")
	if err != nil || got == nil || got.OrgName != "다른팀" {
		t.Fatalf("restore org %+v err=%v", got, err)
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM customers WHERE customer_id='C-MOVE' AND org_id='O02'`).Scan(&n)
	if n != 1 {
		t.Fatal("복구 후 고객이 없다")
	}

	prev, err := r.PreviewSplit("O02", "O01", []string{"C-MOVE"})
	if err != nil {
		t.Fatal(err)
	}
	if prev.Customers != 1 || prev.AS != 1 || prev.Sales != 1 || prev.Work != 1 {
		t.Fatalf("preview %+v", prev)
	}
	if _, err := r.ApplySplit("O02", "O01", []string{"C-MOVE"}); err != nil {
		t.Fatal(err)
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM customers WHERE customer_id='C-MOVE' AND org_id='O01'`).Scan(&n)
	if n != 1 {
		t.Fatal("분리가 안 됐다")
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM as_receipts WHERE as_id='AS1' AND org_id='O01'`).Scan(&n)
	if n != 1 {
		t.Fatal("AS 조직이 안 바뀌었다")
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM users WHERE org_id='O02'`).Scan(&n)
	if n != 0 {
		t.Fatal("계정은 따라가면 안 된다")
	}
}

package repository

import (
	"path/filepath"
	"testing"
)

// 95-229 ①: 접수를 수정해도 연결된 사업(수동 연결 포함)이 지워지지 않는다. 접수 때 고른 사업은 규칙 판정보다 우선한다.
func TestASUpdateReceiptKeepsProject229(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "k229.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{
		`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,is_active) VALUES ('O03','O03','SI',1)`,
		`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id) VALUES ('C1','학교','학교',1,'O03')`,
		`INSERT INTO work_projects(project_id,name,status,org_id) VALUES ('WP1','사업A','active','O03'),('WP2','사업B','active','O03')`,
		`INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, status, org_id, symptom, created_at, updated_at) VALUES ('AS1','AS1','C1','2026-08-12 10:00:00','in_progress','O03','증상',datetime('now'),datetime('now'))`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(q, err)
		}
	}
	as := NewASRepo(db)
	pid := func() string {
		var p string
		_ = db.QueryRow(`SELECT COALESCE(project_id,'') FROM as_receipts WHERE as_id='AS1'`).Scan(&p)
		return p
	}
	if _, _, err := as.SetASReceiptProject("O03", "AS1", "WP1"); err != nil {
		t.Fatal(err)
	}
	// 조회 모델이 project_id 를 읽는다(조치 화면의 「미연결」 오표시 원인)
	rec, err := as.GetByID(OrgAll, "AS1")
	if err != nil || rec == nil || rec.ProjectID != "WP1" {
		t.Fatalf("GetByID project=%v err=%v", rec, err)
	}
	// 불러온 모델로 수정
	rec.Symptom = "증상 수정"
	if err := as.UpdateReceipt(rec); err != nil || pid() != "WP1" {
		t.Fatalf("수정 뒤 사업 %q err=%v", pid(), err)
	}
	// 폼에서 새로 만든 모델(사업 값 없음)로 수정해도 기존 사업을 유지한다
	fresh := *rec
	fresh.ProjectID = ""
	fresh.Symptom = "또 수정"
	if err := as.UpdateReceipt(&fresh); err != nil || pid() != "WP1" {
		t.Fatalf("폼 수정 뒤 사업 %q err=%v", pid(), err)
	}
	// 접수 수정 화면에서 다른 사업을 골랐으면 그 값
	fresh.ProjectID = "WP2"
	if err := as.UpdateReceipt(&fresh); err != nil || pid() != "WP2" {
		t.Fatalf("사업 변경 %q err=%v", pid(), err)
	}
}

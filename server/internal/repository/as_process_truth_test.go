package repository

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestASProcessTruth_ConflictsAndReadPreferProcess(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "p51.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if !tableHasColumn(db, "as_processes", "cause_type") {
		t.Fatal("051 as_processes.cause_type 없음")
	}
	if !tableHasColumn(db, "as_receipts", "action_taken") {
		t.Fatal("접수 action_taken 을 이 단계에서 지우면 안 된다")
	}
	if !tableHasColumn(db, "as_receipts", "complete_datetime") {
		t.Fatal("complete_datetime 을 이 단계에서 옮기면 안 된다")
	}

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c-p51','조치도서관','조치도서관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, status, symptom, data_origin,
		action_taken, cause_type, parts_used, result_code, process_type, transfer_detail
	) VALUES (
		'as-conflict','R2609-CF','c-p51','2026-09-01 10:00:00','completed','증상','app',
		'접수쪽 조치','hw','부품A','completed','visit','waiting'
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, status, symptom, data_origin,
		action_taken, cause_type, parts_used, result_code, process_type, transfer_detail
	) VALUES (
		'as-same','R2609-SM','c-p51','2026-09-01 11:00:00','completed','증상2','app',
		'같은 조치','sw','부품B','completed','remote',''
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_processes (
		process_id, as_id, process_datetime, work_type, work_content, cause_type, parts_used, result_code, transfer_detail, time_spent
	) VALUES (
		'p-cf','as-conflict','2026-09-02 09:00:00','remote','이력쪽 조치','sw','부품P','partial','completed',30
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_processes (
		process_id, as_id, process_datetime, work_type, work_content, cause_type, parts_used, result_code, time_spent
	) VALUES (
		'p-sm','as-same','2026-09-02 10:00:00','remote','같은 조치','sw','부품B','completed',30
	)`); err != nil {
		t.Fatal(err)
	}
	if err := RebuildASSearch(db); err != nil {
		t.Fatal(err)
	}

	items := ListActionConflicts(db)
	if len(items) != 1 {
		t.Fatalf("어긋남 %d건 want 1: %+v", len(items), items)
	}
	if items[0].ASNumber != "R2609-CF" || items[0].ReceiptAction != "접수쪽 조치" || items[0].ProcessAction != "이력쪽 조치" {
		t.Fatalf("어긋남 내용: %+v", items[0])
	}

	got, err := NewASRepo(db).GetByID("as-conflict")
	if err != nil || got == nil {
		t.Fatalf("GetByID: %v %#v", err, got)
	}
	if got.ActionTaken != "이력쪽 조치" {
		t.Fatalf("화면 조치=%q want 이력쪽 조치", got.ActionTaken)
	}
	if got.CauseType != "sw" {
		t.Fatalf("원인=%q want sw", got.CauseType)
	}
	if got.PartsUsed != "부품P" {
		t.Fatalf("부품=%q", got.PartsUsed)
	}
	if got.ResultCode != "partial" {
		t.Fatalf("결과=%q", got.ResultCode)
	}
	if got.ProcessType != "remote" {
		t.Fatalf("처리유형=%q", got.ProcessType)
	}
	if got.TransferDetail != "completed" {
		t.Fatalf("이관=%q", got.TransferDetail)
	}

	hist, err := NewASRepo(db).ListPastHistory("c-p51", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	var histAction string
	for _, it := range hist {
		if it.ASID == "as-conflict" {
			histAction = it.ActionTaken
		}
	}
	if histAction != "이력쪽 조치" {
		t.Fatalf("이력 목록 조치=%q", histAction)
	}

	hits, _, err := NewASRepo(db).SearchAS(model.ASSearchFilter{Query: "접수쪽", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, h := range hits {
		if h.ASID == "as-conflict" {
			found = true
			if h.Action != "이력쪽 조치" {
				t.Fatalf("검색 표시 조치=%q want 이력쪽 조치", h.Action)
			}
		}
	}
	if !found {
		t.Fatalf("검색이 접수 본문으로 건을 못 찾음: %+v", hits)
	}

	var stored string
	if err := db.QueryRow(`SELECT action_taken FROM as_receipts WHERE as_id='as-conflict'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != "접수쪽 조치" {
		t.Fatalf("접수 열을 덮어쓰면 안 된다 stored=%q", stored)
	}
}

func TestASProcessTruth_CreateStoresCauseType(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "p51c.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c-p51c','원인도서관','원인도서관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, status, symptom, data_origin
	) VALUES ('as-cause','R2609-CA','c-p51c','2026-09-01','assigned','증상','app')`); err != nil {
		t.Fatal(err)
	}
	p := &model.ASProcess{
		ASID:            "as-cause",
		WorkType:        "visit",
		CauseType:       "network",
		WorkContent:     "케이블 교체",
		ProcessDatetime: time.Date(2026, 9, 2, 11, 0, 0, 0, time.Local),
	}
	if err := NewASProcessRepo(db).Create(p); err != nil {
		t.Fatal(err)
	}
	got, err := NewASProcessRepo(db).ListByAS("as-cause")
	if err != nil || len(got) != 1 {
		t.Fatalf("ListByAS: %v n=%d", err, len(got))
	}
	if got[0].CauseType != "network" {
		t.Fatalf("cause_type=%q", got[0].CauseType)
	}
}

func TestASProcessTruth_ApplyDoesNotDropReceiptColumns(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "p51d.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	applyASProcessTruth(db)
	for _, col := range []string{
		"action_taken", "cause_type", "parts_used", "result_code", "process_type", "transfer_detail", "complete_datetime",
	} {
		if !tableHasColumn(db, "as_receipts", col) {
			t.Fatalf("지워지면 안 되는 열 as_receipts.%s", col)
		}
	}
}

func TestASLatestProcessColSQL_PrefersNonEmpty(t *testing.T) {
	expr := asActionTakenSQL("ar")
	if !strings.Contains(expr, "work_content") || !strings.Contains(expr, "action_taken") {
		t.Fatalf("expr=%s", expr)
	}
}

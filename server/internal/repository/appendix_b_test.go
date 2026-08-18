package repository

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyAppendixBSchemaAndTriggers(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "b.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	mustCol := func(table, col string) {
		t.Helper()
		rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		found := false
		for rows.Next() {
			var cid, notnull, pk int
			var name, ctype string
			var dflt interface{}
			if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
				t.Fatal(err)
			}
			if name == col {
				found = true
			}
		}
		if !found {
			t.Errorf("%s.%s 컬럼이 없다", table, col)
		}
	}
	mustCol("as_receipts", "data_origin")
	mustCol("as_receipts", "schedule_no_date_reason")
	mustCol("as_receipts", "asset_unlinked_reason")
	mustCol("maintenance_visits", "project_id")
	mustCol("maintenance_visits", "data_origin")

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='holidays'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("holidays 테이블: n=%d err=%v", n, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='ux_mnt_visit_unique'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("ux_mnt_visit_unique: n=%d err=%v", n, err)
	}

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('C1','기관','기관',1)`); err != nil {
		t.Fatal(err)
	}

	// S-6: 예정일 없이 확정 INSERT 차단
	_, err = db.Exec(`INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, status, schedule_confirmed, visit_scheduled_date)
		VALUES ('R1','R1','C1','2026-08-14 10:00:00','received',1,'')`)
	if err == nil || !strings.Contains(err.Error(), "일정확정에는 예정업무일이 필요합니다") {
		t.Fatalf("S-6 INSERT: err=%v", err)
	}

	if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, status, schedule_confirmed, visit_scheduled_date)
		VALUES ('R2','R2','C1','2026-08-14 10:00:00','received',0,'')`); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`UPDATE as_receipts SET schedule_confirmed=1 WHERE as_id='R2'`)
	if err == nil || !strings.Contains(err.Error(), "일정확정에는 예정업무일이 필요합니다") {
		t.Fatalf("S-6 UPDATE: err=%v", err)
	}
	if _, err := db.Exec(`UPDATE as_receipts SET visit_scheduled_date='2026-08-20', schedule_confirmed=1 WHERE as_id='R2'`); err != nil {
		t.Fatalf("예정일+확정 함께 UPDATE 통과해야 한다: %v", err)
	}

	// S-8: 근무구분
	_, err = db.Exec(`UPDATE as_receipts SET work_place='home' WHERE as_id='R2'`)
	if err == nil || !strings.Contains(err.Error(), "근무구분은 office 또는 field 여야 합니다") {
		t.Fatalf("S-8 home: err=%v", err)
	}
	if _, err := db.Exec(`UPDATE as_receipts SET work_place='field' WHERE as_id='R2'`); err != nil {
		t.Fatalf("S-8 field: %v", err)
	}

	// S-7: time_spent=0 INSERT 차단
	_, err = db.Exec(`INSERT INTO as_processes (process_id, as_id, process_datetime, worker, work_content, time_spent)
		VALUES ('P0','R2','2026-08-14 11:00:00','테크','조치',0)`)
	if err == nil || !strings.Contains(err.Error(), "소요시간(분)은 0보다 커야 합니다") {
		t.Fatalf("S-7 time_spent=0: err=%v", err)
	}
	if _, err := db.Exec(`INSERT INTO as_processes (process_id, as_id, process_datetime, worker, work_content, time_spent)
		VALUES ('P1','R2','2026-08-14 11:00:00','테크','조치',30)`); err != nil {
		t.Fatalf("S-7 time_spent=30: %v", err)
	}

	applyAppendixB(db) // 멱등
}

func TestAppendixBCorrectionCountsOnFixture(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "b2.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('C1','기관','기관',1)`); err != nil {
		t.Fatal(err)
	}
	// 트리거 때문에 확정+빈 예정일은 INSERT 불가. 보정 대상은 트리거 이전에 들어온 기존 행이므로
	// 트리거를 잠시 제거하고 fixture를 넣은 뒤 보정을 다시 돌린다.
	_, _ = db.Exec(`DROP TRIGGER IF EXISTS trg_as_sched_confirm_ins`)
	_, _ = db.Exec(`DROP TRIGGER IF EXISTS trg_as_sched_confirm_upd`)
	_, _ = db.Exec(`DROP TRIGGER IF EXISTS trg_proc_time_spent_ins`)

	if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, status, schedule_confirmed, visit_scheduled_date, import_key)
		VALUES
		('A1','A1','C1','2026-08-01 09:00:00','in_progress',1,'','IMP-1'),
		('A2','A2','C1','2026-08-01 09:00:00','in_progress',1,'','IMP-2'),
		('A3','A3','C1','2026-08-01 09:00:00','received',0,'','')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_processes (process_id, as_id, process_datetime, worker, work_content, time_spent)
		VALUES ('P1','A1','2026-08-02 10:00:00','테크','조치',30)`); err != nil {
		t.Fatal(err)
	}

	applyAppendixB(db)

	var origin, start string
	var confirmed int
	_ = db.QueryRow(`SELECT data_origin, COALESCE(start_datetime,''), schedule_confirmed FROM as_receipts WHERE as_id='A1'`).
		Scan(&origin, &start, &confirmed)
	if origin != "import" {
		t.Fatalf("B-3 A1 data_origin=%q", origin)
	}
	if start != "2026-08-02 10:00:00" {
		t.Fatalf("B-2 A1 start=%q", start)
	}
	if confirmed != 0 {
		t.Fatalf("B-1 A1 confirmed=%d", confirmed)
	}

	_ = db.QueryRow(`SELECT data_origin FROM as_receipts WHERE as_id='A3'`).Scan(&origin)
	if origin != "app" {
		t.Fatalf("import_key 없는 건은 app 유지: %q", origin)
	}
}

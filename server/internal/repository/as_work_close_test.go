package repository

import (
	"path/filepath"
	"testing"
	"time"
)

// 접수 완료 후 남은 확인 하부업무가 지연 목록에 남지 않아야 한다.
func TestDelayedExcludesOpenWorkUnderCompletedAS(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "work_close.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now().Format("2006-01-02 15:04:05")
	past := time.Now().AddDate(0, 0, -4).Format("2006-01-02")

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('C-W','테스트도서관','테스트도서관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, receipt_datetime, customer_id, symptom, urgency, status,
		visit_scheduled_date, schedule_confirmed, assigned_to, complete_datetime, created_at, updated_at
	) VALUES ('R2608-023','R2608-023',?,'C-W','증상','중','completed',?,1,'테크',?,?,?)`,
		now, past, now, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_work_items (
		work_id, work_number, as_id, work_kind, scheduled_date, schedule_confirmed, status, notes, created_at, updated_at
	) VALUES ('W-023','R2608-023-W01','R2608-023','confirm',?,1,'open','확인',?,?)`,
		past, now, now); err != nil {
		t.Fatal(err)
	}

	wb := NewWorkBoardRepo(db)
	delayed, err := wb.ListBucket("delayed", "", nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range delayed {
		if it.RefNumber == "R2608-023-W01" || it.RefNumber == "R2608-023" {
			t.Fatalf("완료 접수의 하부업무가 지연에 남음: %+v", it)
		}
	}

	var st string
	if err := db.QueryRow(`SELECT status FROM as_work_items WHERE work_number='R2608-023-W01'`).Scan(&st); err != nil {
		t.Fatal(err)
	}
	if st != "open" {
		t.Fatalf("조회가 as_work_items 를 닫음 status=%s", st)
	}

	RunWorkListHousekeeping(db)
	if err := db.QueryRow(`SELECT status FROM as_work_items WHERE work_number='R2608-023-W01'`).Scan(&st); err != nil {
		t.Fatal(err)
	}
	if st != "done" {
		t.Fatalf("status=%s want done", st)
	}
}

func TestASUpdateClosesOpenWorkItems(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "work_close2.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now()
	nowStr := now.Format("2006-01-02 15:04:05")
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('C-W2','기관','기관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, receipt_datetime, customer_id, symptom, urgency, status,
		visit_scheduled_date, schedule_confirmed, assigned_to, created_at, updated_at
	) VALUES ('AS-X','AS-X',?,'C-W2','증상','중','in_progress',?,1,'테크',?,?)`,
		nowStr, now.Format("2006-01-02"), nowStr, nowStr); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_work_items (
		work_id, work_number, as_id, work_kind, scheduled_date, schedule_confirmed, status, created_at, updated_at
	) VALUES ('W-X','AS-X-W01','AS-X','revisit',?,1,'open',?,?)`,
		now.AddDate(0, 0, -1).Format("2006-01-02"), nowStr, nowStr); err != nil {
		t.Fatal(err)
	}

	as, err := NewASRepo(db).GetByID("AS-X")
	if err != nil || as == nil {
		t.Fatalf("get: %v", err)
	}
	as.Status = "completed"
	complete := now
	as.CompleteDatetime = &complete
	if err := NewASRepo(db).Update(as); err != nil {
		t.Fatal(err)
	}
	var st string
	if err := db.QueryRow(`SELECT status FROM as_work_items WHERE work_id='W-X'`).Scan(&st); err != nil {
		t.Fatal(err)
	}
	if st != "done" {
		t.Fatalf("status=%s want done", st)
	}
}

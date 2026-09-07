package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestApplyAS34ActionUXMigratesProcessTypeAndSeedsCauseCats(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "ux.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('C1','가나도서관','가나도서관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, status, process_type, created_at, updated_at)
		VALUES ('R1','AS-1','C1','2026-08-01 10:00:00','in_progress','replace','2026-08-01 10:00:00','2026-08-01 10:00:00')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_processes (process_id, as_id, process_datetime, worker, work_type, time_spent)
		VALUES ('P1','R1','2026-08-01 11:00:00','테스터','config',30)`); err != nil {
		t.Fatal(err)
	}

	applyAS34ActionUX(db)

	var pt, wt string
	if err := db.QueryRow(`SELECT process_type FROM as_receipts WHERE as_id='R1'`).Scan(&pt); err != nil {
		t.Fatal(err)
	}
	if pt != model.ProcessTypeVisit {
		t.Fatalf("replace→visit 이관 실패: %q", pt)
	}
	if err := db.QueryRow(`SELECT work_type FROM as_processes WHERE process_id='P1'`).Scan(&wt); err != nil {
		t.Fatal(err)
	}
	if wt != model.ProcessTypeVisit {
		t.Fatalf("config→visit 이관 실패: %q", wt)
	}

	var inactive int
	if err := db.QueryRow(`SELECT COUNT(*) FROM codes WHERE code_group='process_type' AND code_value IN ('replace','config') AND is_active=0`).Scan(&inactive); err != nil {
		t.Fatal(err)
	}
	if inactive != 2 {
		t.Fatalf("process_type 비활성 %d", inactive)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM codes WHERE code_group='result_code' AND code_value IN ('revisit_needed','temporary','escalation') AND is_active=0`).Scan(&inactive); err != nil {
		t.Fatal(err)
	}
	if inactive != 3 {
		t.Fatalf("result_code 비활성 %d", inactive)
	}

	cats, err := NewASRepo(db).ListCauseCategories()
	if err != nil {
		t.Fatal(err)
	}
	if len(cats) < 55 {
		t.Fatalf("시드 %d건 (55행 필요)", len(cats))
	}
	l1 := 0
	seen := map[string]bool{}
	for _, c := range cats {
		if c.Level == 1 {
			l1++
			seen[c.Code] = true
		}
	}
	if l1 != 4 {
		t.Fatalf("1차 분류 %d건", l1)
	}
	for _, code := range []string{"rfid", "klas", "web", "server"} {
		if !seen[code] {
			t.Fatalf("1차 %s 없음", code)
		}
	}
}

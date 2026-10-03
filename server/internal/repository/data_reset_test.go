package repository

import (
	"database/sql"
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestExecuteResetTestScopeLeavesLiveRows(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "reset.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id, is_test)
		VALUES ('C-live','실기관','실기관',1,?,0)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id, is_test)
		VALUES ('C-test','테스트기관','테스트기관',1,?,1)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, customer_id, symptom, status, org_id, is_test)
		VALUES ('AS-live','R-LIVE','C-live','실증상','open',?,0)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, customer_id, symptom, status, org_id, is_test)
		VALUES ('AS-test','R-TEST','C-test','테스터증상','open',?,1)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_processes (process_id, as_id, work_content, time_spent) VALUES ('P-live','AS-live','실조치',30)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_processes (process_id, as_id, work_content, time_spent) VALUES ('P-test','AS-test','테스터조치',30)`); err != nil {
		t.Fatal(err)
	}

	prev, err := PreviewReset(db, ResetScopeTest, model.OrgIDLibrary)
	if err != nil {
		t.Fatal(err)
	}
	if countFor(prev, "customers") != 1 || countFor(prev, "as_receipts") != 1 {
		t.Fatalf("미리보기 %+v", prev)
	}

	if _, err := ExecuteReset(db, ResetScopeTest, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}

	assertCount(t, db, `SELECT COUNT(*) FROM customers WHERE customer_id='C-live'`, 1)
	assertCount(t, db, `SELECT COUNT(*) FROM customers WHERE customer_id='C-test'`, 0)
	assertCount(t, db, `SELECT COUNT(*) FROM as_receipts WHERE as_id='AS-live'`, 1)
	assertCount(t, db, `SELECT COUNT(*) FROM as_receipts WHERE as_id='AS-test'`, 0)
	assertCount(t, db, `SELECT COUNT(*) FROM as_processes WHERE process_id='P-live'`, 1)
	assertCount(t, db, `SELECT COUNT(*) FROM as_processes WHERE process_id='P-test'`, 0)
	assertCount(t, db, `SELECT COUNT(*) FROM customers WHERE COALESCE(is_test,0)=0`, 1)
	assertCount(t, db, `SELECT COUNT(*) FROM as_receipts WHERE COALESCE(is_test,0)=0`, 1)
}

func countFor(rows []ResetCount, table string) int64 {
	for _, r := range rows {
		if r.Table == table {
			return r.N
		}
	}
	return 0
}

func assertCount(t *testing.T, db *sql.DB, q string, want int) {
	t.Helper()
	var n int
	if err := db.QueryRow(q).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != want {
		t.Fatalf("%s = %d want %d", q, n, want)
	}
}

package repository

import (
	"path/filepath"
	"testing"
)

func TestASReceiptsCauseReportColumns(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "cause_report.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	for _, col := range []string{"cause_detail", "conclusion", "cause_type"} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('as_receipts') WHERE name=?`, col).Scan(&n); err != nil || n != 1 {
			t.Fatalf("%s 없음 n=%d err=%v", col, n, err)
		}
	}
}

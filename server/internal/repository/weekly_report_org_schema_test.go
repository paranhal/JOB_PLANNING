package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func Test078WeeklyReportOrgMigration(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "p78.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	up, err := weeklyReportOrgMigrationSQL()
	if err != nil {
		t.Fatal(err)
	}
	if weeklyReportSQLHasSeedInsert(up) {
		t.Fatal("078 up SQL 에 시드 INSERT 가 있으면 안 됨")
	}
	down, err := weeklyReportOrgDownSQL()
	if err != nil {
		t.Fatal(err)
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('weekly_report_rows') WHERE name='activity_kind'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("activity_kind n=%d err=%v", n, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='company_weekly_reports'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("company_weekly_reports n=%d err=%v", n, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM weekly_report_rows WHERE org_id=?`, model.OrgIDLibrary).Scan(&n); err != nil || n < 6 {
		t.Fatalf("O01 시드 행 n=%d err=%v", n, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM weekly_report_rows WHERE org_id IN ('O02','O03')`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("O02/O03 시드 금지 n=%d", n)
	}
	var kind string
	if err := db.QueryRow(`SELECT activity_kind FROM weekly_report_rows WHERE row_key='X02'`).Scan(&kind); err != nil || kind != "asset" {
		t.Fatalf("X02 kind=%q err=%v", kind, err)
	}
	if err := db.QueryRow(`SELECT activity_kind FROM weekly_report_rows WHERE row_key='X03'`).Scan(&kind); err != nil || kind != "tech" {
		t.Fatalf("X03 kind=%q err=%v", kind, err)
	}

	if err := execSQLIgnoreDupCol(db, down); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='company_weekly_reports'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("down reports n=%d", n)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('weekly_report_rows') WHERE name='activity_kind'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("down activity_kind n=%d", n)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('weekly_report_rows') WHERE name='org_id'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("down org_id 유지 n=%d", n)
	}

	if err := execSQLIgnoreDupCol(db, up); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('weekly_report_rows') WHERE name='sort_order'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("re-up sort_order n=%d", n)
	}
}

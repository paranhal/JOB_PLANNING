package repository

import (
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func Test070ContractYearsOverlapAndTables(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "p70.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, tbl := range []string{"contract_years", "project_sites", "equipment_categories", "project_equipment"} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, tbl).Scan(&n); err != nil || n != 1 {
			t.Fatalf("table %s n=%d err=%v", tbl, n, err)
		}
	}
	if _, err := db.Exec(`INSERT INTO work_projects(project_id,name,org_id) VALUES('WP70','다년',?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO contract_years(id,project_id,year_no,label,start_date,end_date,amount,org_id)
		VALUES('Y1','WP70',1,'1차','2026-01-01','2026-12-31',100,?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO contract_years(id,project_id,year_no,label,start_date,end_date,amount,org_id)
		VALUES('Y2','WP70',2,'2차','2026-06-01','2027-05-31',100,?)`, model.OrgIDLibrary)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "overlap") {
		t.Fatalf("겹침 거부 기대, err=%v", err)
	}
	_, err = db.Exec(`INSERT INTO contract_years(id,project_id,year_no,label,start_date,end_date,amount,org_id)
		VALUES('Y1b','WP70',1,'중복','2027-01-01','2027-12-31',1,?)`, model.OrgIDLibrary)
	if err == nil {
		t.Fatal("year_no 중복")
	}
}

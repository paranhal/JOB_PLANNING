package repository

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestCompanyWeeklyASDoneEmptyCompleteIncludesOldReceipt(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "as59.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := NewSettingsRepo(db).Set(SettingMetricsBaseComplete, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('C-OLD','old','old',1)`); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 10; i++ {
		id := fmt.Sprintf("AS-OLD-%d", i)
		if _, err := db.Exec(`
			INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, complete_datetime, status, project_id)
			VALUES (?,?,?,'2026-04-01 10:00:00','2026-08-12 11:00:00','completed','WPSEED01')`, id, id, "C-OLD"); err != nil {
			t.Fatal(err)
		}
	}
	n, err := NewStatsRepo(db).countCompanyASDone("WPSEED01", "2026-08-10", "2026-08-17", OrgAll)
	if err != nil {
		t.Fatal(err)
	}
	if n != 10 {
		t.Fatalf("AS=%d want 10 (완료 기준일 비움)", n)
	}
}

func TestIncompleteMntVisitsListed(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "inc.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO maintenance_plans (plan_id, plan_year, title, status) VALUES ('mp1',2026,'t','draft')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('C1','유구도서관','유구',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO maintenance_visits (visit_id, plan_id, visit_date, customer_id, product_type, completed, completed_date, project_id)
		VALUES ('V1','mp1','2026-08-12','C1','KLAS',0,'','WPSEED03')`); err != nil {
		t.Fatal(err)
	}
	got, err := NewStatsRepo(db).listIncompleteMntVisits("2026-08-10", "2026-08-17")
	if err != nil || len(got) != 1 || got[0].OrgName != "유구도서관" {
		t.Fatalf("incomplete=%v err=%v", got, err)
	}
}

func TestDropDupMntAdminLine(t *testing.T) {
	lines := []adminLine{
		{Title: "도솔도서관 정기점검", CustomerID: "C1"},
		{Title: "계약변경 회신", CustomerID: "C1"},
	}
	out := dropDupMntAdmin(lines, map[string]bool{"C1": true})
	if len(out) != 1 || out[0].Title != "계약변경 회신" {
		t.Fatalf("%+v", out)
	}
}

func TestWeeklyActivityRowsFillWithoutDoubleCount(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "act.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('CA','기관','기관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO work_tasks (task_id, work_type, project_id, title, status, complete_date, due_date, customer_id)
		VALUES
		('WT-asset-free','admin',NULL,'RFID 설치','complete','2026-08-12','2026-08-12','CA'),
		('WT-asset-mapped','admin','WPSEED01','RFID 설치 매핑','complete','2026-08-12','2026-08-12','CA')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO sales_projects (sales_id, name, stage, status) VALUES ('S-FREE','기술영업','propose','active')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO sales_activities (activity_id, sales_id, activity_date, activity_type, title)
		VALUES ('SA1','S-FREE','2026-08-12','proposal','데모 제안')`); err != nil {
		t.Fatal(err)
	}
	draft, err := NewStatsRepo(db).BuildCompanyWeeklyDraft(time.Date(2026, 8, 17, 0, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]model.CompanyWeeklyRow{}
	for _, row := range draft.Rows {
		byKey[row.RowKey] = row
	}
	if byKey["X02"].RowKind != "activity" {
		t.Fatalf("X02 kind=%q", byKey["X02"].RowKind)
	}
	if !strings.Contains(byKey["X02"].PrevText, "RFID 설치") {
		t.Fatalf("자산관리 비었음: %q", byKey["X02"].PrevText)
	}
	if strings.Contains(byKey["X02"].PrevText, "매핑") {
		t.Fatal("사업 행 건이 자산관리에도 들어감")
	}
	if byKey["X03"].RowKind != "activity" {
		t.Fatalf("X03 없음 %+v", byKey["X03"])
	}
	if !strings.Contains(byKey["X03"].PrevText, "데모 제안") {
		t.Fatalf("기술영업=%q", byKey["X03"].PrevText)
	}
}

func TestWeeklyReportActivitySeedKeepsOldMeta(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "seed59.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	var kind string
	if err := db.QueryRow(`SELECT row_kind FROM weekly_report_rows WHERE row_key='X02'`).Scan(&kind); err != nil || kind != "activity" {
		t.Fatalf("X02 kind=%q err=%v", kind, err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM weekly_report_rows WHERE row_key='X03'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("X03=%d", n)
	}
}

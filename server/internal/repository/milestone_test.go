package repository

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestMilestoneSpecWeek20261005(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "ms.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('c1','도서관','도서관',1)`); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT OR IGNORE INTO holidays(holiday_date, holiday_year, name, kind) VALUES ('2026-10-09', 2026, '한글날', 'public')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		INSERT INTO as_receipts (
			as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date,
			complete_datetime, status, assigned_to, data_origin, org_id, is_test
		) VALUES
		('A1','R-A1','c1','2026-09-28 09:00:00','2026-09-29','2026-10-06 16:00:00','completed','기술A','app','O01',0),
		('A2','R-A2','c1','2026-10-06 09:00:00','2026-10-06','2026-10-07 16:00:00','completed','기술A','app','O01',0),
		('A3','R-A3','c1','2026-10-07 09:00:00','2026-10-07',NULL,'in_progress','기술A','app','O01',0),
		('A4','R-A4','c1','2026-09-21 09:00:00','2026-09-22',NULL,'in_progress','기술A','app','O01',0),
		('A5','R-A5','c1','2026-10-08 09:00:00','2026-10-08','2026-10-13 16:00:00','in_progress','기술A','app','O01',0),
		('A9','R-A9','c1','2026-10-06 09:00:00','2026-10-06','2026-10-06 16:00:00','completed','테스터','app','O01',1),
		('B1','R-B1','c1','2026-10-06 09:00:00','2026-10-06','2026-10-06 16:00:00','completed','기술B','app','O99',0)
	`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		INSERT INTO work_tasks (task_id, work_type, title, status, work_date, due_date, complete_date,
			org_id, is_test, recurrence_role, parent_task_id, source_type, assignee)
		VALUES
		('P1','admin','반복부모','complete','2026-10-06','2026-10-06','2026-10-06','O01',0,'parent','','','관리'),
		('C1','admin','자식1','complete','2026-10-06','2026-10-06','2026-10-06','O01',0,'occurrence','P1','','관리'),
		('C2','admin','자식2','complete','2026-10-07','2026-10-07','2026-10-07','O01',0,'occurrence','P1','','관리'),
		('C3','admin','자식3','complete','2026-10-08','2026-10-08','2026-10-08','O01',0,'occurrence','P1','','관리')
	`)
	if err != nil {
		t.Fatal(err)
	}

	repo := NewStatsRepo(db)
	mon := time.Date(2026, 10, 5, 0, 0, 0, 0, time.Local)

	if err := repo.ComputeMilestones(mon, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	rows, err := repo.ListMilestones("2026-10-05", model.OrgIDLibrary, model.MilestoneDomainAS, model.MilestoneScopeOrg)
	if err != nil || len(rows) != 1 {
		t.Fatalf("as rows=%d err=%v", len(rows), err)
	}
	as := rows[0]
	if as.Received != 3 {
		t.Fatalf("AS 접수=%d want 3", as.Received)
	}
	if as.Processed != 2 {
		t.Fatalf("AS 조치=%d want 2", as.Processed)
	}
	if as.Carried != 3 {
		t.Fatalf("AS 이월=%d want 3", as.Carried)
	}

	admin, err := repo.ListMilestones("2026-10-05", model.OrgIDLibrary, model.MilestoneDomainAdmin, model.MilestoneScopeOrg)
	if err != nil || len(admin) != 1 {
		t.Fatal(err)
	}
	if admin[0].Processed != 1 {
		t.Fatalf("행정 완료=%d want 1 (자식 중복 금지)", admin[0].Processed)
	}

	if as.Received != 3 {
		t.Fatal("테스터·타조직이 섞이면 접수 3이 아님")
	}

	if err := repo.FixMilestones("2026-10-05", model.OrgIDLibrary, "관리자"); err != nil {
		t.Fatal(err)
	}
	if repo.MilestoneOrgState("2026-10-05", model.OrgIDLibrary) != model.MilestoneFixed {
		t.Fatal("확정 안 됨")
	}
	as.Received = 99
	as.ComputedAt = "x"
	if err := repo.upsertMilestone(as); err != nil {
		t.Fatal(err)
	}
	again, _ := repo.ListMilestones("2026-10-05", model.OrgIDLibrary, model.MilestoneDomainAS, model.MilestoneScopeOrg)
	if again[0].Received != 3 {
		t.Fatal("확정 후 다시 찍히면 안 된다")
	}
}

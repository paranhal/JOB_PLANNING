package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestListCompletedForStatusPeriodUsesCompleteDate(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "done.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c1','도서관','도서관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_plans (plan_id, plan_year, title, status)
		VALUES ('P1', 2026, '2026', 'approved')`); err != nil {
		t.Fatal(err)
	}
	// 완료일은 금주, 일일업무 배정일은 전주 → 통계 처리에는 포함
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date,
		status, assigned_to, complete_datetime, symptom
	) VALUES
		('a-late','R1','c1','2026-08-07 09:00:00','2026-08-07','completed','최혜영','2026-08-10 11:00:00','전주배정'),
		('a-future','R2','c1','2026-08-05 09:00:00','2026-08-14','completed','최혜영','2026-08-17 11:00:00','다음주완료'),
		('a-none','R3','c1','2026-08-12 09:00:00',NULL,'completed','양기헌','2026-08-12 15:00:00','일일업무없음')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks (task_id, work_type, title, work_date, due_date, duration_min, status, assignee, source_type, source_id)
		VALUES
		('WT-late','as','[AS]전주','2026-08-07','2026-08-07',30,'complete','최혜영','as','a-late'),
		('WT-fut','as','[AS]다음주','2026-08-14','2026-08-14',30,'complete','최혜영','as','a-future')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits (visit_id, plan_id, visit_date, customer_id, completed, completed_date, assignee, product_type)
		VALUES ('V-orphan','P1','2026-08-10','c1',1,'2026-08-10','양기헌','앤로보틱스')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks (task_id, work_type, title, work_date, due_date, duration_min, status, assignee, complete_date)
		VALUES ('WT-adm','admin','행정완료','2026-08-11','2026-08-11',30,'complete','태자운','2026-08-11')`); err != nil {
		t.Fatal(err)
	}

	items, err := NewWBRepo(db).ListCompletedForStatusPeriod("2026-08-10", "2026-08-16")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]model.WorkTask{}
	for _, it := range items {
		key := it.SourceID
		if key == "" {
			key = it.TaskID
		}
		got[key] = it
	}
	if _, ok := got["a-late"]; !ok {
		t.Fatal("완료일이 금주인 AS가 빠짐")
	}
	if got["a-late"].WorkDate != "2026-08-10" {
		t.Fatalf("조치 표시일=%s want 완료일 8/10", got["a-late"].WorkDate)
	}
	if _, ok := got["a-future"]; ok {
		t.Fatal("완료일이 다음주인 AS가 금주 조치에 들어가면 안 됨")
	}
	if _, ok := got["a-none"]; !ok {
		t.Fatal("일일업무 없는 완료 AS가 빠짐")
	}
	if _, ok := got["V-orphan"]; !ok {
		t.Fatal("일일업무 없는 완료 점검이 빠짐")
	}
	if _, ok := got["WT-adm"]; !ok {
		t.Fatal("완료 행정이 빠짐")
	}

	// 통계 처리 건수와 같아야 함
	f := ParseMeetingFilter(model.StatsScopeTeam, "", "")
	asS, err := NewStatsRepo(db).countASSlice("2026-08-10", "2026-08-17", f)
	if err != nil {
		t.Fatal(err)
	}
	mntS, err := NewStatsRepo(db).countMntSlice("2026-08-10", "2026-08-17", f)
	if err != nil {
		t.Fatal(err)
	}
	admS, err := NewStatsRepo(db).countAdminSlice("2026-08-10", "2026-08-17", f)
	if err != nil {
		t.Fatal(err)
	}
	nAS, nM, nAd := 0, 0, 0
	for _, it := range items {
		switch it.SourceType {
		case model.WBSourceAS:
			nAS++
		case model.WBSourceMaintenance:
			nM++
		default:
			nAd++
		}
	}
	if nAS != asS.Process || nM != mntS.Process || nAd != admS.Process {
		t.Fatalf("조치 AS/Mnt/Admin=%d/%d/%d stats process=%d/%d/%d",
			nAS, nM, nAd, asS.Process, mntS.Process, admS.Process)
	}
}

func TestCompletedAdminUsesCompleteDateNotWorkDate(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "adm.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO work_tasks (
		task_id, work_type, title, work_date, due_date, duration_min, status, assignee, complete_date
	) VALUES ('WT-sep','admin','8월배정9월완료','2026-08-11','2026-08-11',30,'complete','태자운','2026-09-03')`); err != nil {
		t.Fatal(err)
	}

	aug, err := NewWBRepo(db).ListCompletedForStatusPeriod("2026-08-01", "2026-08-31")
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range aug {
		if it.TaskID == "WT-sep" {
			t.Fatal("8월 배정·9월 완료 건이 8월 조치에 들어갔다")
		}
	}
	sep, err := NewWBRepo(db).ListCompletedForStatusPeriod("2026-09-01", "2026-09-30")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, it := range sep {
		if it.TaskID == "WT-sep" {
			found = true
			if it.WorkDate != "2026-09-03" {
				t.Fatalf("표시일=%s want 완료일 9/3", it.WorkDate)
			}
		}
	}
	if !found {
		t.Fatal("9월 완료 행정이 9월 조치에 없다")
	}
}

func TestCompletedSkipsMissingCompleteDate(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "miss.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c1','도서관','도서관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_plans (plan_id, plan_year, title, status)
		VALUES ('P1', 2026, '2026', 'approved')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, status, assigned_to, updated_at
	) VALUES ('a-nodate','R9','c1','2026-08-10 09:00:00','completed','최혜영','2026-09-01 10:00:00')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits (visit_id, plan_id, visit_date, customer_id, completed, assignee)
		VALUES ('V-nodate','P1','2026-08-12','c1',1,'양기헌')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks (task_id, work_type, title, work_date, status, assignee, updated_at)
		VALUES ('WT-nodate','admin','완료일없음','2026-08-11','complete','태자운','2026-09-01 11:00:00')`); err != nil {
		t.Fatal(err)
	}

	items, err := NewWBRepo(db).ListCompletedForStatusPeriod("2026-08-01", "2026-09-30")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("완료일 없는 건이 조치에 들어감: %d", len(items))
	}
	miss, err := NewWBRepo(db).CountMissingCompleteDates()
	if err != nil {
		t.Fatal(err)
	}
	if miss.AS != 1 || miss.Mnt != 1 || miss.Admin != 1 || miss.Total() != 3 {
		t.Fatalf("미기록 %+v", miss)
	}
	list, err := NewWBRepo(db).ListMissingCompleteDates()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("미기록 목록 %d", len(list))
	}
}

func TestCountMissingAssignees(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "unassigned.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c1','도서관','도서관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_plans (plan_id, plan_year, title, status)
		VALUES ('P1', 2026, '2026', 'approved')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, status, complete_datetime
	) VALUES ('a-none','R1','c1','2026-08-10 09:00:00','completed','2026-08-11 10:00:00')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits (visit_id, plan_id, visit_date, customer_id, completed, completed_date)
		VALUES ('V-none','P1','2026-08-12','c1',1,'2026-08-12')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks (task_id, work_type, title, work_date, status, complete_date)
		VALUES ('WT-none','admin','담당없음','2026-08-11','complete','2026-08-11')`); err != nil {
		t.Fatal(err)
	}

	miss, err := NewWBRepo(db).CountMissingAssignees()
	if err != nil {
		t.Fatal(err)
	}
	if miss.AS != 1 || miss.Mnt != 1 || miss.Admin != 1 || miss.Total() != 3 {
		t.Fatalf("미배정 %+v", miss)
	}
	list, err := NewWBRepo(db).ListMissingAssignees()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("미배정 목록 %d", len(list))
	}
}

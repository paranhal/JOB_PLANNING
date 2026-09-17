package repository

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestListCompletedOnAndScheduledOnMeeting(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "meeting.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	anchor := time.Date(2026, 8, 11, 12, 0, 0, 0, time.Local)
	today := anchor.Format("2006-01-02")
	yesterday := anchor.AddDate(0, 0, -1).Format("2006-01-02")

	mustExecMeeting(t, db, `INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c-meet','회의도서관','회의도서관',1)`)
	mustExecMeeting(t, db, `INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date,
		schedule_confirmed, status, assigned_to, complete_datetime, symptom, updated_at
	) VALUES
		('as-done','R2608-M01','c-meet','2026-08-01','`+yesterday+`',1,'partial_complete','양기헌',
		 '`+yesterday+` 18:00:00','어제 조치','`+yesterday+` 18:00:00'),
		('as-today','R2608-M02','c-meet','2026-08-05','`+today+`',1,'assigned','양기헌',
		 NULL,'오늘 예정','`+today+` 09:00:00')`)
	mustExecMeeting(t, db, `INSERT INTO maintenance_plans (plan_id, plan_year, title, status)
		VALUES ('mp-meet', 2026, '2026 회의용', 'approved')`)
	mustExecMeeting(t, db, `INSERT INTO maintenance_visits (
		visit_id, plan_id, visit_date, customer_id, sort_order, completed, assignee, product_type
	) VALUES ('mv-today','mp-meet','`+today+`','c-meet',1,0,'양기헌','KLAS')`)
	mustExecMeeting(t, db, `INSERT INTO work_tasks (
		task_id, work_type, title, due_date, work_date, status, assignee, updated_at, complete_date
	) VALUES
		('WT-meet-done','admin','어제 행정 완료','`+yesterday+`','`+yesterday+`','complete','양기헌','`+yesterday+` 17:00:00','`+yesterday+`'),
		('WT-meet-open','support','오늘 지원 예정','`+today+`','`+today+`','waiting','양기헌','`+today+` 08:00:00',NULL)`)

	repo := NewWorkBoardRepo(db)
	done, err := repo.ListCompletedOn(yesterday, "", nil, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !hasWorkRef(done, "as-done") {
		t.Fatalf("전일 실적에 부분완료 AS 없음: %+v", refsOf(done))
	}
	if !hasWorkRef(done, "WT-meet-done") {
		t.Fatalf("전일 실적에 행정 완료 없음: %+v", refsOf(done))
	}
	if hasWorkRef(done, "as-today") {
		t.Fatal("오늘 예정 AS가 전일 실적에 들어가면 안 됨")
	}

	sched, err := repo.ListScheduledOn(today, "", nil, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !hasWorkRef(sched, "as-today") {
		t.Fatalf("오늘 예정에 AS 없음: %+v", refsOf(sched))
	}
	if !hasWorkRef(sched, "mv-today") {
		t.Fatalf("오늘 예정에 점검 없음: %+v", refsOf(sched))
	}
	if !hasWorkRef(sched, "WT-meet-open") {
		t.Fatalf("오늘 예정에 행정/지원 없음: %+v", refsOf(sched))
	}
	if hasWorkRef(sched, "as-done") {
		t.Fatal("어제 완료 AS가 오늘 예정에 들어가면 안 됨")
	}

	for _, it := range done {
		if it.RefID == "as-done" && it.Prefix != model.WorkPrefixAS {
			t.Fatalf("AS prefix: %s", it.Prefix)
		}
		if it.RefID == "WT-meet-done" && (it.Prefix != model.WorkPrefixGeneral || it.SubLabel != "행정") {
			t.Fatalf("행정 item: %+v", it)
		}
	}
}

func TestListScheduledOnIncludesCompleteAndUnconfirmed(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "meeting-sched.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	today := "2026-09-07"
	mustExecMeeting(t, db, `INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c-38','회의도서관','회의도서관',1)`)
	mustExecMeeting(t, db, `INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date,
		schedule_confirmed, status, assigned_to, complete_datetime, symptom, updated_at
	) VALUES
		('as-done-today','R2609-D01','c-38','2026-09-01','`+today+`',1,'completed','최혜영',
		 '`+today+` 16:00:00','오늘 완료','`+today+` 16:00:00'),
		('as-open-unconf','R2609-U01','c-38','2026-09-01','`+today+`',0,'assigned','최혜영',
		 NULL,'미확정 예정','`+today+` 09:00:00')`)

	repo := NewWorkBoardRepo(db)
	sched, err := repo.ListScheduledOn(today, "", nil, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !hasWorkRef(sched, "as-done-today") {
		t.Fatalf("완료 건이 예정에 없음: %+v", refsOf(sched))
	}
	if !hasWorkRef(sched, "as-open-unconf") {
		t.Fatalf("미확정 건이 예정에 없음: %+v", refsOf(sched))
	}
	var sawUnconf bool
	for _, it := range sched {
		if it.RefID == "as-open-unconf" {
			if !it.Tentative {
				t.Fatal("미확정 뱃지 플래그가 없다")
			}
			sawUnconf = true
		}
		if it.RefID == "as-done-today" && it.Tentative {
			t.Fatal("확정 완료 건에 미확정이 붙었다")
		}
	}
	if !sawUnconf {
		t.Fatal("미확정 건 스캔 실패")
	}

	cols := BuildStatsPeriodColumns(model.StatsViewDay, time.Date(2026, 9, 7, 0, 0, 0, 0, time.Local))
	if err := NewStatsRepo(db).FillPeriodOverview(cols, ParseMeetingFilter(model.StatsScopeAssignee, "최혜영", "")); err != nil {
		t.Fatal(err)
	}
	planned := cols[1].Counts.PlannedTotal()
	if planned != 2 {
		t.Fatalf("통계 예정=%d want 2", planned)
	}
	if len(sched) != planned {
		t.Fatalf("목록 %d 통계 %d", len(sched), planned)
	}
}

func mustExecMeeting(t *testing.T, db *sql.DB, q string) {
	t.Helper()
	if _, err := db.Exec(q); err != nil {
		t.Fatalf("exec: %v\n%s", err, q)
	}
}

func hasWorkRef(items []model.WorkListItem, refID string) bool {
	for _, it := range items {
		if it.RefID == refID {
			return true
		}
	}
	return false
}

func refsOf(items []model.WorkListItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.RefID
	}
	return out
}

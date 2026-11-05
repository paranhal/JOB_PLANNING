package repository

import (
	"database/sql"
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestAdminGTDCompleteDateWithoutProcessMatchesGeneral(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "gtd_counts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	day := "2026-08-11"
	insertGTDTask(t, db, "WT-adm-done", model.WBWorkAdmin, model.WBTaskComplete, day, day, day)
	insertGTDTask(t, db, "WT-sup-done", model.WBWorkSupport, model.WBTaskComplete, day, day, day)
	insertGTDTask(t, db, "WT-sale-done", model.WBWorkSales, model.WBTaskComplete, day, day, day)
	insertGTDTask(t, db, "WT-pw-done", model.WBWorkProjectWork, model.WBTaskComplete, day, day, day)

	wb := NewWorkBoardRepo(db)
	done, err := wb.ListCompletedOn(day, "", nil, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"WT-adm-done", "WT-sup-done", "WT-sale-done", "WT-pw-done"} {
		if !hasWorkRef(done, id) {
			t.Fatalf("complete_date만 있고 조치 이력 없는 %s가 완료 집계에서 빠짐: %+v", id, refsOf(done))
		}
	}

	f := ParseMeetingFilter(model.StatsScopeTeam, "", "")
	adm, err := NewStatsRepo(db).countAdminSlice(day, "2026-08-12", f)
	if err != nil {
		t.Fatal(err)
	}
	if adm.Process != 2 {
		t.Fatalf("통계 행정/지원 처리=%d want 2 (조치 이력 없이 complete_date)", adm.Process)
	}
	sale, err := NewStatsRepo(db).countSalesSlice(day, "2026-08-12", f)
	if err != nil {
		t.Fatal(err)
	}
	if sale.Process != 1 {
		t.Fatalf("통계 영업 처리=%d want 1", sale.Process)
	}

	procs, days, err := NewWBRepo(db).ListTasksWithProcessBetweenForOrg(day, day, OrgAll)
	if err != nil {
		t.Fatal(err)
	}
	if len(procs) != 4 {
		t.Fatalf("완료일만 있는 건이 완료 보기에 없음: %d days=%v", len(procs), days)
	}
}

func TestAdminGTDStatusInclusionCounts(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "gtd_status.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	day := "2026-08-11"
	insertGTDTask(t, db, "WT-inbox", model.WBWorkAdmin, model.WBTaskInbox, day, "", "")
	insertGTDTask(t, db, "WT-wait", model.WBWorkAdmin, model.WBTaskWaiting, day, "", "")
	insertGTDTask(t, db, "WT-prog", model.WBWorkSupport, model.WBTaskInProgress, day, "", "")
	insertGTDTask(t, db, "WT-wfor", model.WBWorkAdmin, model.WBTaskWaitingFor, day, "", "")
	insertGTDTask(t, db, "WT-hold", model.WBWorkAdmin, model.WBTaskHold, day, "", "")
	insertGTDTask(t, db, "WT-done", model.WBWorkAdmin, model.WBTaskComplete, day, "", day)
	insertGTDTask(t, db, "WT-canc", model.WBWorkAdmin, model.WBTaskCancelled, day, "", "")
	insertGTDTask(t, db, "WT-inbox-nodate", model.WBWorkAdmin, model.WBTaskInbox, "", "", "")

	wb := NewWorkBoardRepo(db)
	sched, err := wb.ListScheduledOn(day, "", nil, 100)
	if err != nil {
		t.Fatal(err)
	}
	open, err := wb.ListWorkToday("", nil, day)
	if err != nil {
		t.Fatal(err)
	}
	done, err := wb.ListCompletedOn(day, "", nil, 100)
	if err != nil {
		t.Fatal(err)
	}
	unplanned, kinds, err := wb.ListUnplanned("", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	unplaced, err := NewWBRepo(db).ListUnplacedAdminTasks()
	if err != nil {
		t.Fatal(err)
	}
	st, err := NewWBRepo(db).CountAdminWorkStats()
	if err != nil {
		t.Fatal(err)
	}

	wantSched := map[string]bool{
		"WT-inbox": true, "WT-wait": true, "WT-prog": true, "WT-wfor": true, "WT-hold": true,
		"WT-done": true, "WT-canc": true, "WT-inbox-nodate": false,
	}
	wantOpen := map[string]bool{
		"WT-inbox": true, "WT-wait": true, "WT-prog": true, "WT-wfor": true, "WT-hold": true,
		"WT-done": true, "WT-canc": false, "WT-inbox-nodate": false,
	}
	wantDone := map[string]bool{
		"WT-inbox": false, "WT-wait": false, "WT-prog": false, "WT-wfor": false, "WT-hold": false,
		"WT-done": true, "WT-canc": false, "WT-inbox-nodate": false,
	}
	for id, want := range wantSched {
		if hasWorkRef(sched, id) != want {
			t.Fatalf("오늘 예정 %s got=%v want=%v refs=%v", id, hasWorkRef(sched, id), want, refsOf(sched))
		}
	}
	for id, want := range wantOpen {
		if hasWorkRef(open, id) != want {
			t.Fatalf("오늘 내 업무 %s got=%v want=%v refs=%v", id, hasWorkRef(open, id), want, refsOf(open))
		}
	}
	for id, want := range wantDone {
		if hasWorkRef(done, id) != want {
			t.Fatalf("완료 집계 %s got=%v want=%v refs=%v", id, hasWorkRef(done, id), want, refsOf(done))
		}
	}
	if !hasUnplannedRef(unplanned, "WT-inbox-nodate") {
		t.Fatalf("날짜 없는 수집함(레거시 inbox)이 미계획에 없음 kinds=%+v", kinds)
	}
	foundUnplaced := false
	for _, tsk := range unplaced {
		if tsk.TaskID == "WT-inbox-nodate" {
			foundUnplaced = true
		}
	}
	if !foundUnplaced {
		t.Fatal("날짜 없는 수집함(레거시 inbox)이 일정 미배치에 없음")
	}
	if st.Inbox != 2 {
		t.Fatalf("행정 현황 Inbox=%d want 2 (레거시 status=inbox만)", st.Inbox)
	}
	if st.Complete != 1 {
		t.Fatalf("행정 현황 Complete=%d want 1", st.Complete)
	}

	created := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "빠른메모", Status: model.WBTaskInbox}
	if err := NewWBRepo(db).CreateTask(created); err != nil {
		t.Fatal(err)
	}
	got, err := NewWBRepo(db).GetTask(created.TaskID)
	if err != nil || got == nil || got.Status != model.WBTaskWaiting {
		t.Fatalf("CreateTask inbox→waiting: %+v err=%v", got, err)
	}
	unplanned2, _, err := wb.ListUnplanned("", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if !hasUnplannedRef(unplanned2, created.TaskID) {
		t.Fatal("날짜 없는 빠른등록(waiting)이 미계획 업무함에 없음")
	}
}

func hasUnplannedRef(items []model.UnplannedItem, refID string) bool {
	for _, it := range items {
		if it.RefID == refID {
			return true
		}
	}
	return false
}

func insertGTDTask(t *testing.T, db *sql.DB, id, workType, status, due, work, complete string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO work_tasks (
		task_id, work_type, title, due_date, work_date, status, assignee, updated_at, complete_date
	) VALUES (?,?,?,?,?,?, '테스터', '2026-08-11 09:00:00', ?)`,
		id, workType, id, due, work, status, complete)
	if err != nil {
		t.Fatal(err)
	}
}

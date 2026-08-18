package repository

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestS11WorkTaskDateColumns(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "s11.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	for _, col := range []string{"receipt_date", "complete_date"} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('work_tasks') WHERE name=?`, col).Scan(&n); err != nil || n != 1 {
			t.Fatalf("%s 컬럼 없음: n=%d err=%v", col, n, err)
		}
	}

	if _, err := db.Exec(`
		INSERT INTO work_tasks (task_id, work_type, title, status, created_at, updated_at)
		VALUES ('WT-old','admin','옛 완료','complete','2026-08-10 09:00:00','2026-08-12 18:00:00')`); err != nil {
		t.Fatal(err)
	}
	applyS11WorkTaskDates(db)

	var receipt, complete string
	if err := db.QueryRow(`SELECT receipt_date, complete_date FROM work_tasks WHERE task_id='WT-old'`).Scan(&receipt, &complete); err != nil {
		t.Fatal(err)
	}
	if receipt != "2026-08-10" {
		t.Fatalf("receipt_date=%s want 2026-08-10", receipt)
	}
	if complete != "2026-08-12" {
		t.Fatalf("complete_date=%s want 2026-08-12", complete)
	}
}

func TestAdminCompleteDateNotShiftedOnLaterEdit(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "s11edit.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	repo := NewWBRepo(db)
	t0 := &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "IRM 회신 취합",
		Status: model.WBTaskComplete, Assignee: "양기헌",
		ReceiptDate: "2026-08-10", CompleteDate: "2026-08-11", CompleteNote: "회신 반영",
	}
	if err := repo.CreateTask(t0); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetTask(t0.TaskID)
	if err != nil || got == nil {
		t.Fatalf("get: %v", err)
	}
	if got.CompleteDate != "2026-08-11" || got.ReceiptDate != "2026-08-10" {
		t.Fatalf("dates receipt=%s complete=%s", got.ReceiptDate, got.CompleteDate)
	}

	got.Assignee = "태자운"
	got.Progress = 100
	got.CompleteDate = "" // 화면이 완료일을 안 보내도 기존 값을 유지
	if err := repo.UpdateTask(got); err != nil {
		t.Fatal(err)
	}
	after, _ := repo.GetTask(t0.TaskID)
	if after.CompleteDate != "2026-08-11" {
		t.Fatalf("완료 후 수정에 완료일이 밀림: %s", after.CompleteDate)
	}
	if after.Assignee != "태자운" {
		t.Fatalf("assignee=%s", after.Assignee)
	}

	wb := NewWorkBoardRepo(db)
	done, err := wb.ListCompletedOn("2026-08-11", "", nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, it := range done {
		if it.RefID == t0.TaskID {
			found = true
		}
	}
	if !found {
		t.Fatal("전일 실적이 complete_date 가 아니라 updated_at 을 쓰면 완료일이 밀린 날로 빠진다")
	}

	today := time.Now().Format("2006-01-02")
	if today != "2026-08-11" {
		later, err := wb.ListCompletedOn(today, "", nil, 50)
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range later {
			if it.RefID == t0.TaskID {
				t.Fatal("수정한 오늘이 완료일로 잡히면 안 된다")
			}
		}
	}

	f := ParseMeetingFilter(model.StatsScopeTeam, "", "")
	slice, err := NewStatsRepo(db).countAdminSlice("2026-08-11", "2026-08-12", f)
	if err != nil {
		t.Fatal(err)
	}
	if slice.Process != 1 {
		t.Fatalf("처리(완료일)=%d want 1", slice.Process)
	}
	avg, n, err := NewStatsRepo(db).avgAdminCompleteDays("2026-08-11", "2026-08-12", f)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || avg < 0.9 || avg > 1.1 {
		t.Fatalf("리드타임 avg=%v n=%d want 1일 (complete_date−receipt_date)", avg, n)
	}
}

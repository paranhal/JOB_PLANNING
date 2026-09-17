package repository

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestWeekReviewStatsLastWeek(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "week-review.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	now := time.Date(2026, 9, 14, 9, 0, 0, 0, time.Local) // Monday
	from, to := model.LastISOWeekRange(now)
	if from != "2026-09-07" || to != "2026-09-13" {
		t.Fatalf("last week %s %s", from, to)
	}
	wb := NewWBRepo(db)
	mustTask := func(t *testing.T, task *model.WorkTask) {
		t.Helper()
		if err := wb.CreateTask(task); err != nil {
			t.Fatal(err)
		}
	}
	mustTask(t, &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "완료기한내", Status: model.WBTaskComplete,
		Assignee: "관리자", ReceiptDate: "2026-09-08", CompleteDate: "2026-09-10",
		DueDate: "2026-09-11", WorkDate: "2026-09-10",
	})
	mustTask(t, &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "완료지연", Status: model.WBTaskComplete,
		Assignee: "관리자", ReceiptDate: "2026-09-07", CompleteDate: "2026-09-11",
		DueDate: "2026-09-08", WorkDate: "2026-09-11",
	})
	mustTask(t, &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "밀려넘어옴", Status: model.WBTaskWaiting,
		Assignee: "관리자", DueDate: "2026-09-10", WorkDate: "2026-09-10",
	})
	mustTask(t, &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "기다리는건", Status: model.WBTaskWaiting,
		Assignee: "관리자", DueDate: "2026-09-09", WorkDate: "2026-09-09",
	})
	tasks, err := wb.ListTasks()
	if err != nil {
		t.Fatal(err)
	}
	var waitID, carryID string
	for _, tsk := range tasks {
		switch tsk.Title {
		case "기다리는건":
			waitID = tsk.TaskID
		case "밀려넘어옴":
			carryID = tsk.TaskID
		}
	}
	if waitID == "" || carryID == "" {
		t.Fatal("task id")
	}
	if err := wb.SetTaskBlocked(waitID, "부품 대기"); err != nil {
		t.Fatal(err)
	}

	rows, carried, err := NewWorkBoardRepo(db).WeekReviewParts("", []string{"관리자"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("completed=%d", len(rows))
	}
	onTime := 0
	for _, row := range rows {
		if row.Due == "" || row.Complete <= row.Due {
			onTime++
		}
	}
	if onTime != 1 {
		t.Fatalf("ontime=%d", onTime)
	}
	if carried != 1 {
		t.Fatalf("carried=%d", carried)
	}
}

func TestBlockedExcludedFromDelayed(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "blocked-delay.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	wb := NewWBRepo(db)
	due := time.Now().AddDate(0, 0, -3).Format("2006-01-02")
	if err := wb.CreateTask(&model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "지연대기", Status: model.WBTaskWaiting,
		Assignee: "관리자", DueDate: due, WorkDate: due,
	}); err != nil {
		t.Fatal(err)
	}
	tasks, err := wb.ListTasks()
	if err != nil || len(tasks) == 0 {
		t.Fatal(err)
	}
	if err := wb.SetTaskBlocked(tasks[0].TaskID, model.BlockedReasonParts); err != nil {
		t.Fatal(err)
	}
	board := NewWorkBoardRepo(db)
	delayed, err := board.ListBucket(model.WorkBucketDelayed, "", []string{"관리자"}, 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range delayed {
		if it.RefID == tasks[0].TaskID {
			t.Fatal("기다리는 건이 지연 버킷에 있다")
		}
	}
	today, err := board.ListWorkToday("", []string{"관리자"}, time.Now().Format("2006-01-02"))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, it := range today {
		if it.RefID == tasks[0].TaskID {
			found = true
			if it.BlockedReason != model.BlockedReasonParts {
				t.Fatalf("blocked=%s", it.BlockedReason)
			}
		}
	}
	if !found {
		t.Fatal("오늘 목록에서 기다리는 건이 빠졌다")
	}
}

package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestApplyInboxToWaiting(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "inbox_mig.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	wb := NewWBRepo(db)
	task := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "운영잔여", Status: model.WBTaskWaiting, DueDate: "2026-08-20"}
	if err := wb.CreateTask(task); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE work_tasks SET status='inbox' WHERE task_id=?`, task.TaskID); err != nil {
		t.Fatal(err)
	}
	applyInboxToWaiting(db)
	got, err := wb.GetTask(task.TaskID)
	if err != nil || got == nil || got.Status != model.WBTaskWaiting {
		t.Fatalf("after migrate %+v err=%v", got, err)
	}
}

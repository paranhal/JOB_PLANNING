package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestCreateSubtasksAndNextFreeSlot(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "wb.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewWBRepo(db)

	parent := &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "보고서 작성", DueDate: "2026-08-10",
		DurationMin: 60, Status: model.WBTaskWaiting,
	}
	if err := repo.CreateTask(parent); err != nil {
		t.Fatal(err)
	}
	dates, err := ExpandDailyDates("2026-08-05", "2026-08-07")
	if err != nil || len(dates) != 3 {
		t.Fatalf("dates: %v %v", dates, err)
	}
	n, err := repo.CreateSubtasks(parent, dates)
	if err != nil || n != 3 {
		t.Fatalf("subtasks n=%d err=%v", n, err)
	}
	children, err := repo.ListChildren(parent.TaskID)
	if err != nil || len(children) != 3 {
		t.Fatalf("children: %d %v", len(children), err)
	}
	if children[0].ParentTaskID != parent.TaskID || children[0].DueDate != "2026-08-05" {
		t.Fatalf("child0: %+v", children[0])
	}

	start, end, err := repo.NextFreeSlot("2026-08-05", 30)
	if err != nil || start != "07:00" || end != "07:30" {
		t.Fatalf("empty day slot: %s-%s err=%v", start, end, err)
	}
	if err := repo.PlaceTask(children[0].TaskID, "2026-08-05", "07:00", "08:00"); err != nil {
		t.Fatal(err)
	}
	start, end, err = repo.NextFreeSlot("2026-08-05", 45)
	if err != nil || start != "08:00" || end != "08:45" {
		t.Fatalf("after place: %s-%s err=%v", start, end, err)
	}
}

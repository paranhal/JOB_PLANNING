package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestRegisterInputStatsFromDailyTasks(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "reg.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	wb := NewWBRepo(db)
	if err := wb.CreateTask(&model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "서류", WorkDate: "2026-10-12",
		DurationMin: 90, Status: "complete", Assignee: "김현우",
	}); err != nil {
		t.Fatal(err)
	}
	if err := wb.CreateTask(&model.WorkTask{
		WorkType: model.WBWorkAS, Title: "현장", WorkDate: "2026-10-12",
		DurationMin: 30, Status: "waiting", Assignee: "박준석",
	}); err != nil {
		t.Fatal(err)
	}
	tot, err := wb.RegisterInputTotals("2026-10-12")
	if err != nil {
		t.Fatal(err)
	}
	if tot.Tasks != 2 || tot.People != 2 || tot.Minutes != 120 || tot.Complete != 1 {
		t.Fatalf("totals=%+v", tot)
	}
	empty, err := wb.RegisterInputTotals("2026-10-11")
	if err != nil {
		t.Fatal(err)
	}
	if empty.Tasks != 0 {
		t.Fatalf("empty=%+v", empty)
	}
}

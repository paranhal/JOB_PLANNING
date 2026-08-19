package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestNextFreeSlotForAssigneeNoOverlap(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewWBRepo(db)

	a := &model.WorkTask{
		TaskID: "A", Title: "점검A", WorkType: model.WBWorkMaintenance,
		WorkDate: "2026-08-05", StartTime: "09:00", EndTime: "11:00", DurationMin: 120,
		Assignee: "최혜영", Status: model.WBTaskWaiting,
	}
	b := &model.WorkTask{
		TaskID: "B", Title: "점검B", WorkType: model.WBWorkMaintenance,
		WorkDate: "2026-08-05", StartTime: "09:00", EndTime: "10:00", DurationMin: 60,
		Assignee: "양기헌", Status: model.WBTaskWaiting,
	}
	if err := repo.CreateTask(a); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(b); err != nil {
		t.Fatal(err)
	}

	// 최혜영은 11:00부터, 양기헌과 독립
	s, e, err := repo.NextFreeSlotForAssignee("2026-08-05", "최혜영", 60, "")
	if err != nil || s != "11:00" || e != "12:00" {
		t.Fatalf("최혜영 slot %s-%s err=%v", s, e, err)
	}
	s2, _, err := repo.NextFreeSlotForAssignee("2026-08-05", "양기헌", 30, "")
	if err != nil || s2 != "10:00" {
		t.Fatalf("양기헌 slot %s err=%v", s2, err)
	}

	ok, err := repo.AssigneeTimeOverlaps("2026-08-05", "최혜영", "10:00", "10:30", "")
	if err != nil || !ok {
		t.Fatalf("overlap want true: %v %v", ok, err)
	}
	ok, err = repo.AssigneeTimeOverlaps("2026-08-05", "최혜영", "11:00", "11:30", "")
	if err != nil || ok {
		t.Fatalf("no overlap: %v %v", ok, err)
	}
}

func TestAssigneeTimeOverlapsIncludesSupportMember(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "sup.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewWBRepo(db)
	a := &model.WorkTask{
		Title: "공동", WorkType: model.WBWorkAdmin,
		WorkDate: "2026-08-18", StartTime: "09:00", EndTime: "11:00", DurationMin: 120,
		Assignee: "최혜영", Status: model.WBTaskWaiting,
	}
	if err := repo.CreateTask(a); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceSupportMembers(a.TaskID, []model.WorkTaskMember{
		{Assignee: "양기헌", Role: model.WBMemberSupport, DurationMin: 120},
	}); err != nil {
		t.Fatal(err)
	}
	ok, err := repo.AssigneeTimeOverlaps("2026-08-18", "양기헌", "09:00", "10:00", "")
	if err != nil || !ok {
		t.Fatalf("지원자 겹침 want true: %v %v", ok, err)
	}
	ok, err = repo.AssigneeTimeOverlaps("2026-08-18", "양기헌", "11:00", "11:30", "")
	if err != nil || ok {
		t.Fatalf("지원자 빈 시간: %v %v", ok, err)
	}
}

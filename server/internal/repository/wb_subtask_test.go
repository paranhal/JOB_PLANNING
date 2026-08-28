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

func TestSubtaskIDsDepthOccurrenceAndDelete(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "sub_tree.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewWBRepo(db)

	root := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "상위", DueDate: "2026-09-30", Status: model.WBTaskWaiting, Assignee: "최혜영"}
	if err := repo.CreateTask(root); err != nil {
		t.Fatal(err)
	}
	c1 := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "하위1", DueDate: "2026-09-10", Status: model.WBTaskWaiting, ParentTaskID: root.TaskID}
	if err := repo.CreateTask(c1); err != nil {
		t.Fatal(err)
	}
	if c1.TaskID != root.TaskID+"-1" {
		t.Fatalf("child id=%s want %s-1", c1.TaskID, root.TaskID)
	}
	c2 := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "하위2", DueDate: "2026-09-11", Status: model.WBTaskWaiting, ParentTaskID: root.TaskID}
	if err := repo.CreateTask(c2); err != nil {
		t.Fatal(err)
	}
	if c2.TaskID != root.TaskID+"-2" {
		t.Fatalf("second=%s", c2.TaskID)
	}
	if err := repo.DeleteParentTask(c1.TaskID); err != nil {
		t.Fatal(err)
	}
	c3 := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "하위3", DueDate: "2026-09-12", Status: model.WBTaskWaiting, ParentTaskID: root.TaskID}
	if err := repo.CreateTask(c3); err != nil {
		t.Fatal(err)
	}
	if c3.TaskID != root.TaskID+"-3" {
		t.Fatalf("reuse deleted seq: %s", c3.TaskID)
	}

	g := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "손자", DueDate: "2026-09-13", Status: model.WBTaskWaiting, ParentTaskID: c2.TaskID}
	if err := repo.CreateTask(g); err != nil {
		t.Fatal(err)
	}
	if g.TaskID != c2.TaskID+"-1" {
		t.Fatalf("grandchild=%s", g.TaskID)
	}
	gg := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "4단계", DueDate: "2026-09-14", Status: model.WBTaskWaiting, ParentTaskID: g.TaskID}
	if err := repo.CreateTask(gg); err != model.ErrSubtaskDepth {
		t.Fatalf("depth 4 err=%v", err)
	}

	if _, err := repo.GenerateOccurrences(root, model.WorkRecurrence{
		StartDate: "2026-09-01", EndDate: "2026-09-01", RuleType: model.RecurrenceManual,
		HolidayPolicy: model.HolidayPolicyAsIs, CompletePolicy: model.CompletePolicyManual,
	}, []string{"2026-09-01"}); err != nil {
		t.Fatal(err)
	}
	subs, err := repo.ListSubtasks(root.TaskID)
	if err != nil || len(subs) != 2 {
		t.Fatalf("subtasks len=%d err=%v", len(subs), err)
	}
	for _, s := range subs {
		if s.RecurrenceRole == model.RecurrenceRoleOccurrence {
			t.Fatalf("occurrence in ListSubtasks: %+v", s)
		}
	}
	all, _ := repo.ListChildren(root.TaskID)
	occN := 0
	for _, ch := range all {
		if ch.RecurrenceRole == model.RecurrenceRoleOccurrence {
			occN++
		}
	}
	if occN != 1 {
		t.Fatalf("occurrences=%d children=%d", occN, len(all))
	}

	if err := repo.DeleteParentTask(root.TaskID); err == nil {
		t.Fatal("delete with subtasks should fail")
	} else if n, ok := model.HasSubtasksN(err); !ok || n != 2 {
		t.Fatalf("has subtasks err=%v n=%d ok=%v", err, n, ok)
	}

	cands, err := repo.ListSubtaskParentCandidates(c2.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range cands {
		if it.TaskID == c2.TaskID || it.TaskID == g.TaskID {
			t.Fatalf("candidate includes self/descendant %s", it.TaskID)
		}
	}
}


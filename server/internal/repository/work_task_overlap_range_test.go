package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestListAssigneeTimeOverlapsRange_OneSpanQuery(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "ovl.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r := NewWBRepo(db)
	name := "김현우"
	for i, d := range []string{"2026-11-10", "2026-11-11", "2026-11-12"} {
		tk := &model.WorkTask{
			Title: "기존" + string(rune('A'+i)), WorkDate: d, DueDate: d,
			StartTime: "09:00", EndTime: "18:30", OrgID: model.OrgIDLibrary,
			Assignee: name, Status: model.WBTaskWaiting, WorkType: model.WBWorkAdmin, Priority: model.WBPriorityNormal,
		}
		if err := r.CreateTask(tk); err != nil {
			t.Fatal(err)
		}
	}
	hits, err := r.ListAssigneeTimeOverlapsRange("2026-11-10", "2026-11-12", "09:00", "09:30", "", []string{name})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) < 3 {
		t.Fatalf("날짜별 겹침 %d", len(hits))
	}
	seen := map[string]bool{}
	for _, h := range hits {
		seen[h.Date] = true
	}
	for _, d := range []string{"2026-11-10", "2026-11-11", "2026-11-12"} {
		if !seen[d] {
			t.Fatalf("%s 없음", d)
		}
	}
}

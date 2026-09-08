package repository

import (
	"path/filepath"
	"testing"
)

func TestWorkListIndexesExist(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "idx.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	want := []string{
		"idx_as_receipts_status_visit",
		"idx_as_receipts_assigned_to",
		"idx_as_receipts_assigned_user_id",
		"idx_as_receipts_complete_datetime",
		"idx_maintenance_visits_completed_date",
		"idx_maintenance_visits_assignee",
		"idx_as_processes_as_id",
		"idx_as_work_items_status_as",
	}
	for _, name := range want {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`, name).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("index %s missing", name)
		}
	}
}

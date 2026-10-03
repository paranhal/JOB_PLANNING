package repository

import (
	"path/filepath"
	"testing"
)

func TestUsersHaveProfileColumns(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "user-cols.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, col := range []string{"org_id", "base_role", "mobile", "tel", "email", "signature_path", "profile_done", "username_changed_at", "is_test"} {
		if !tableHasColumn(db, "users", col) {
			t.Fatalf("users.%s 없음", col)
		}
	}
}

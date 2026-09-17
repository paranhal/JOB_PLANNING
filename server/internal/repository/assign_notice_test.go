package repository

import (
	"path/filepath"
	"testing"
	"time"
)

func TestRecordAssignmentSkipsSelfAndKeepsSeen(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "wan.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r := NewAssignNoticeRepo(db)

	if err := r.RecordAssignment("as", "A1", "U1", "U1"); err != nil {
		t.Fatal(err)
	}
	n, err := r.CountUnseen("U1")
	if err != nil || n != 0 {
		t.Fatalf("자기 배정 알림 n=%d err=%v", n, err)
	}

	if err := r.RecordAssignment("as", "A1", "U2", "U1"); err != nil {
		t.Fatal(err)
	}
	if n, _ := r.CountUnseen("U2"); n != 1 {
		t.Fatalf("배정 알림 n=%d", n)
	}

	if err := r.RecordAssignment("as", "A1", "U3", "U1"); err != nil {
		t.Fatal(err)
	}
	if n, _ := r.CountUnseen("U2"); n != 0 {
		t.Fatal("옛 사람 안 본 알림이 남았다")
	}
	if n, _ := r.CountUnseen("U3"); n != 1 {
		t.Fatalf("새 담당 알림 n=%d", n)
	}

	if err := r.RecordAssignment("as", "B1", "U2", "U1"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE work_assign_notices SET seen_at=? WHERE user_id='U2'`, time.Now().Format("2006-01-02 15:04:05")); err != nil {
		t.Fatal(err)
	}
	if err := r.RecordAssignment("as", "B1", "U3", "U1"); err != nil {
		t.Fatal(err)
	}
	var seenKept int
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_assign_notices WHERE source_id='B1' AND user_id='U2' AND seen_at IS NOT NULL`).Scan(&seenKept); err != nil {
		t.Fatal(err)
	}
	if seenKept != 1 {
		t.Fatal("이미 본 알림이 지워졌다")
	}

	if err := r.RecordAssignment("as", "A1", "", "U1"); err != nil {
		t.Fatal(err)
	}
	var unseenA1 int
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_assign_notices WHERE source_id='A1' AND seen_at IS NULL`).Scan(&unseenA1); err != nil {
		t.Fatal(err)
	}
	if unseenA1 != 0 {
		t.Fatal("미배정인데 안 본 알림이 남았다")
	}
}

func TestListUnseenKeepsOldAssignments(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "wan-old.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r := NewAssignNoticeRepo(db)
	if err := r.RecordAssignment("as", "OLD1", "U2", "U1"); err != nil {
		t.Fatal(err)
	}
	if err := r.RecordAssignment("as", "OLD2", "U2", "U1"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE work_assign_notices SET assigned_at=?`, time.Now().AddDate(0, 0, -3).Format("2006-01-02 15:04:05")); err != nil {
		t.Fatal(err)
	}
	items, err := r.ListUnseen("U2")
	if err != nil || len(items) != 2 {
		t.Fatalf("3일 전 배정 len=%d err=%v", len(items), err)
	}
}

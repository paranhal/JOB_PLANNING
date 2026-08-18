package backup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/audit"
	"customer-support/internal/repository"
)

func TestNextBackupTime(t *testing.T) {
	loc := time.FixedZone("KST", 9*3600)
	cases := []struct {
		now, want string
	}{
		{"2026-08-14 00:30:00", "2026-08-14 01:00:00"},
		{"2026-08-14 00:59:59", "2026-08-14 01:00:00"},
		{"2026-08-14 01:00:00", "2026-08-15 01:00:00"},
		{"2026-08-14 09:00:00", "2026-08-15 01:00:00"},
		{"2026-08-14 23:59:00", "2026-08-15 01:00:00"},
	}
	layout := "2006-01-02 15:04:05"
	for _, tc := range cases {
		now, _ := time.ParseInLocation(layout, tc.now, loc)
		want, _ := time.ParseInLocation(layout, tc.want, loc)
		got := NextBackupTime(now, loc)
		if !got.Equal(want) {
			t.Errorf("now=%s got=%s want=%s", tc.now, got.Format(layout), tc.want)
		}
	}
}

func TestRunCopiesDBAndUploads(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	if err := os.MkdirAll(filepath.Join(dataDir, "uploads", "as", "R1"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "uploads", "as", "R1", "보고서.pdf"), []byte("pdf"), 0644); err != nil {
		t.Fatal(err)
	}
	db, err := repository.InitDB(filepath.Join(dataDir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	audit.Init(db)
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('C1','테스트','테스트',1)`); err != nil {
		t.Fatal(err)
	}

	loc := time.FixedZone("KST", 9*3600)
	now := time.Date(2026, 8, 14, 1, 0, 2, 0, loc)
	if err := Run(Config{DataDir: dataDir, DB: db, Now: func() time.Time { return now }, Loc: loc}); err != nil {
		t.Fatal(err)
	}

	dayDir := filepath.Join(dataDir, "backups", "자동백업_2026-08-14")
	snap := filepath.Join(dayDir, "app.db")
	if _, err := os.Stat(snap); err != nil {
		t.Fatalf("DB 스냅샷 없음: %v", err)
	}
	copied := filepath.Join(dayDir, "uploads", "as", "R1", "보고서.pdf")
	b, err := os.ReadFile(copied)
	if err != nil || string(b) != "pdf" {
		t.Fatalf("uploads 복사 실패: %v %q", err, b)
	}
	if _, err := os.Stat(filepath.Join(dayDir, "backup.txt")); err != nil {
		t.Fatal("backup.txt 없음")
	}
	nested := filepath.Join(dayDir, "backups")
	if _, err := os.Stat(nested); !os.IsNotExist(err) {
		t.Fatal("backups 폴더를 다시 복사하면 안 된다")
	}

	snapDB, err := repository.InitDB(snap)
	if err != nil {
		t.Fatal(err)
	}
	defer snapDB.Close()
	var n int
	if err := snapDB.QueryRow(`SELECT COUNT(*) FROM customers WHERE customer_id='C1'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("스냅샷 내용: n=%d err=%v", n, err)
	}
	if audit.BackupRowCount() != 1 {
		t.Fatalf("data_backups 건수=%d want 1", audit.BackupRowCount())
	}
}

func TestPruneKeepsRecentDays(t *testing.T) {
	root := t.TempDir()
	loc := time.FixedZone("KST", 9*3600)
	now := time.Date(2026, 8, 14, 1, 0, 0, 0, loc)
	for _, d := range []string{
		"2026-07-30", "2026-07-31", "2026-07-30_120000",
		"자동백업_2026-07-30", "수동저장_2026-07-30_12시00분00초",
		"관리로그_2026-07-30-2026-08-01",
		"2026-08-01", "2026-08-14", "자동백업_2026-08-01", "not-a-date",
	} {
		if err := os.MkdirAll(filepath.Join(root, d), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := prune(root, 14, now, loc); err != nil {
		t.Fatal(err)
	}
	mustGone(t, filepath.Join(root, "2026-07-30"))
	mustGone(t, filepath.Join(root, "2026-07-31"))
	mustGone(t, filepath.Join(root, "2026-07-30_120000"))
	mustGone(t, filepath.Join(root, "자동백업_2026-07-30"))
	mustGone(t, filepath.Join(root, "수동저장_2026-07-30_12시00분00초"))
	mustExist(t, filepath.Join(root, "2026-08-01"))
	mustExist(t, filepath.Join(root, "2026-08-14"))
	mustExist(t, filepath.Join(root, "자동백업_2026-08-01"))
	mustExist(t, filepath.Join(root, "관리로그_2026-07-30-2026-08-01"))
	mustExist(t, filepath.Join(root, "not-a-date"))
}

func TestManualSnapshotUsesTimestamp(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	db, err := repository.InitDB(filepath.Join(dataDir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	audit.Init(db)

	loc := time.FixedZone("KST", 9*3600)
	now := time.Date(2026, 8, 14, 10, 11, 12, 0, loc)
	name, err := Snapshot(Config{DataDir: dataDir, DB: db, Now: func() time.Time { return now }, Loc: loc}, KindManual)
	if err != nil {
		t.Fatal(err)
	}
	if name != "수동저장_2026-08-14_10시11분12초" {
		t.Fatalf("폴더명: %q", name)
	}
	dayDir := filepath.Join(dataDir, "backups", name)
	if _, err := os.Stat(filepath.Join(dayDir, "app.db")); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dayDir, "backup.txt"))
	if !strings.Contains(string(b), "kind=manual") {
		t.Fatalf("manifest: %s", b)
	}
	list, err := List(dataDir)
	if err != nil || len(list) != 1 || list[0].Kind != KindManual {
		t.Fatalf("목록: %+v err=%v", list, err)
	}
	if audit.BackupRowCount() != 1 {
		t.Fatalf("data_backups 건수=%d want 1", audit.BackupRowCount())
	}
}

func TestSnapshotWritesBackupRowWithoutPriorInit(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	db, err := repository.InitDB(filepath.Join(dataDir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	audit.Init(nil)

	loc := time.FixedZone("KST", 9*3600)
	now := time.Date(2026, 8, 14, 1, 0, 2, 0, loc)
	if err := Run(Config{DataDir: dataDir, DB: db, Now: func() time.Time { return now }, Loc: loc}); err != nil {
		t.Fatal(err)
	}
	if audit.BackupRowCount() != 1 {
		t.Fatalf("폴더 생성 시 data_backups=%d want 1", audit.BackupRowCount())
	}
}

func TestSyncIndexBackfillsMissingRows(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	db, err := repository.InitDB(filepath.Join(dataDir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	audit.Init(db)

	orphan := filepath.Join(dataDir, "backups", "자동백업_2026-08-10")
	if err := os.MkdirAll(orphan, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(orphan, "app.db"), []byte("sqlite"), 0644); err != nil {
		t.Fatal(err)
	}
	if audit.BackupRowCount() != 0 {
		t.Fatalf("보정 전 테이블은 비어 있어야 함: %d", audit.BackupRowCount())
	}

	loc := time.FixedZone("KST", 9*3600)
	n, err := SyncIndex(Config{DataDir: dataDir, DB: db, Loc: loc})
	if err != nil || n != 1 {
		t.Fatalf("보정: n=%d err=%v", n, err)
	}
	if audit.BackupRowCount() != 1 {
		t.Fatalf("보정 후 data_backups=%d", audit.BackupRowCount())
	}
	n2, err := SyncIndex(Config{DataDir: dataDir, DB: db, Loc: loc})
	if err != nil || n2 != 0 {
		t.Fatalf("재보정은 0건이어야 함: n=%d err=%v", n2, err)
	}
}

func TestLegacyFolderRename(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "2026-08-14"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "2026-08-14_101112"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacyNames(root); err != nil {
		t.Fatal(err)
	}
	mustGone(t, filepath.Join(root, "2026-08-14"))
	mustGone(t, filepath.Join(root, "2026-08-14_101112"))
	mustExist(t, filepath.Join(root, "자동백업_2026-08-14"))
	mustExist(t, filepath.Join(root, "수동저장_2026-08-14_10시11분12초"))
}

func mustGone(t *testing.T, p string) {
	t.Helper()
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("남아 있으면 안 됨: %s", p)
	}
}

func mustExist(t *testing.T, p string) {
	t.Helper()
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("있어야 함: %s (%v)", p, err)
	}
}

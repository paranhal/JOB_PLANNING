package repository

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAppVersionsRestartSameBuild(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "v.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	s := AppStart{Version: "v2.37", Commit: "aaa1111", BuiltAt: "2026-09-13 14:22", StartedAt: "2026-09-13 14:25", Host: "box"}
	if err := RecordAppStart(db, s); err != nil {
		t.Fatal(err)
	}
	if n := CountAppVersions(db); n != 1 {
		t.Fatalf("첫 기동 행=%d", n)
	}
	s.StartedAt = "2026-09-13 15:00"
	if err := RecordAppStart(db, s); err != nil {
		t.Fatal(err)
	}
	if n := CountAppVersions(db); n != 1 {
		t.Fatalf("재기동에 행이 늘면 안 됨 n=%d", n)
	}
	var count int
	var last string
	if err := db.QueryRow(`SELECT start_count, last_started_at FROM app_versions`).Scan(&count, &last); err != nil {
		t.Fatal(err)
	}
	if count != 2 || last != "2026-09-13 15:00" {
		t.Fatalf("count=%d last=%s", count, last)
	}
}

func TestAppVersionsNewBuildAddsRow(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "v2.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := RecordAppStart(db, AppStart{Version: "v2.36", Commit: "oldold1", BuiltAt: "2026-09-11 09:10", StartedAt: "2026-09-11 09:10"}); err != nil {
		t.Fatal(err)
	}
	if err := RecordAppStart(db, AppStart{Version: "v2.37", Commit: "newnew1", BuiltAt: "2026-09-13 14:22", StartedAt: "2026-09-13 14:25"}); err != nil {
		t.Fatal(err)
	}
	if n := CountAppVersions(db); n != 2 {
		t.Fatalf("n=%d", n)
	}
}

func TestAppVersionsRollbackFlag(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "v3.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := RecordAppStart(db, AppStart{Version: "v2.36", Commit: "aaaaaaa", BuiltAt: "2026-09-13 14:22", StartedAt: "2026-09-13 14:25"}); err != nil {
		t.Fatal(err)
	}
	if err := RecordAppStart(db, AppStart{Version: "v2.35", Commit: "bbbbbbb", BuiltAt: "2026-09-10 18:40", StartedAt: "2026-09-13 16:00"}); err != nil {
		t.Fatal(err)
	}
	list, err := ListAppVersions(db, 20)
	if err != nil {
		t.Fatal(err)
	}
	var saw bool
	for _, r := range list {
		if r.Version == "v2.35" && r.Rollback {
			saw = true
		}
		if r.Version == "v2.36" && r.Rollback {
			t.Fatal("새 빌드에 롤백 표시가 붙었다")
		}
	}
	if !saw {
		t.Fatal("옛 빌드 재배포에 롤백 표시가 없다")
	}
}

func TestSchemaMismatchMessages(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "v4.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if msg := SchemaMismatchMessage(db); msg != "" {
		t.Fatalf("맞는데 경고=%q", msg)
	}
	if err := SetAppliedSchemaVersionForTest(db, 38); err != nil {
		t.Fatal(err)
	}
	msg := SchemaMismatchMessage(db)
	want := "DB 스키마가 낡았습니다 — " + SchemaNo(AppSchemaVersion) + " 필요, 현재 038"
	if msg != want {
		t.Fatalf("stale=%q want=%q", msg, want)
	}
	if err := SetAppliedSchemaVersionForTest(db, 50); err != nil {
		t.Fatal(err)
	}
	msg = SchemaMismatchMessage(db)
	if !strings.HasPrefix(msg, "DB 스키마가 앱보다 새 것입니다") {
		t.Fatalf("ahead=%q", msg)
	}
}

func TestRecordAppStartFailsWithoutKilling(t *testing.T) {
	if err := RecordAppStart(nil, AppStart{Version: "v2.37"}); err == nil {
		t.Fatal("nil db 는 실패해야 한다")
	}
	db, err := InitDB(filepath.Join(t.TempDir(), "v5.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err := RecordAppStart(db, AppStart{Version: "v2.37", Commit: "x", BuiltAt: "t", StartedAt: "s"}); err == nil {
		t.Fatal("닫힌 db 는 실패해야 한다")
	}
}

func TestInitDBStampsSchema42(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "v6.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if got := AppliedSchemaVersion(db); got != AppSchemaVersion {
		t.Fatalf("applied=%d want=%d", got, AppSchemaVersion)
	}
}

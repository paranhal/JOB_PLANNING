package repository

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestFTS5TrigramAvailableWithoutCGO(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "fts.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE VIRTUAL TABLE as_search USING fts5(
		as_id UNINDEXED, symptom, action, cause_detail, conclusion,
		tokenize='trigram'
	)`)
	if err != nil {
		t.Fatalf("FTS5+trigram 생성 실패 (LIKE 로 가야 함): %v", err)
	}

	_, err = db.Exec(`INSERT INTO as_search(as_id, symptom, action, cause_detail, conclusion)
		VALUES ('a1','무인예약이 안 됩니다','재시작 후 정상','설정','무인예약 장애 해소')`)
	if err != nil {
		t.Fatal(err)
	}

	var id string
	err = db.QueryRow(`SELECT as_id FROM as_search WHERE as_search MATCH ?`, `"무인예약"`).Scan(&id)
	if err != nil {
		t.Fatalf("trigram MATCH 무인예약 → 무인예약이: %v", err)
	}
	if id != "a1" {
		t.Fatalf("as_id=%s", id)
	}
}

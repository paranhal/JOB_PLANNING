package auditlog

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendWritesRowAndChainsHash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "access.db")
	w, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	w.Append(Record{Action: ActionDelete, Username: "admin", Detail: "first"})
	w.Append(Record{Action: ActionLogin, Username: "admin", Detail: "second"})
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	type row struct {
		action, prev, hash, at string
	}
	rows, err := db.Query(`SELECT action, prev_hash, row_hash, access_at FROM access_logs ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.action, &r.prev, &r.hash, &r.at); err != nil {
			t.Fatal(err)
		}
		got = append(got, r)
	}
	if len(got) != 2 {
		t.Fatalf("rows=%d", len(got))
	}
	if got[0].action != ActionDelete || got[1].action != ActionLogin {
		t.Fatalf("actions=%q %q", got[0].action, got[1].action)
	}
	if got[0].prev != "" {
		t.Fatalf("first prev_hash should be empty, got %q", got[0].prev)
	}
	if got[1].prev != got[0].hash {
		t.Fatalf("chain break: prev=%q want %q", got[1].prev, got[0].hash)
	}
	if got[0].hash == "" || got[1].hash == "" || got[0].hash == got[1].hash {
		t.Fatalf("hashes: %+v", got)
	}
	if !strings.Contains(got[0].at, ".") {
		t.Fatalf("access_at should include milliseconds: %q", got[0].at)
	}

	head, err := os.ReadFile(filepath.Join(dir, "chain.head"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(head)) != got[1].hash {
		t.Fatalf("chain.head=%q want %q", head, got[1].hash)
	}
}

func TestSchemaHasNoMACColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "access.db")
	w, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = w.Close()

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(`PRAGMA table_info(access_logs)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.ToLower(name), "mac") {
			t.Fatalf("MAC 컬럼이 있으면 안 된다: %s", name)
		}
	}
}

func TestExecRejectsReadsAndMutations(t *testing.T) {
	w, err := Open(filepath.Join(t.TempDir(), "access.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	for _, q := range []string{
		"SELECT 1",
		"UPDATE access_logs SET result='x'",
		"DELETE FROM access_logs",
		"PRAGMA table_info(access_logs)",
	} {
		if err := w.exec(q); err == nil {
			t.Fatalf("허용되면 안 된다: %s", q)
		}
	}
}

func TestWriterSourceHasNoQueryAPI(t *testing.T) {
	src, err := os.ReadFile("writer.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]byte{[]byte(".Query("), []byte(".QueryRow(")} {
		if bytes.Contains(src, bad) {
			t.Fatalf("writer.go 에 조회 API가 있다: %s", bad)
		}
	}
}

func TestAppendWithoutInitIsNoop(t *testing.T) {
	Close()
	Append(Record{Action: ActionDelete}) // panic 없어야 한다
}

func TestSessionFingerprintStableAndNotRaw(t *testing.T) {
	tok := "header.payload.sig"
	a := SessionFingerprint(tok)
	b := SessionFingerprint(tok)
	if a == "" || a != b {
		t.Fatalf("fingerprint: %q %q", a, b)
	}
	if a == tok || strings.Contains(a, "payload") {
		t.Fatal("세션 식별자에 토큰 원문이 남음")
	}
}

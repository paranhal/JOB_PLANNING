package repository

import (
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func appDBPath() string {
	candidates := []string{
		filepath.Join("..", "..", "data", "app.db"),
		filepath.Join("data", "app.db"),
	}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func openAppDBCopy(t *testing.T) *sql.DB {
	t.Helper()
	src := appDBPath()
	if src == "" {
		t.Skip("server/data/app.db 없음")
	}
	dir := t.TempDir()
	dst := filepath.Join(dir, "app.db")
	for _, ext := range []string{"", "-wal", "-shm"} {
		in := src + ext
		if _, err := os.Stat(in); err != nil {
			continue
		}
		if err := copyFile(in, dst+ext); err != nil {
			t.Fatal(err)
		}
	}
	db, err := sql.Open("sqlite", dst)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		t.Fatal(err)
	}
	return db
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func TestListWorkToday_AppDBTimingAndCount(t *testing.T) {
	db := openAppDBCopy(t)
	today := time.Now().Format("2006-01-02")
	repo := NewWorkBoardRepo(db)

	start := time.Now()
	items, err := repo.ListWorkToday("", nil, today)
	if err != nil {
		t.Fatal(err)
	}
	dur := time.Since(start)
	t.Logf("ListWorkToday n=%d dur=%s today=%s", len(items), dur, today)
	if len(items) != 65 {
		t.Errorf("목록 건수=%d want 65 (고치기 전과 같아야 한다)", len(items))
	}

	applyWorkListIndexes(db)
	rows, err := db.Query(`EXPLAIN QUERY PLAN
		SELECT ar.as_id FROM as_receipts ar
		WHERE ar.status IN ('received','assigned','in_progress','hold','transfer')
		  AND ar.visit_scheduled_date != ''
		  AND ar.visit_scheduled_date <= ?`, today)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan []string
	hasSearch, hasScanReceipts := false, false
	for rows.Next() {
		var sel, orig, notused int
		var detail string
		if err := rows.Scan(&sel, &orig, &notused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
		d := strings.ToUpper(detail)
		if strings.Contains(d, "AS_RECEIPTS") && strings.Contains(d, "SEARCH") {
			hasSearch = true
		}
		if strings.Contains(d, "SCAN") && strings.Contains(d, "AS_RECEIPTS") {
			hasScanReceipts = true
		}
	}
	t.Logf("EXPLAIN %s", strings.Join(plan, " | "))
	if !hasSearch || hasScanReceipts {
		t.Errorf("as_receipts 가 SEARCH 가 아니다: %s", strings.Join(plan, " | "))
	}
}

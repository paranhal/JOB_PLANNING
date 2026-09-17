package repository

import (
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
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

func openAppDBCopyMigrated(t *testing.T) *sql.DB {
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
	db, err := InitDB(dst)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
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

func TestMeetingAndAS_AppDBTiming(t *testing.T) {
	db := openAppDBCopyMigrated(t)
	today := time.Now().Format("2006-01-02")
	prev := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	work := NewWorkBoardRepo(db)
	stats := NewStatsRepo(db)
	asRepo := NewASRepo(db)

	start := time.Now()
	yesterday, err := work.ListCompletedOn(prev, "", nil, 300)
	if err != nil {
		t.Fatal(err)
	}
	todayDone, err := work.ListCompletedOn(today, "", nil, 300)
	if err != nil {
		t.Fatal(err)
	}
	todayItems, err := work.ListScheduledOn(today, "", nil, 300)
	if err != nil {
		t.Fatal(err)
	}
	unplanned, _, err := work.ListUnplanned("", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	progress, err := work.ListBucketOn(model.WorkBucketInProgress, today, "", nil, 300)
	if err != nil {
		t.Fatal(err)
	}
	listDur := time.Since(start)

	ovStart := time.Now()
	cols := BuildStatsPeriodColumns(model.StatsViewDay, time.Now())
	if err := stats.FillPeriodOverview(cols, ParseMeetingFilter(model.StatsScopeTeam, "", "")); err != nil {
		t.Fatal(err)
	}
	ovDur := time.Since(ovStart)
	meetingDur := listDur + ovDur

	asStart := time.Now()
	items, total, err := asRepo.ListFiltered("", "", "", nil, "receipt", "desc", 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	asListDur := time.Since(asStart)

	asID := ""
	if err := db.QueryRow(`SELECT as_id FROM as_receipts ORDER BY receipt_datetime DESC LIMIT 1`).Scan(&asID); err != nil {
		t.Fatal(err)
	}
	showStart := time.Now()
	got, err := asRepo.GetByID(asID)
	if err != nil || got == nil {
		t.Fatalf("GetByID: %v", err)
	}
	asShowDur := time.Since(showStart)

	t.Logf("meeting lists n_y=%d n_done=%d n_sched=%d n_unplanned=%d n_progress=%d dur=%s",
		len(yesterday), len(todayDone), len(todayItems), len(unplanned), len(progress), listDur)
	t.Logf("meeting FillPeriodOverview dur=%s", ovDur)
	t.Logf("meeting bundled (lists+overview) dur=%s", meetingDur)
	t.Logf("as list n_page=%d total=%d dur=%s", len(items), total, asListDur)
	t.Logf("as getByID id=%s dur=%s", asID, asShowDur)

	if listDur > 500*time.Millisecond {
		t.Errorf("/meeting 목록 조회 %s > 500ms", listDur)
	}
	if asListDur > 500*time.Millisecond {
		t.Errorf("/as 목록 %s > 500ms", asListDur)
	}
	if asShowDur > 500*time.Millisecond {
		t.Errorf("/as/{id} GetByID %s > 500ms", asShowDur)
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
	hasSearch, hasScan := false, false
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
			hasScan = true
		}
	}
	t.Logf("meeting/as visit_scheduled_date EXPLAIN %s", strings.Join(plan, " | "))
	if !hasSearch || hasScan {
		t.Errorf("visit_scheduled_date 가 SEARCH 가 아니다: %s", strings.Join(plan, " | "))
	}

	src, err := os.ReadFile("work_board_repo.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), "date(ar.visit_scheduled_date)") || strings.Contains(string(src), "visit_scheduled_date < date('now'") {
		t.Fatal("날짜 전용 컬럼에 date() 가 남아 있다")
	}
	asSrc, err := os.ReadFile("as_repo.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(asSrc), "date(ar.visit_scheduled_date)") {
		t.Fatal("as_repo 가 visit_scheduled_date 를 date() 로 감싼다")
	}
}

package repository

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAppendixCUsesSQLStatusStatsCompleted(t *testing.T) {
	b, err := os.ReadFile("appendix_c.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	if strings.Contains(src, "('completed','closed','partial_complete')") {
		t.Fatal("appendix_c.go 가 완료 상태를 하드코딩한다")
	}
	if strings.Count(src, "model.SQLStatusStatsCompleted") < 3 {
		t.Fatal("V-2/V-5/V-6 이 SQLStatusStatsCompleted 를 써야 한다")
	}
}

func TestNoCompleteDatetimeUpdatedAtFallback(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "no_updated_at_fallback.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('C-FB','폴백도서관','폴백도서관',1,'O01')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, receipt_datetime, customer_id, symptom, urgency, status,
		assigned_to, complete_datetime, updated_at, org_id, data_origin
	) VALUES
		('AS-FB-empty','R-FB-E','2026-10-01 09:00:00','C-FB','증상','중','completed','테크',NULL,'2026-11-03 18:00:00','O01','app'),
		('AS-FB-done','R-FB-D','2026-10-01 09:00:00','C-FB','증상','중','completed','테크','2026-11-03 16:00:00','2026-11-03 18:00:00','O01','app')`); err != nil {
		t.Fatal(err)
	}

	items, err := NewWorkBoardRepo(db).ListCompletedOn("2026-11-03", "", nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	var sawEmpty, sawDone bool
	for _, it := range items {
		if it.RefID == "AS-FB-empty" || it.RefNumber == "R-FB-E" {
			sawEmpty = true
		}
		if it.RefID == "AS-FB-done" || it.RefNumber == "R-FB-D" {
			sawDone = true
		}
	}
	if sawEmpty {
		t.Fatal("complete_datetime 없는 완료 건이 updated_at 날짜로 집계됐다")
	}
	if !sawDone {
		t.Fatal("complete_datetime 있는 완료 건이 집계되지 않았다")
	}
}

func TestASUpdateFillsCompleteDatetimeOnComplete(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "as_complete.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now()
	nowStr := now.Format("2006-01-02 15:04:05")
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('C-CD','기관','기관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, receipt_datetime, customer_id, symptom, urgency, status,
		visit_scheduled_date, schedule_confirmed, assigned_to, created_at, updated_at
	) VALUES ('AS-CD','AS-CD',?,'C-CD','증상','중','in_progress',?,1,'테크',?,?)`,
		nowStr, now.Format("2006-01-02"), nowStr, nowStr); err != nil {
		t.Fatal(err)
	}

	as, err := NewASRepo(db).GetByID(OrgAll, "AS-CD")
	if err != nil || as == nil {
		t.Fatalf("get: %v", err)
	}
	as.Status = "completed"
	as.CompleteDatetime = nil
	if err := NewASRepo(db).Update(as); err != nil {
		t.Fatal(err)
	}
	got, err := NewASRepo(db).GetByID(OrgAll, "AS-CD")
	if err != nil || got == nil {
		t.Fatalf("reload: %v", err)
	}
	if got.CompleteDatetime == nil || got.CompleteDatetime.IsZero() {
		t.Fatal("완료 저장 시 complete_datetime 이 비면 안 됨")
	}
}

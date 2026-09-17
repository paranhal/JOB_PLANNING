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
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		b, err := os.ReadFile(e.Name())
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		for _, needle := range []string{
			"complete_datetime, ar.updated_at",
			"complete_datetime, updated_at",
			"complete_datetime,ar.updated_at",
		} {
			if strings.Contains(src, needle) {
				t.Errorf("%s: COALESCE(complete_datetime, updated_at) 폴백이 남아 있다", e.Name())
			}
		}
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

	as, err := NewASRepo(db).GetByID("AS-CD")
	if err != nil || as == nil {
		t.Fatalf("get: %v", err)
	}
	as.Status = "completed"
	as.CompleteDatetime = nil
	if err := NewASRepo(db).Update(as); err != nil {
		t.Fatal(err)
	}
	got, err := NewASRepo(db).GetByID("AS-CD")
	if err != nil || got == nil {
		t.Fatalf("reload: %v", err)
	}
	if got.CompleteDatetime == nil || got.CompleteDatetime.IsZero() {
		t.Fatal("완료 저장 시 complete_datetime 이 비면 안 됨")
	}
}

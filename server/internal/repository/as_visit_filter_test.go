package repository

import (
	"path/filepath"
	"testing"
	"time"
)

func TestListFiltered_VisitBuckets(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "visit.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	today := time.Now().Format("2006-01-02")
	past := time.Now().AddDate(0, 0, -3).Format("2006-01-02")
	future := time.Now().AddDate(0, 0, 5).Format("2006-01-02")
	now := time.Now().Format("2006-01-02 15:04:05")

	_, err = db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('C-T1','테스트기관','테스트기관',1)`)
	if err != nil {
		t.Fatal(err)
	}

	insertAS := func(id, num, visit, status string) {
		t.Helper()
		_, err := db.Exec(`INSERT INTO as_receipts (
			as_id, as_number, receipt_datetime, customer_id, symptom, urgency, status,
			visit_scheduled_date, schedule_confirmed, assigned_to, created_at, updated_at
		) VALUES (?,?,?,?, '증상','중',?,?,1,'테크',?,?)`,
			id, num, now, "C-T1", status, visit, now, now)
		if err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}
	insertAS("AS-P", "R2607-P01", past, "in_progress")
	insertAS("AS-T", "R2607-T01", today, "in_progress")
	insertAS("AS-U", "R2607-U01", future, "in_progress")
	insertAS("AS-C", "R2607-C01", past, "completed") // 완료는 버킷 제외

	repo := NewASRepo(db)
	stats, err := repo.DashboardStats("", nil)
	if err != nil {
		t.Fatalf("DashboardStats: %v", err)
	}
	if stats.VisitPast != 1 || stats.VisitToday != 1 || stats.VisitUpcoming != 1 {
		t.Fatalf("stats past=%d today=%d up=%d want 1,1,1 (in_progress=%d)",
			stats.VisitPast, stats.VisitToday, stats.VisitUpcoming, stats.InProgress)
	}

	pastItems, n, err := repo.ListFiltered("visit_past", "", "", nil, "visit", "asc", 1, 20)
	if err != nil || n != 1 || len(pastItems) != 1 || pastItems[0].ASNumber != "R2607-P01" {
		t.Fatalf("visit_past: n=%d items=%v err=%v", n, pastItems, err)
	}
	if pastItems[0].VisitDaysOverdue < 1 {
		t.Fatalf("expected overdue days >0 got %d", pastItems[0].VisitDaysOverdue)
	}

	todayItems, n, err := repo.ListFiltered("visit_today", "", "", nil, "visit", "asc", 1, 20)
	if err != nil || n != 1 || todayItems[0].ASNumber != "R2607-T01" || todayItems[0].VisitDaysOverdue != 0 {
		t.Fatalf("visit_today: n=%d item=%+v err=%v", n, todayItems, err)
	}

	upItems, n, err := repo.ListFiltered("visit_upcoming", "", "", nil, "visit", "asc", 1, 20)
	if err != nil || n != 1 || upItems[0].ASNumber != "R2607-U01" || upItems[0].VisitDaysOverdue >= 0 {
		t.Fatalf("visit_upcoming: n=%d item=%+v err=%v", n, upItems, err)
	}

	insertAS("AS-X", "R2607-X01", past, "transfer")
	insertAS("AS-XF", "R2607-XF1", future, "transfer")
	if stats2, err := repo.DashboardStats("", nil); err != nil {
		t.Fatal(err)
	} else if stats2.TransferOverdue < 1 {
		t.Fatalf("transfer overdue want >=1 got %d", stats2.TransferOverdue)
	}
	overItems, n, err := repo.ListFiltered("transfer_overdue", "", "", nil, "visit", "asc", 1, 20)
	if err != nil || n < 1 {
		t.Fatalf("transfer_overdue: n=%d err=%v", n, err)
	}
	found := false
	for _, it := range overItems {
		if it.ASNumber == "R2607-X01" {
			found = true
		}
		if it.ASNumber == "R2607-XF1" {
			t.Fatal("future followup should not be overdue")
		}
	}
	if !found {
		t.Fatal("past transfer missing from overdue")
	}
}

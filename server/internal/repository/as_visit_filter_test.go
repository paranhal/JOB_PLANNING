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

// 일정을 조정해 예정일보다 늦게 다녀온 건은 '예정일 경과'가 아니라 '다음 일정 미정'이다.
func TestVisitPastExcludesAlreadyVisited(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "visit_done.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now().Format("2006-01-02 15:04:05")
	scheduled := time.Now().AddDate(0, 0, -9).Format("2006-01-02")
	visited := time.Now().AddDate(0, 0, -7).Format("2006-01-02") + " 13:30:00"

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('C-V','방문기관','방문기관',1)`); err != nil {
		t.Fatal(err)
	}
	insert := func(id, num string) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO as_receipts (
			as_id, as_number, receipt_datetime, customer_id, symptom, urgency, status,
			visit_scheduled_date, schedule_confirmed, assigned_to, created_at, updated_at
		) VALUES (?,?,?,'C-V','증상','중','in_progress',?,1,'테크',?,?)`,
			id, num, now, scheduled, now, now); err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}
	insert("AS-DONE", "R2605-001") // 예정일 이후 방문 이력 있음
	insert("AS-NONE", "R2605-002") // 아직 안 다녀온 건

	if _, err := db.Exec(`INSERT INTO as_processes (process_id, process_number, as_id, process_datetime, worker, work_content)
		VALUES ('P1','P1','AS-DONE',?,'테크','임시조치')`, visited); err != nil {
		t.Fatal(err)
	}

	repo := NewASRepo(db)
	stats, err := repo.DashboardStats("", nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.VisitPast != 1 {
		t.Fatalf("예정일 경과 = %d, want 1 (미방문 건만)", stats.VisitPast)
	}
	if stats.VisitDoneOpen != 1 {
		t.Fatalf("다음 일정 미정 = %d, want 1", stats.VisitDoneOpen)
	}

	pastItems, n, err := repo.ListFiltered("visit_past", "", "", nil, "visit", "asc", 1, 20)
	if err != nil || n != 1 || pastItems[0].ASNumber != "R2605-002" {
		t.Fatalf("visit_past: n=%d items=%+v err=%v", n, pastItems, err)
	}

	doneItems, n, err := repo.ListFiltered("visit_done_open", "", "", nil, "visit", "asc", 1, 20)
	if err != nil || n != 1 || doneItems[0].ASNumber != "R2605-001" {
		t.Fatalf("visit_done_open: n=%d items=%+v err=%v", n, doneItems, err)
	}
	if !doneItems[0].VisitDone {
		t.Fatal("VisitDone 플래그가 서지 않았다")
	}

	// 대시보드 지연 업무에서도 빠지고, 방문 미확정(다음 일정 미정)으로 잡힌다.
	wb := NewWorkBoardRepo(db)
	delayed, err := wb.ListBucket("delayed", "", nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range delayed {
		if it.RefNumber == "R2605-001" {
			t.Fatal("다녀온 건이 지연 업무에 남아 있다")
		}
	}
	pending, err := wb.ListBucket("schedule_pending", "", nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	foundPending := false
	for _, it := range pending {
		if it.RefNumber == "R2605-001" {
			foundPending = true
		}
	}
	if !foundPending {
		t.Fatal("다녀온 건이 방문 미확정 목록에 없다")
	}
}

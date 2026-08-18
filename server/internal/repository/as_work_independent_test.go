package repository

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

// 하부업무 완료는 원 접수 상태를 바꾸지 않는다.
func TestMarkDoneDoesNotChangeParentStatus(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "work_indep.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now().Format("2006-01-02 15:04:05")
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('C-I','나성동도서관','나성동도서관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, receipt_datetime, customer_id, symptom, urgency, status,
		visit_scheduled_date, schedule_confirmed, assigned_to, created_at, updated_at
	) VALUES ('R2606-007','R2606-007',?,'C-I','증상','중','partial_complete',?,1,'양기헌',?,?)`,
		now, time.Now().Format("2006-01-02"), now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_work_items (
		work_id, work_number, as_id, work_kind, scheduled_date, schedule_confirmed,
		assigned_to, status, notes, created_at, updated_at
	) VALUES ('W-007','R2606-007-W01','R2606-007','confirm',?,1,'최혜영','open','KLAS',?,?)`,
		time.Now().Format("2006-01-02"), now, now); err != nil {
		t.Fatal(err)
	}

	if err := NewASWorkRepo(db).MarkDone("W-007"); err != nil {
		t.Fatal(err)
	}
	var parentStatus, workStatus, parentAssignee, workAssignee string
	if err := db.QueryRow(`SELECT status, assigned_to FROM as_receipts WHERE as_id='R2606-007'`).
		Scan(&parentStatus, &parentAssignee); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT status, assigned_to FROM as_work_items WHERE work_id='W-007'`).
		Scan(&workStatus, &workAssignee); err != nil {
		t.Fatal(err)
	}
	if parentStatus != "partial_complete" {
		t.Fatalf("parent status=%s want partial_complete", parentStatus)
	}
	if parentAssignee != "양기헌" {
		t.Fatalf("parent assignee=%s want 양기헌", parentAssignee)
	}
	if workStatus != "done" {
		t.Fatalf("work status=%s want done", workStatus)
	}
	if workAssignee != "최혜영" {
		t.Fatalf("work assignee=%s want 최혜영", workAssignee)
	}
}

// 하부업무 담당자 변경이 원 접수 담당자를 덮어쓰지 않는다.
func TestWorkUpdateAssigneeIndependentOfParent(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "work_assignee.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now().Format("2006-01-02 15:04:05")
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('C-A','기관','기관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, receipt_datetime, customer_id, symptom, urgency, status,
		assigned_to, assigned_user_id, created_at, updated_at
	) VALUES ('AS-P','AS-P',?,'C-A','증상','중','in_progress','양기헌','U-YANG',?,?)`,
		now, now, now); err != nil {
		t.Fatal(err)
	}
	repo := NewASWorkRepo(db)
	w := &model.ASWorkItem{
		ASID: "AS-P", WorkKind: model.WorkKindConfirm,
		ScheduledDate: time.Now().Format("2006-01-02"), ScheduleConfirmed: true,
		AssignedTo: "최혜영", AssignedUserID: "U-CHOI", Status: "open", Notes: "KLAS",
	}
	if err := repo.Create(w); err != nil {
		t.Fatal(err)
	}
	w.AssignedTo = "최혜영"
	w.AssignedUserID = "U-CHOI"
	w.Notes = "점검 완료"
	if err := repo.Update(w); err != nil {
		t.Fatal(err)
	}

	var parentAssignee string
	if err := db.QueryRow(`SELECT assigned_to FROM as_receipts WHERE as_id='AS-P'`).Scan(&parentAssignee); err != nil {
		t.Fatal(err)
	}
	if parentAssignee != "양기헌" {
		t.Fatalf("parent assignee changed to %s", parentAssignee)
	}
	got, err := repo.GetByID(w.WorkID)
	if err != nil || got == nil {
		t.Fatalf("get: %v", err)
	}
	if got.AssignedTo != "최혜영" {
		t.Fatalf("work assignee=%s", got.AssignedTo)
	}
}

// 원 접수 담당자 변경이 하부업무 담당자를 덮어쓰지 않는다.
func TestParentAssigneeChangeDoesNotOverwriteWork(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "parent_assignee.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now()
	nowStr := now.Format("2006-01-02 15:04:05")
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('C-B','기관','기관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, receipt_datetime, customer_id, symptom, urgency, status,
		assigned_to, created_at, updated_at
	) VALUES ('AS-B','AS-B',?,'C-B','증상','중','partial_complete','양기헌',?,?)`,
		nowStr, nowStr, nowStr); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_work_items (
		work_id, work_number, as_id, work_kind, scheduled_date, schedule_confirmed,
		assigned_to, status, created_at, updated_at
	) VALUES ('W-B','AS-B-W01','AS-B','confirm',?,1,'최혜영','open',?,?)`,
		now.Format("2006-01-02"), nowStr, nowStr); err != nil {
		t.Fatal(err)
	}

	as, err := NewASRepo(db).GetByID("AS-B")
	if err != nil || as == nil {
		t.Fatalf("get: %v", err)
	}
	as.AssignedTo = "양기헌"
	as.AssignedUserID = "U-YANG"
	if err := NewASRepo(db).Update(as); err != nil {
		t.Fatal(err)
	}
	var workAssignee string
	if err := db.QueryRow(`SELECT assigned_to FROM as_work_items WHERE work_id='W-B'`).Scan(&workAssignee); err != nil {
		t.Fatal(err)
	}
	if workAssignee != "최혜영" {
		t.Fatalf("work assignee overwritten to %s", workAssignee)
	}
}

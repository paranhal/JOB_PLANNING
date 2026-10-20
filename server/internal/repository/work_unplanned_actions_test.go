package repository

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func setupUnplannedActions(t *testing.T) (*WorkBoardRepo, string) {
	t.Helper()
	db, err := InitDB(filepath.Join(t.TempDir(), "unplanned-act.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ts := time.Now().Format("2006-01-02 15:04:05")
	today := time.Now().Format("2006-01-02")
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('C1','기관A','기관A',1,'O01')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, receipt_datetime, customer_id, symptom, status, assigned_to, assigned_user_id, org_id, created_at, updated_at)
		VALUES ('A1','AS-1',?,'C1','증상','received','','','O01',?,?)`, ts, ts, ts); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_work_items (work_id, work_number, as_id, work_kind, status, assigned_to, assigned_user_id, created_at, updated_at)
		VALUES ('W1','AS-1-1','A1','confirm','open','','',?,?)`, ts, ts); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_plans (plan_id, plan_year, title, status) VALUES ('P1',2026,'계획','active')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits (visit_id, plan_id, visit_date, customer_id, completed, assignee)
		VALUES ('V1','P1',?,'C1',0,'')`, today); err != nil {
		t.Fatal(err)
	}
	wb := NewWBRepo(db)
	if err := wb.CreateTask(&model.WorkTask{WorkType: model.WBWorkAdmin, Title: "행정미배정", Status: model.WBTaskWaiting, OrgID: "O01"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_other (other_id, work_date, title, phase) VALUES ('G1',?,'기타', 'receipt')`, today); err != nil {
		t.Fatal(err)
	}
	sales := NewSalesRepo(db)
	p := &model.SalesProject{Name: "영업건", OrgID: "O01"}
	if err := sales.Create(p); err != nil {
		t.Fatal(err)
	}
	a := &model.SalesActivity{SalesID: p.SalesID, ActivityDate: today, ActivityType: "other", ActivityMethod: "call", Title: "콜"}
	if err := sales.CreateActivity(a, "테스터", nil); err != nil {
		t.Fatal(err)
	}
	return NewWorkBoardRepo(db), a.ActivityID
}

func unassignedKeys(t *testing.T, r *WorkBoardRepo) map[string]bool {
	t.Helper()
	items, err := r.CollectUnassigned("O01")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, it := range items {
		key := ""
		if strings.Contains(it.Href, "/as/work/") {
			key = "wi:" + it.RefID
		} else {
			switch it.Prefix {
			case model.WorkPrefixAS:
				key = "as:" + it.RefID
			case model.WorkPrefixMaintenance:
				key = "mnt:" + it.RefID
			default:
				key = "task:" + it.RefID
			}
		}
		out[key] = true
	}
	return out
}

func TestAssignUnplannedDateSavesAssigneeByKind(t *testing.T) {
	r, actID := setupUnplannedActions(t)
	today := time.Now().Format("2006-01-02")
	cases := []struct {
		key, wantCol, wantID string
	}{
		{"as:A1", "assigned_to", "A1"},
		{"wi:W1", "assigned_to", "W1"},
		{"mnt:V1", "assignee", "V1"},
	}
	for _, tc := range cases {
		if err := r.AssignUnplannedDate(tc.key, today, "홍길동", "U-h"); err != nil {
			t.Fatalf("%s assign: %v", tc.key, err)
		}
	}
	keys := unassignedKeys(t, r)
	for _, k := range []string{"as:A1", "wi:W1", "mnt:V1"} {
		if keys[k] {
			t.Fatalf("배정 후에도 미배정: %s", k)
		}
	}
	var asTo, wiTo, mntTo string
	_ = r.db.QueryRow(`SELECT COALESCE(assigned_to,'') FROM as_receipts WHERE as_id='A1'`).Scan(&asTo)
	_ = r.db.QueryRow(`SELECT COALESCE(assigned_to,'') FROM as_work_items WHERE work_id='W1'`).Scan(&wiTo)
	_ = r.db.QueryRow(`SELECT COALESCE(assignee,'') FROM maintenance_visits WHERE visit_id='V1'`).Scan(&mntTo)
	if asTo == "" || wiTo == "" || mntTo == "" {
		t.Fatalf("담당자 미저장 as=%q wi=%q mnt=%q", asTo, wiTo, mntTo)
	}
	if err := r.AssignUnplannedDate("salesact:"+actID, today, "홍길동", ""); err != nil {
		t.Fatal(err)
	}
	t.Log("salesact assign ok")
	if err := r.AssignUnplannedDate("slot:P1|C1|klas", today, "홍길동", ""); err != nil {
		t.Fatal(err)
	}
	if err := r.AssignUnplannedDate("gen:G1", today, "홍길동", ""); err != nil {
		t.Fatal(err)
	}
}

func TestAssignUnplannedMissingDate(t *testing.T) {
	r, _ := setupUnplannedActions(t)
	if err := r.AssignUnplannedDate("as:A1", "", "홍길동", ""); err == nil {
		t.Fatal("날짜 없이 성공")
	}
	keys := unassignedKeys(t, r)
	if !keys["as:A1"] {
		t.Fatal("실패 후 AS가 미배정에서 빠졌다")
	}
}

func TestTransferUnplannedByKind(t *testing.T) {
	r, actID := setupUnplannedActions(t)
	today := time.Now().Format("2006-01-02")
	_ = r.AssignUnplannedDate("as:A1", today, "홍길동", "")
	if err := r.TransferUnplanned("as:A1", "이관대상", "", today); err != nil {
		t.Fatal(err)
	}
	var to string
	_ = r.db.QueryRow(`SELECT COALESCE(assigned_to,'') FROM as_receipts WHERE as_id='A1'`).Scan(&to)
	if to != "이관대상" {
		t.Fatalf("AS 이관=%q", to)
	}
	if err := r.TransferUnplanned("wi:W1", "이관대상", "", today); err != nil {
		t.Fatal(err)
	}
	_ = r.db.QueryRow(`SELECT COALESCE(assigned_to,'') FROM as_work_items WHERE work_id='W1'`).Scan(&to)
	if to != "이관대상" {
		t.Fatalf("wi 이관=%q", to)
	}
	if err := r.TransferUnplanned("mnt:V1", "이관대상", "", today); err != nil {
		t.Fatal(err)
	}
	if err := r.TransferUnplanned("gen:G1", "이관대상", "", today); err == nil {
		t.Fatal("gen 이관이 조용히 성공")
	}
	if err := r.TransferUnplanned("salesact:"+actID, "이관대상", "", today); err != nil {
		t.Fatal(err)
	}
	if err := r.TransferUnplanned("xx:1", "이관대상", "", today); err == nil {
		t.Fatal("미지원 이관이 성공")
	}
}

func TestHoldUnplannedExcludesFromUnassigned(t *testing.T) {
	r, actID := setupUnplannedActions(t)
	today := time.Now().Format("2006-01-02")
	if err := r.HoldUnplanned("as:A1", "부품대기", today); err != nil {
		t.Fatal(err)
	}
	var st, reason string
	_ = r.db.QueryRow(`SELECT status, COALESCE(hold_reason,'') FROM as_receipts WHERE as_id='A1'`).Scan(&st, &reason)
	if st != "hold" || reason != "부품대기" {
		t.Fatalf("AS 보류 status=%s reason=%s", st, reason)
	}
	if err := r.HoldUnplanned("wi:W1", "일정조정", today); err != nil {
		t.Fatal(err)
	}
	if err := r.HoldUnplanned("mnt:V1", "고객요청", today); err != nil {
		t.Fatal(err)
	}
	taskID := ""
	_ = r.db.QueryRow(`SELECT task_id FROM work_tasks WHERE title='행정미배정'`).Scan(&taskID)
	if err := r.HoldUnplanned("task:"+taskID, "내부조정", today); err != nil {
		t.Fatal(err)
	}
	var hold string
	_ = r.db.QueryRow(`SELECT COALESCE(hold_reason,'') FROM work_tasks WHERE task_id=?`, taskID).Scan(&hold)
	if hold != "내부조정" {
		t.Fatalf("task hold=%q", hold)
	}
	keys := unassignedKeys(t, r)
	for _, k := range []string{"as:A1", "wi:W1", "mnt:V1", "task:" + taskID} {
		if keys[k] {
			t.Fatalf("보류 건이 미배정에 남음 %s", k)
		}
	}
	if err := r.HoldUnplanned("as:A1", "", today); err == nil {
		t.Fatal("사유 없이 보류")
	}
	if err := r.HoldUnplanned("salesact:"+actID, "다음주", today); err != nil {
		t.Fatal(err)
	}
	if err := r.HoldUnplanned("bogus:1", "x", today); err == nil {
		t.Fatal("미지원 보류 성공")
	}
}

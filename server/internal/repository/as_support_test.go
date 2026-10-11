package repository

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestCreateSupportFiveTypesAndDoesNotCloseAS(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "sup5.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().Format("2006-01-02 15:04:05")
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('C-S','기관','기관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, receipt_datetime, customer_id, symptom, urgency, status,
		assigned_to, created_at, updated_at
	) VALUES ('AS-S','AS-S',?,'C-S','증상','중','in_progress','양기헌',?,?)`, now, now, now); err != nil {
		t.Fatal(err)
	}
	repo := NewASWorkRepo(db)
	types := []model.ASSupportDetail{
		{SupportType: model.SupportFactoryInbound, VendorName: "한빛", DueDate: "2026-10-20"},
		{SupportType: model.SupportFactoryReturn, VendorName: "한빛", DueDate: "2026-10-25"},
		{SupportType: model.SupportLoanerSend, AssetID: "A1"},
		{SupportType: model.SupportCourier, TrackingNo: "123456"},
		{SupportType: model.SupportOther, Memo: "고객 안내"},
	}
	nums := map[string]bool{}
	for _, d := range types {
		w := &model.ASWorkItem{ASID: "AS-S"}
		if err := repo.CreateSupport(w, d); err != nil {
			t.Fatalf("%s: %v", d.SupportType, err)
		}
		if w.WorkKind != model.WorkKindSupport {
			t.Fatal(w.WorkKind)
		}
		if nums[w.WorkNumber] {
			t.Fatalf("번호 중복 %s", w.WorkNumber)
		}
		nums[w.WorkNumber] = true
		got, err := repo.GetDetail(w.WorkID)
		if err != nil || got == nil || got.SupportType != d.SupportType {
			t.Fatalf("detail %+v %v", got, err)
		}
	}
	var st string
	if err := db.QueryRow(`SELECT status FROM as_receipts WHERE as_id='AS-S'`).Scan(&st); err != nil {
		t.Fatal(err)
	}
	if st != "in_progress" {
		t.Fatalf("AS closed: %s", st)
	}
	n, err := repo.CountOpenSupport("AS-S")
	if err != nil || n != 5 {
		t.Fatalf("open=%d %v", n, err)
	}
}

func TestSupportCompleteDoesNotChangeParent(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "supdone.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().Format("2006-01-02 15:04:05")
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('C-S','기관','기관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, receipt_datetime, customer_id, symptom, urgency, status,
		assigned_to, created_at, updated_at
	) VALUES ('AS-S','AS-S',?,'C-S','증상','중','in_progress','양기헌',?,?)`, now, now, now); err != nil {
		t.Fatal(err)
	}
	repo := NewASWorkRepo(db)
	w := &model.ASWorkItem{ASID: "AS-S", AssignedTo: "최혜영"}
	if err := repo.CreateSupport(w, model.ASSupportDetail{SupportType: model.SupportOther, Memo: "메모"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkDone(w.WorkID); err != nil {
		t.Fatal(err)
	}
	var pst, wst, pass, wass string
	_ = db.QueryRow(`SELECT status, assigned_to FROM as_receipts WHERE as_id='AS-S'`).Scan(&pst, &pass)
	_ = db.QueryRow(`SELECT status, assigned_to FROM as_work_items WHERE work_id=?`, w.WorkID).Scan(&wst, &wass)
	if pst != "in_progress" || pass != "양기헌" {
		t.Fatalf("parent %s %s", pst, pass)
	}
	if wst != "done" || wass != "최혜영" {
		t.Fatalf("work %s %s", wst, wass)
	}
}

func TestASCompleteKeepsOpenSupport(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "supkeep.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now()
	nowStr := now.Format("2006-01-02 15:04:05")
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('C-S','기관','기관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, receipt_datetime, customer_id, symptom, urgency, status,
		assigned_to, created_at, updated_at
	) VALUES ('AS-S','AS-S',?,'C-S','증상','중','in_progress','양기헌',?,?)`, nowStr, nowStr, nowStr); err != nil {
		t.Fatal(err)
	}
	repo := NewASWorkRepo(db)
	w := &model.ASWorkItem{ASID: "AS-S"}
	if err := repo.CreateSupport(w, model.ASSupportDetail{SupportType: model.SupportOther, Memo: "메모"}); err != nil {
		t.Fatal(err)
	}
	as, err := NewASRepo(db).GetByID(OrgAll, "AS-S")
	if err != nil {
		t.Fatal(err)
	}
	as.Status = "completed"
	as.CompleteDatetime = &now
	if err := NewASRepo(db).Update(as); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.GetByID(w.WorkID)
	if got == nil || got.Status != "open" {
		t.Fatalf("support closed: %+v", got)
	}
}

func TestSupportShowsOnBoardNotAsRevisit(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "supboard.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().Format("2006-01-02 15:04:05")
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('C-S','기관','기관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, receipt_datetime, customer_id, symptom, urgency, status,
		assigned_to, created_at, updated_at
	) VALUES ('AS-S','AS-S',?,'C-S','증상','중','in_progress','양기헌',?,?)`, now, now, now); err != nil {
		t.Fatal(err)
	}
	repo := NewASWorkRepo(db)
	w := &model.ASWorkItem{ASID: "AS-S", ScheduledDate: time.Now().Format("2006-01-02")}
	if err := repo.CreateSupport(w, model.ASSupportDetail{SupportType: model.SupportCourier, TrackingNo: "1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE as_work_items SET assigned_to='', assigned_user_id='' WHERE work_id=?`, w.WorkID); err != nil {
		t.Fatal(err)
	}
	wb := NewWorkBoardRepo(db)
	items, err := wb.CollectUnassigned(OrgAll)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, it := range items {
		if it.RefID == w.WorkID {
			found = true
			if it.Prefix != model.WorkPrefixSupport || it.SubLabel != "지원" {
				t.Fatalf("board %+v", it)
			}
		}
	}
	if !found {
		t.Fatal("미배정에 지원 없음")
	}
}

func TestSupportMigrationRoundTrip(t *testing.T) {
	up, err := os.ReadFile(filepath.Join("..", "..", "migrations", "056_as_support_details.sql"))
	if err != nil {
		t.Fatal(err)
	}
	down, err := os.ReadFile(filepath.Join("..", "..", "migrations", "056_as_support_details.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(up), "as_support_details") {
		t.Fatal("up")
	}
	db, err := InitDB(filepath.Join(t.TempDir(), "mig.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(string(down)); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='as_support_details'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("down leftover n=%d %v", n, err)
	}
	if _, err := db.Exec(string(up)); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='as_support_details'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("up missing n=%d %v", n, err)
	}
}

func TestFactoryWaitInProgressAndNone(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "wait.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().Format("2006-01-02 15:04:05")
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('C-S','기관','기관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, receipt_datetime, customer_id, symptom, urgency, status, created_at, updated_at
	) VALUES ('AS-S','AS-S',?,'C-S','증상','중','in_progress',?,?)`, now, now, now); err != nil {
		t.Fatal(err)
	}
	repo := NewASWorkRepo(db)
	none := repo.FactoryWaitForAS("AS-S", "2026-10-20")
	if none.Label != "—" {
		t.Fatalf("none=%s", none.Label)
	}
	w := &model.ASWorkItem{ASID: "AS-S"}
	if err := repo.CreateSupport(w, model.ASSupportDetail{
		SupportType: model.SupportFactoryInbound, VendorName: "한빛", DueDate: "2026-10-20", ActionDate: "2026-10-13",
	}); err != nil {
		t.Fatal(err)
	}
	prog := repo.FactoryWaitForAS("AS-S", "2026-10-20")
	if prog.Mode != model.FactoryWaitInProgress {
		t.Fatalf("prog=%+v", prog)
	}
}

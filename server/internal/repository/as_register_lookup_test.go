package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestLookupForRegister_OrgLimitEscapeAndPlaced(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "aslookup.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O03','O03','SI','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c-seoch','서천도서관','서천도서관',1,'O01'),
		       ('c-empty','무AS도서관','무AS도서관',1,'O01'),
		       ('c-si','SI학교','SI학교',1,'O03')`); err != nil {
		t.Fatal(err)
	}
	ts := "2026-10-19 10:00:00"
	if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, receipt_datetime, customer_id, symptom, status, assigned_to, org_id, created_at, updated_at)
		VALUES ('as-open','R2610-001',?,'c-seoch','문 안 열림','in_progress','도서관담당A','O01',?,?),
		       ('as-place','R2610-075',?,'c-seoch','서천 점검','in_progress','도서관담당A','O01',?,?),
		       ('as-done','R2610-099',?,'c-seoch','완료증상','completed','도서관담당A','O01',?,?),
		       ('as-si','R2610-SI1',?,'c-si','SI증상','in_progress','SI담당C','O03',?,?)`,
		ts, ts, ts, ts, ts, ts, ts, ts, ts, ts, ts, ts); err != nil {
		t.Fatal(err)
	}
	as := NewASRepo(db)
	empty, err := as.LookupForRegister("O01", "서", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("1글자 %d", len(empty))
	}
	wild, err := as.LookupForRegister("O01", "%%", 20)
	if err != nil || len(wild) != 0 {
		t.Fatalf("와일드카드 %d %v", len(wild), err)
	}
	under, err := as.LookupForRegister("O01", "__", 20)
	if err != nil || len(under) != 0 {
		t.Fatalf("밑줄 %d %v", len(under), err)
	}
	hits, err := as.LookupForRegister("O01", "서천", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 3 {
		t.Fatalf("O01 서천 %d", len(hits))
	}
	for _, h := range hits {
		if h.ASID == "as-si" {
			t.Fatal("O01 결과에 O03")
		}
	}
	o03, err := as.LookupForRegister("O03", "서천", 20)
	if err != nil || len(o03) != 0 {
		t.Fatalf("O03이 O01을 봄 %d %v", len(o03), err)
	}
	num, err := as.LookupForRegister("O01", "R2610-075", 20)
	if err != nil || len(num) != 1 || num[0].ASID != "as-place" {
		t.Fatalf("번호 %+v %v", num, err)
	}

	cust, err := NewCustomerRepo(db).ExactCustomerIDsByOrgName("O01", "무AS도서관")
	if err != nil || len(cust) != 1 || cust[0] != "c-empty" {
		t.Fatalf("정확 고객 %+v %v", cust, err)
	}
	none, err := NewCustomerRepo(db).ExactCustomerIDsByOrgName("O03", "무AS도서관")
	if err != nil || len(none) != 0 {
		t.Fatalf("타조직 정확명 %+v %v", none, err)
	}

	wb := NewWBRepo(db)
	if err := wb.CreateTask(&model.WorkTask{
		WorkType: model.WBWorkAS, Title: "서천배치", OrgID: model.OrgIDLibrary,
		Status: model.WBTaskWaiting, SourceType: model.WBSourceAS, SourceID: "as-place",
		WorkDate: "2026-10-19", StartTime: "09:00", EndTime: "09:30", Assignee: "도서관담당A",
	}); err != nil {
		t.Fatal(err)
	}
	placed, err := wb.LatestPlacedASTasks([]string{"as-place", "as-open"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := placed["as-place"]; !ok {
		t.Fatal("배치 누락")
	}
	if _, ok := placed["as-open"]; ok {
		t.Fatal("미배치가 배치됨")
	}
}

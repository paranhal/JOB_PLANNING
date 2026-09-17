package repository

import (
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func TestMatchProductKeys(t *testing.T) {
	if !matchProductKeys(model.ProductKeySejongKLAS, "세종KLAS") {
		t.Fatal("세종KLAS 는 sejong_klas")
	}
	if matchProductKeys(model.ProductKeyKLAS, "세종KLAS") {
		t.Fatal("세종KLAS 는 klas 가 아니다")
	}
	if !matchProductKeys(model.ProductKeyKLAS, "도서관 KLAS") {
		t.Fatal("KLAS 는 klas")
	}
	if !matchProductKeys(model.ProductKeyAnrobotics, "RFID 게이트") {
		t.Fatal("RFID 는 anrobotics")
	}
	if !matchProductKeys("", "아무거나") {
		t.Fatal("제품키 비면 통과")
	}
}

func TestResolveProjectIDSejongTwoLevel(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "sejong.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`
		INSERT INTO customers (customer_id, org_name, official_name, is_active, has_parent, parent_customer_id) VALUES
		('C000-26-005','세종시교육청','세종시교육청',1,0,NULL),
		('C044-26-002','세종중간','세종중간',1,1,'C000-26-005'),
		('C_SITE','세종도서관','세종도서관',1,1,'C044-26-002'),
		('ANR','앤로사이트','앤로사이트',1,0,NULL)`)
	if err != nil {
		t.Fatal(err)
	}

	repo := NewProjectRepo(db)
	if err := repo.ReplaceRules("WPSEED02", []model.ProjectScopeRule{{
		ParentCustomerID: "C000-26-005",
		ProductKeys:      model.ProductKeySejongKLAS,
		WorkKinds:        model.ScopeWorkAS + "," + model.ScopeWorkMaintenance,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceRules("WPSEED03", []model.ProjectScopeRule{{
		ParentCustomerID: "C000-26-005",
		ProductKeys:      model.ProductKeyAnrobotics,
		WorkKinds:        model.ScopeWorkAS + "," + model.ScopeWorkMaintenance,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceRules("WPSEED05", []model.ProjectScopeRule{{
		ParentCustomerID: "C000-26-005",
		ProductKeys:      model.ProductKeyKLAS + "," + model.ProductKeySejongKLAS + "," + model.ProductKeyAnrobotics,
		WorkKinds:        model.ScopeWorkAS + "," + model.ScopeWorkMaintenance,
	}}); err != nil {
		t.Fatal(err)
	}

	anc, err := repo.customerAncestorIDs("C_SITE")
	if err != nil {
		t.Fatal(err)
	}
	if len(anc) != 3 || anc[0] != "C_SITE" || anc[1] != "C044-26-002" || anc[2] != "C000-26-005" {
		t.Fatalf("조상=%v want C_SITE → C044-26-002 → C000-26-005", anc)
	}

	pid, err := repo.ResolveProjectID("C_SITE", "세종KLAS", model.ScopeWorkMaintenance)
	if err != nil {
		t.Fatal(err)
	}
	if pid != "WPSEED02" {
		t.Fatalf("2단 상위기관 매칭 실패: got %q want WPSEED02", pid)
	}

	asPID, err := repo.ResolveProjectID("C_SITE", "세종 KLAS 로그인", model.ScopeWorkAS)
	if err != nil || asPID != "WPSEED02" {
		t.Fatalf("AS 2단 매칭: %q err=%v", asPID, err)
	}

	empty, err := repo.ResolveProjectID("ANR", "앤로보틱스 RFID", model.ScopeWorkMaintenance)
	if err != nil {
		t.Fatal(err)
	}
	if empty != "" {
		t.Fatalf("상위기관 없는 앤로보틱스는 미배정이어야 함: %q", empty)
	}

	plan, err := NewMaintenanceRepo(db).CreatePlan(2031, "세종테스트")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO maintenance_visits (visit_id, plan_id, visit_date, customer_id, product_type)
		VALUES ('V-SJ','` + plan.PlanID + `','2026-08-10','C_SITE','세종KLAS'),
		       ('V-AN','` + plan.PlanID + `','2026-08-11','ANR','앤로보틱스')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, status, symptom)
		VALUES ('R-SJ','R-SJ','C_SITE','2026-08-10 09:00:00','received','세종KLAS 장애'),
		       ('R-AN','R-AN','ANR','2026-08-10 09:00:00','received','RFID 게이트')`); err != nil {
		t.Fatal(err)
	}

	backfillStoredProjectIDs(db)

	var vSJ, vAN, aSJ, aAN string
	_ = db.QueryRow(`SELECT COALESCE(project_id,'') FROM maintenance_visits WHERE visit_id='V-SJ'`).Scan(&vSJ)
	_ = db.QueryRow(`SELECT COALESCE(project_id,'') FROM maintenance_visits WHERE visit_id='V-AN'`).Scan(&vAN)
	_ = db.QueryRow(`SELECT COALESCE(project_id,'') FROM as_receipts WHERE as_id='R-SJ'`).Scan(&aSJ)
	_ = db.QueryRow(`SELECT COALESCE(project_id,'') FROM as_receipts WHERE as_id='R-AN'`).Scan(&aAN)
	if vSJ != "WPSEED02" || aSJ != "WPSEED02" {
		t.Fatalf("백필 세종 visit=%q as=%q want WPSEED02", vSJ, aSJ)
	}
	if vAN != "" || aAN != "" {
		t.Fatalf("해석 실패는 NULL 유지 visit=%q as=%q", vAN, aAN)
	}

	var visitNull int
	if err := db.QueryRow(`SELECT COUNT(*) FROM maintenance_visits WHERE project_id IS NULL`).Scan(&visitNull); err != nil {
		t.Fatal(err)
	}
	t.Logf("SELECT COUNT(*) FROM maintenance_visits WHERE project_id IS NULL → %d", visitNull)
	if visitNull != 1 {
		t.Fatalf("미해석 방문 건수=%d want 1 (V-AN)", visitNull)
	}

	unres, err := repo.ListUnresolvedProjectRows()
	if err != nil {
		t.Fatal(err)
	}
	if len(unres) < 2 {
		t.Fatalf("미배정 목록 %d건, AS·방문 실패분이 보여야 한다", len(unres))
	}
	var sawVisit, sawAS bool
	for _, row := range unres {
		if row.ID == "V-AN" {
			sawVisit = true
			if !strings.Contains(row.Reason, "상위기관") {
				t.Fatalf("앤로 방문 사유=%q", row.Reason)
			}
		}
		if row.ID == "R-AN" {
			sawAS = true
		}
	}
	if !sawVisit || !sawAS {
		t.Fatalf("미배정 목록에 실패 건이 없다 visit=%v as=%v rows=%d", sawVisit, sawAS, len(unres))
	}

	if err := NewMaintenanceRepo(db).InsertVisitFull(model.MaintenanceVisit{
		PlanID: plan.PlanID, VisitDate: "2026-08-12", CustomerID: "C_SITE", ProductType: "세종KLAS",
	}); err != nil {
		t.Fatal(err)
	}
	var stored string
	_ = db.QueryRow(`SELECT COALESCE(project_id,'') FROM maintenance_visits
		WHERE customer_id='C_SITE' AND visit_date='2026-08-12'`).Scan(&stored)
	if stored != "WPSEED02" {
		t.Fatalf("생성 시점에 저장되어야 함: %q", stored)
	}
}

func TestASReceiptsProjectIDColumn(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "col.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('as_receipts') WHERE name='project_id'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("as_receipts.project_id 없음 n=%d err=%v", n, err)
	}
}

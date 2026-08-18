package repository

import (
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func TestProjectSeedAndMatch(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, has_parent) VALUES
		('P_CN','충청남도교육청','충청남도교육청',1,0),
		('C_LIB','테스트도서관','테스트도서관',1,1)`)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = db.Exec(`UPDATE customers SET parent_customer_id='P_CN' WHERE customer_id='C_LIB'`)

	_, _ = db.Exec(`DELETE FROM id_sequences WHERE seq_key=?`, projectSeedMetaKey)
	seedDefaultProjects(db)

	repo := NewProjectRepo(db)
	list, err := repo.List(2026, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) < 5 {
		t.Fatalf("시드 사업 부족: %d", len(list))
	}

	p, err := repo.Get("WPSEED01")
	if err != nil {
		t.Fatal(err)
	}
	if p.PlanYear != 2026 || !p.IsPaid {
		t.Fatalf("WPSEED01 필드: year=%d paid=%v", p.PlanYear, p.IsPaid)
	}
	if len(p.ScopeRules) == 0 {
		t.Fatal("WPSEED01 규칙 없음")
	}
	rules := p.ScopeRules
	rules[0].ParentCustomerID = "P_CN"
	if err := repo.ReplaceRules(p.ProjectID, rules); err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`INSERT INTO assets (asset_id, customer_id, product_name, product_type)
		VALUES ('A1','C_LIB','도서관 KLAS','KLAS')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO as_receipts (as_id, as_number, customer_id, asset_id, receipt_datetime, symptom, status, urgency)
		VALUES ('AS1','R2601-001','C_LIB','A1','2026-03-01 10:00:00','KLAS 로그인 오류','received','normal')`)
	if err != nil {
		t.Fatal(err)
	}

	rows, total, err := repo.MatchAS("WPSEED01", 10)
	if err != nil {
		t.Fatal(err)
	}
	if total < 1 || len(rows) < 1 {
		t.Fatalf("AS 매칭 실패 total=%d rows=%d", total, len(rows))
	}

	np := &model.WorkProject{Name: "테스트사업", PlanYear: 2026, IsPaid: true, Status: model.WBProjectActive}
	if err := repo.Create(np); err != nil {
		t.Fatal(err)
	}
	np.ShortName = "테스트"
	if err := repo.Update(np); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(np.ProjectID)
	if err != nil || got.ShortName != "테스트" {
		t.Fatalf("update/get: %+v err=%v", got, err)
	}
	if err := repo.ReplaceRules(np.ProjectID, []model.ProjectScopeRule{{
		ParentCustomerID: "P_CN",
		ProductKeys:      model.ProductKeyKLAS,
		WorkKinds:        model.ScopeWorkAS,
		Notes:            "t",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(np.ProjectID); err != nil {
		t.Fatal(err)
	}
}

func TestProductKeysSQL(t *testing.T) {
	sqlFrag, _ := productKeysSQL(model.ProductKeyKLAS+","+model.ProductKeyAnrobotics, "a", "ar.symptom", true)
	if sqlFrag == "" || !strings.Contains(sqlFrag, "KLAS") || !strings.Contains(sqlFrag, "앤로") {
		t.Fatalf("unexpected sql: %s", sqlFrag)
	}
}

func TestDedupeWorkProjectsByName(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "dedupe.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	name := "세종시 도서관 ICT 통합정보시스템 유지관리(2026년)"
	_, err = db.Exec(`
		INSERT INTO work_projects (project_id, name, short_name, plan_year, is_paid, sort_order, color, status)
		VALUES ('WP-004', ?, '', 0, 1, 0, '#3B82F6', 'active')`, name)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('C1','기관','기관',1);
		INSERT INTO assets (asset_id, customer_id, product_name, project_id) VALUES ('A1','C1','KLAS','WP-004');
		INSERT INTO work_tasks (task_id, project_id, title, status) VALUES ('T1','WP-004','업무','waiting')`)
	if err != nil {
		t.Fatal(err)
	}

	_, _ = db.Exec(`DELETE FROM id_sequences WHERE seq_key=?`, projectDedupeByNameMetaKey)
	dedupeWorkProjectsByName(db)

	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM work_projects WHERE TRIM(name)=?`, name).Scan(&n)
	if n != 1 {
		t.Fatalf("중복 미정리: %d", n)
	}
	var keep string
	_ = db.QueryRow(`SELECT project_id FROM work_projects WHERE TRIM(name)=?`, name).Scan(&keep)
	if keep != "WPSEED02" {
		t.Fatalf("시드 ID를 남겨야 함: %s", keep)
	}
	var assetPID, taskPID string
	_ = db.QueryRow(`SELECT project_id FROM assets WHERE asset_id='A1'`).Scan(&assetPID)
	_ = db.QueryRow(`SELECT project_id FROM work_tasks WHERE task_id='T1'`).Scan(&taskPID)
	if assetPID != "WPSEED02" || taskPID != "WPSEED02" {
		t.Fatalf("참조 미이전 asset=%s task=%s", assetPID, taskPID)
	}
	var old int
	_ = db.QueryRow(`SELECT COUNT(*) FROM work_projects WHERE project_id='WP-004'`).Scan(&old)
	if old != 0 {
		t.Fatal("중복 WP-004가 남아 있음")
	}
}

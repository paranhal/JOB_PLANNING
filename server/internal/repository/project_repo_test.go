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

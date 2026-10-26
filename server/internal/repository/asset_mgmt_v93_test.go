package repository

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func seedV93Distribution(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order)
		VALUES('O02','O02','SI사업팀','business',1,20),('O03','O03','미디어','business',1,30)`)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ id, name, org string }{
		{"c-lib", "도서관본관", model.OrgIDLibrary},
		{"c-si", "SI고객", "O02"},
		{"c-media", "미디어고객", "O03"},
	} {
		_, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
			VALUES (?,?,?,1,?)`, c.id, c.name, c.name, c.org)
		if err != nil {
			t.Fatal(err)
		}
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	st, err := tx.Prepare(`INSERT INTO assets (asset_id, customer_id, product_name, management_type, maint_contract_type, maint_cycle, org_id)
		VALUES (?,?,?,?,?,?,?)`)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	add := func(cust, mgmt, contract, cycle, org string) {
		t.Helper()
		n++
		id := fmt.Sprintf("A%04d", n)
		if _, err := st.Exec(id, cust, "장비"+id, mgmt, contract, cycle, org); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 400; i++ {
		add("c-lib", "third_party", "", "monthly", model.OrgIDLibrary)
	}
	for i := 0; i < 8; i++ {
		add("c-si", "third_party", "", "Call ", "O02")
	}
	// periodic 227: 12 none, rest paid/free
	for i := 0; i < 12; i++ {
		cy := "Call "
		if i == 0 {
			cy = "수시"
		}
		add("c-lib", "periodic", "none", cy, model.OrgIDLibrary)
	}
	for i := 0; i < 200; i++ {
		add("c-lib", "periodic", "paid", "monthly", model.OrgIDLibrary)
	}
	for i := 0; i < 15; i++ {
		add("c-lib", "periodic", "free", "수시", model.OrgIDLibrary)
	}
	// fault 100: 77 none (2 empty cycle), 8 paid, 15 free
	for i := 0; i < 77; i++ {
		cy := "Call "
		if i < 2 {
			cy = ""
		}
		add("c-lib", "fault", "none", cy, model.OrgIDLibrary)
	}
	for i := 0; i < 8; i++ {
		add("c-lib", "fault", "paid", "monthly", model.OrgIDLibrary)
	}
	for i := 0; i < 15; i++ {
		add("c-lib", "fault", "free", "Call ", model.OrgIDLibrary)
	}
	for i := 0; i < 4; i++ {
		add("c-lib", "direct", "paid", "monthly", model.OrgIDLibrary)
	}
	add("c-media", "on_demand", "free", "quarterly", "O03")
	for i := 0; i < 9; i++ {
		add("c-lib", "", "paid", "monthly", model.OrgIDLibrary)
	}
	for i := 0; i < 3; i++ {
		add("c-lib", "", "none", "Call ", model.OrgIDLibrary)
	}
	for i := 0; i < 9; i++ {
		add("c-lib", "", "", "", model.OrgIDLibrary)
	}
	st.Close()
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if n != 761 {
		t.Fatalf("seed %d", n)
	}
}

func countMgmt(t *testing.T, db *sql.DB, v string) int {
	t.Helper()
	var n int
	q := `SELECT COUNT(*) FROM assets WHERE TRIM(COALESCE(management_type,''))=?`
	if err := db.QueryRow(q, v).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAssetMgmtV93SeededDistribution(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "app.db")
	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	seedV93Distribution(t, db)

	bak, err := BackupAppDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if bak == "" {
		t.Fatal("백업 경로 없음")
	}
	if _, err := os.Stat(bak); err != nil {
		t.Fatal(err)
	}

	res, err := ApplyAssetMgmtTypesV93(db, true)
	if err != nil {
		t.Fatal(err)
	}
	if countMgmt(t, db, "third_party") != 408 {
		t.Fatalf("타사장비 %d", countMgmt(t, db, "third_party"))
	}
	if countMgmt(t, db, "periodic") != 241 {
		t.Fatalf("정기점검 %d", countMgmt(t, db, "periodic"))
	}
	if countMgmt(t, db, "fault") != 103 {
		t.Fatalf("장애대응 %d", countMgmt(t, db, "fault"))
	}
	if countMgmt(t, db, "") != 9 {
		t.Fatalf("미입력 %d", countMgmt(t, db, ""))
	}
	if countMgmt(t, db, "direct") != 0 || countMgmt(t, db, "on_demand") != 0 {
		t.Fatal("구 관리유형이 남아 있다")
	}
	var callN, emptyN int
	_ = db.QueryRow(`SELECT COUNT(*) FROM assets WHERE LOWER(TRIM(maint_contract_type))='none' AND maint_cycle='call'`).Scan(&callN)
	_ = db.QueryRow(`SELECT COUNT(*) FROM assets WHERE LOWER(TRIM(maint_contract_type))='none' AND TRIM(COALESCE(maint_cycle,''))=''`).Scan(&emptyN)
	if callN != 90 || emptyN != 2 {
		t.Fatalf("미계약 주기 call=%d empty=%d", callN, emptyN)
	}
	var paidCall, freeSusi, tpCall int
	_ = db.QueryRow(`SELECT COUNT(*) FROM assets WHERE maint_contract_type='paid' AND maint_cycle='Call '`).Scan(&paidCall)
	_ = db.QueryRow(`SELECT COUNT(*) FROM assets WHERE management_type='periodic' AND maint_contract_type='free' AND maint_cycle='수시'`).Scan(&freeSusi)
	_ = db.QueryRow(`SELECT COUNT(*) FROM assets WHERE management_type='third_party' AND maint_cycle='Call '`).Scan(&tpCall)
	if paidCall != 0 {
		t.Fatal("유상 Call 을 건드리면 안 됨")
	}
	if freeSusi != 15 {
		t.Fatalf("무상 수시 유지 %d", freeSusi)
	}
	if tpCall != 8 {
		t.Fatalf("타사장비 Call 유지 %d", tpCall)
	}
	if res.Contradict["fault+paid"] != 8 || res.Contradict["periodic+none"] != 12 {
		t.Fatalf("모순 %+v", res.Contradict)
	}
	if len(res.Unresolved) != 9 {
		t.Fatalf("unresolved %d", len(res.Unresolved))
	}

	out := filepath.Join(dir, "output")
	if err := WriteMigration093Reports(out, res); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "migration_093_changes.csv")); err != nil {
		t.Fatal(err)
	}

	res2, err := ApplyAssetMgmtTypesV93(db, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(res2.Changes) != 0 {
		t.Fatalf("두 번째 변경 %d", len(res2.Changes))
	}

	codes, err := NewCodeRepo(db).ActiveByGroup("management_type")
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != 3 {
		t.Fatalf("활성 관리유형 %d", len(codes))
	}
	cyc, _ := NewCodeRepo(db).ActiveByGroup("maint_cycle")
	foundCall := false
	for _, c := range cyc {
		if c.CodeValue == "call" {
			foundCall = true
		}
	}
	if !foundCall {
		t.Fatal("call 코드 없음")
	}
}

func TestAssetMgmtV93RevertCSV(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, _ = db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('c1','기관','기관',1)`)
	_, _ = db.Exec(`INSERT INTO assets (asset_id, customer_id, product_name, management_type, maint_contract_type, maint_cycle)
		VALUES ('X1','c1','p','direct','paid','monthly')`)
	res, err := ApplyAssetMgmtTypesV93(db, true)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "output")
	if err := WriteMigration093Reports(out, res); err != nil {
		t.Fatal(err)
	}
	n, err := RevertAssetMgmtFromCSV(db, filepath.Join(out, "migration_093_changes.csv"))
	if err != nil || n < 1 {
		t.Fatalf("revert n=%d err=%v", n, err)
	}
	var mgmt string
	_ = db.QueryRow(`SELECT management_type FROM assets WHERE asset_id='X1'`).Scan(&mgmt)
	if mgmt != "direct" {
		t.Fatalf("revert mgmt=%q", mgmt)
	}
}

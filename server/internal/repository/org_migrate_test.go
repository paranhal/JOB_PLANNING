package repository

import (
	"path/filepath"
	"testing"
)

func TestOrgSplitV266(t *testing.T) {
	path := filepath.Join(t.TempDir(), "org.db")
	db, err := InitDB(path)
	if err != nil {
		t.Fatal(err)
	}

	var orgN int
	var name string
	if err := db.QueryRow(`SELECT COUNT(*), MAX(org_name) FROM orgs`).Scan(&orgN, &name); err != nil {
		t.Fatal(err)
	}
	if orgN != 1 || name != "도서관사업팀" {
		t.Fatalf("orgs n=%d name=%q", orgN, name)
	}
	var oid, ono string
	if err := db.QueryRow(`SELECT org_id, org_no FROM orgs WHERE org_id='O01'`).Scan(&oid, &ono); err != nil {
		t.Fatal(err)
	}
	if oid != "O01" || ono != "O01" {
		t.Fatalf("seed id=%q no=%q", oid, ono)
	}

	for _, tbl := range orgRootTables {
		if !tableHasColumn(db, tbl, "org_id") {
			t.Fatalf("%s.org_id 없음", tbl)
		}
		var empty int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + tbl + ` WHERE org_id IS NULL OR TRIM(org_id)=''`).Scan(&empty); err != nil {
			t.Fatalf("%s empty count: %v", tbl, err)
		}
		if empty != 0 {
			t.Fatalf("%s org_id='' %d건", tbl, empty)
		}
	}
	if !tableHasColumn(db, "users", "org_id") {
		t.Fatal("users.org_id 없음")
	}

	for _, tbl := range []string{
		"customer_buildings", "as_processes", "maintenance_visits", "sales_activities", "work_actions",
	} {
		if tableHasColumn(db, tbl, "org_id") {
			t.Fatalf("자식 표 %s 에 org_id 가 있다", tbl)
		}
	}
	for _, tbl := range orgSharedTables {
		if tableHasColumn(db, tbl, "org_id") {
			t.Fatalf("공용 표 %s 에 org_id 가 있다", tbl)
		}
	}

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name) VALUES ('C-O02','이전고객','이전고객')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE customers SET org_id='O02' WHERE customer_id='C-O02'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (user_id, username, password_hash, full_name, role, is_active)
		VALUES ('u-va','vision','x','비젼관리자','vision_admin',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (user_id, username, password_hash, full_name, role, is_active)
		VALUES ('u-late','lateuser','x','늦가입','tech',1)`); err != nil {
		t.Fatal(err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db2, err := InitDB(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db2.Close() })

	var orgN2 int
	if err := db2.QueryRow(`SELECT COUNT(*) FROM orgs`).Scan(&orgN2); err != nil {
		t.Fatal(err)
	}
	if orgN2 != 1 {
		t.Fatalf("재부팅 후 orgs=%d", orgN2)
	}
	var moved string
	if err := db2.QueryRow(`SELECT org_id FROM customers WHERE customer_id='C-O02'`).Scan(&moved); err != nil {
		t.Fatal(err)
	}
	if moved != "O02" {
		t.Fatalf("O02 행이 되돌아갔다 org_id=%q", moved)
	}
	var vaOrg, lateOrg string
	if err := db2.QueryRow(`SELECT COALESCE(org_id,'') FROM users WHERE user_id='u-va'`).Scan(&vaOrg); err != nil {
		t.Fatal(err)
	}
	if err := db2.QueryRow(`SELECT COALESCE(org_id,'') FROM users WHERE user_id='u-late'`).Scan(&lateOrg); err != nil {
		t.Fatal(err)
	}
	if vaOrg != "" {
		t.Fatalf("비젼관리자가 조직에 들어갔다 org_id=%q", vaOrg)
	}
	if lateOrg != "" {
		t.Fatalf("가드 이후 빈 사용자가 O01 로 덮였다 org_id=%q", lateOrg)
	}

	if !metaDone(db2, orgSplitMetaKey) {
		t.Fatal("metaDone 이 안 섰다")
	}

	applyOrgSplitV266(db2)
	var orgN3 int
	if err := db2.QueryRow(`SELECT COUNT(*) FROM orgs`).Scan(&orgN3); err != nil {
		t.Fatal(err)
	}
	if orgN3 != 1 {
		t.Fatalf("가드 없이 다시 돈 듯 orgs=%d", orgN3)
	}
}

func TestOrgSplitVisionAdminSkippedOnFirstRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "org_va.db")
	db, err := InitDB(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`DELETE FROM id_sequences WHERE seq_key=?`, orgSplitMetaKey); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (user_id, username, password_hash, full_name, role, is_active, org_id)
		VALUES ('u-va2','va2','x','비젼','vision_admin',1,'')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (user_id, username, password_hash, full_name, role, is_active, org_id)
		VALUES ('u-tech2','tech2','x','기술','tech',1,'')`); err != nil {
		t.Fatal(err)
	}
	applyOrgSplitV266(db)

	var va, tech string
	if err := db.QueryRow(`SELECT org_id FROM users WHERE user_id='u-va2'`).Scan(&va); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT org_id FROM users WHERE user_id='u-tech2'`).Scan(&tech); err != nil {
		t.Fatal(err)
	}
	if va != "" {
		t.Fatalf("비젼관리자 org_id=%q", va)
	}
	if tech != "O01" {
		t.Fatalf("기술 org_id=%q", tech)
	}
}

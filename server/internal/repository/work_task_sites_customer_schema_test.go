package repository

import (
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func Test077WorkTaskSitesCustomerNoSeed(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "p77.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('work_task_sites') WHERE name='customer_id'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("customer_id n=%d err=%v", n, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('work_task_sites') WHERE name='site_id'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("site_id leftover n=%d", n)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_task_sites`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("시드 금지 sites=%d", n)
	}

	up, err := workTaskSitesCustomerMigrationSQL()
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(up, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "--") {
			continue
		}
		if strings.Contains(strings.ToUpper(trim), "INSERT INTO work_task_sites(") ||
			strings.HasPrefix(strings.ToUpper(trim), "INSERT INTO CUSTOMERS") {
			t.Fatal("077 up SQL 에 시드 INSERT 가 있으면 안 됨")
		}
	}

	down, err := workTaskSitesCustomerDownSQL()
	if err != nil {
		t.Fatal(err)
	}
	if err := execSQLIgnoreDupCol(db, down); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('work_task_sites') WHERE name='site_id'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("down site_id n=%d err=%v", n, err)
	}

	if _, err := db.Exec(`INSERT INTO work_projects(project_id,name,status,org_id,project_kind,project_kind_set)
		VALUES('WP77','공사','active',?,'construction',1)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers(customer_id,org_name,official_name,is_active,org_id)
		VALUES('C77','이관교','이관교',1,?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO project_sites(id,project_id,customer_id,site_name,status,org_id,is_active)
		VALUES('PS77','WP77','C77','이관교','planned',?,1)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO project_sites(id,project_id,site_name,status,org_id,is_active)
		VALUES('PS77X','WP77','미연결','planned',?,1)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks(task_id,work_type,title,status,org_id) VALUES('WT77','project_work','이관',?,?)`,
		model.WBTaskWaiting, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_task_sites(task_id,site_id) VALUES('WT77','PS77'),('WT77','PS77X')`); err != nil {
		t.Fatal(err)
	}
	var leftover int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM work_task_sites w
		LEFT JOIN project_sites s ON s.id=w.site_id
		WHERE TRIM(COALESCE(s.customer_id,''))=''`).Scan(&leftover); err != nil {
		t.Fatal(err)
	}
	if leftover != 1 {
		t.Fatalf("미연결 leftover=%d", leftover)
	}

	applyWorkTaskSitesCustomer(db)
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_task_sites`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("이관 후 n=%d err=%v", n, err)
	}
	var cid, psid string
	if err := db.QueryRow(`SELECT customer_id, COALESCE(project_site_id,'') FROM work_task_sites WHERE task_id='WT77'`).Scan(&cid, &psid); err != nil {
		t.Fatal(err)
	}
	if cid != "C77" || psid != "PS77" {
		t.Fatalf("이관 cid=%s psid=%s", cid, psid)
	}
}

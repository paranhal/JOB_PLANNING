package repository

import (
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func Test075ProjectSitesProgressNoSeedAndStatusMap(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "p75.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='project_site_events'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("project_site_events n=%d err=%v", n, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM project_site_events`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("시드 금지 events=%d err=%v", n, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM site_import_batches`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("시드 금지 batches=%d", n)
	}

	var ddl string
	if err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='project_sites'`).Scan(&ddl); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ddl, "in_progress") || !strings.Contains(ddl, "inspected") {
		t.Fatalf("status CHECK 미적용: %s", ddl)
	}

	if _, err := db.Exec(`INSERT INTO work_projects(project_id,name,org_id) VALUES('WP75','무선',?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers(customer_id,org_name,official_name,is_active,org_id,has_parent,org_code)
		VALUES('C75P','충남교육청','충남교육청',1,?,'0',''),( 'C75S','계룡고','계룡고',1,?,1,'G001')`, model.OrgIDLibrary, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE customers SET parent_customer_id='C75P' WHERE customer_id='C75S'`); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(`INSERT INTO project_sites(id,project_id,customer_id,site_name,status,org_id)
		VALUES('S75','WP75','C75S','계룡고','in_progress',?)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO project_sites(id,project_id,customer_id,site_name,status,org_id)
		VALUES('S75b','WP75','C75S','계룡고2','active',?)`, model.OrgIDLibrary)
	if err == nil {
		t.Fatal("옛 status=active 허용되면 안 됨")
	}

	if _, err := db.Exec(`INSERT INTO customers(customer_id,org_name,official_name,is_active,org_id)
		VALUES('C75O','타조직학교','타조직학교',1,'O03')`); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO project_sites(id,project_id,customer_id,site_name,status,org_id)
		VALUES('S75o','WP75','C75O','타조직','planned',?)`, model.OrgIDLibrary)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "org") {
		t.Fatalf("타 조직 거부 기대 err=%v", err)
	}

	_, err = db.Exec(`INSERT INTO project_sites(id,project_id,customer_id,site_name,status,org_id)
		VALUES('S75d','WP75','C75S','계룡고중복','planned',?)`, model.OrgIDLibrary)
	if err == nil {
		t.Fatal("같은 고객 두 사이트 UNIQUE 기대")
	}

	up, _ := projectSitesProgressMigrationSQL()
	for _, line := range strings.Split(up, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "--") {
			continue
		}
		if strings.Contains(strings.ToUpper(trim), "INSERT") {
			t.Fatal("075 up SQL 에 INSERT 가 있으면 안 됨")
		}
	}

	down, err := projectSitesProgressDownSQL()
	if err != nil {
		t.Fatal(err)
	}
	if err := execSQLIgnoreDupCol(db, down); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='project_site_events'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("down events n=%d err=%v", n, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM project_sites WHERE id='S75'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("down 후 사이트 유지 n=%d err=%v", n, err)
	}
	applyProjectSitesProgress(db)
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='project_site_events'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("재적용 n=%d err=%v", n, err)
	}
}

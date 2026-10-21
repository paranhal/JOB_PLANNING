package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestSyncSitesOnProjectWorkComplete(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "sync-sites.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO work_projects(project_id,name,status,org_id,project_kind,project_kind_set)
		VALUES('WP1','공사','active',?,'construction',1)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers(customer_id,org_name,official_name,is_active,org_id)
		VALUES('CA','예정교','예정교',1,?),('CB','검수교','검수교',1,?),('CC','설치교','설치교',1,?),('CD','외부교','외부교',1,?)`,
		model.OrgIDLibrary, model.OrgIDLibrary, model.OrgIDLibrary, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO project_sites(id,project_id,customer_id,site_name,status,org_id,is_active)
		VALUES('SA','WP1','CA','예정교','planned',?,1),
		      ('SB','WP1','CB','검수교','inspected',?,1),
		      ('SC','WP1','CC','설치교','installed',?,1)`,
		model.OrgIDLibrary, model.OrgIDLibrary, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks(task_id,work_type,work_subtype,project_id,title,status,org_id,complete_date,complete_note)
		VALUES('WT1','project_work','build','WP1','배선',?,?,?,?)`,
		model.WBTaskComplete, model.OrgIDLibrary, "2026-10-21", "완료메모"); err != nil {
		t.Fatal(err)
	}
	wb := NewWBRepo(db)
	if err := wb.ReplaceTaskCustomers("WT1", "WP1", []string{"CA", "CB", "CC", "CD"}); err != nil {
		t.Fatal(err)
	}
	t1, err := wb.GetTask("WT1")
	if err != nil || t1 == nil {
		t.Fatal(err)
	}
	if err := wb.SyncSitesOnProjectWorkComplete(t1, "u1"); err != nil {
		t.Fatal(err)
	}
	pe := NewProjectEquipRepo(db)
	sa, _ := pe.GetSite("SA")
	sb, _ := pe.GetSite("SB")
	sc, _ := pe.GetSite("SC")
	if model.NormalizeSiteStatus(sa.Status) != model.SiteInstalled {
		t.Fatalf("예정→설치 got=%s", sa.Status)
	}
	if model.NormalizeSiteStatus(sb.Status) != model.SiteInspected {
		t.Fatalf("검수 유지 got=%s", sb.Status)
	}
	if model.NormalizeSiteStatus(sc.Status) != model.SiteInstalled {
		t.Fatalf("설치 유지 got=%s", sc.Status)
	}
	var nWork, nInsp int
	_ = db.QueryRow(`SELECT COUNT(*) FROM project_site_events WHERE site_id='SA' AND note LIKE '일일업무 완료%'`).Scan(&nWork)
	_ = db.QueryRow(`SELECT COUNT(*) FROM project_site_events WHERE site_id IN ('SB','SC') AND note LIKE '일일업무 완료%'`).Scan(&nInsp)
	if nWork != 1 {
		t.Fatalf("완료 이력 SA=%d", nWork)
	}
	if nInsp != 0 {
		t.Fatalf("검수/설치 사이트에 완료 이력 %d", nInsp)
	}
	hist, err := wb.ListSiteWorksByProject("WP1")
	if err != nil {
		t.Fatal(err)
	}
	if len(hist["SA"]) != 1 || hist["SA"][0].Line() != "구축 · 배선 · 완료" {
		t.Fatalf("이력 %+v", hist["SA"])
	}
	if len(hist["CD"]) != 0 {
		t.Fatalf("미연결 고객 이력이 사업 사이트에 잡힘 %+v", hist)
	}
}

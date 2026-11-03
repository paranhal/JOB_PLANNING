package repository

import (
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func TestProjectCreateRejectsEmptyOrg(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "p87-empty.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	err = NewProjectRepo(db).Create(&model.WorkProject{Name: "빈조직", Status: model.WBProjectActive})
	if err == nil || !strings.Contains(err.Error(), "소속 조직") {
		t.Fatalf("빈 org 허용: %v", err)
	}
}

func TestSalesCreateRejectsEmptyOrg(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "s87-empty.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	err = NewSalesRepo(db).Create(&model.SalesProject{Name: "빈영업"})
	if err == nil || !strings.Contains(err.Error(), "소속 조직") {
		t.Fatalf("빈 org 허용: %v", err)
	}
}

func TestWorkProjectOrgsV100Backfill(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "p87-bf.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO work_projects(project_id,name,status,org_id,is_shared,share_to) VALUES
		('WP-SH','대상공개','active','O01',1,'O03'),
		('WP-ALL','전팀공개','active','O01',1,''),
		('WP-OWN','비공개','active','O01',0,'')`); err != nil {
		t.Fatal(err)
	}
	applyWorkProjectOrgsV100(db)
	var n, allN, ownN int
	_ = db.QueryRow(`SELECT COUNT(*) FROM work_project_orgs WHERE project_id='WP-SH' AND org_id='O03'`).Scan(&n)
	_ = db.QueryRow(`SELECT COUNT(*) FROM work_project_orgs WHERE project_id='WP-ALL'`).Scan(&allN)
	_ = db.QueryRow(`SELECT COUNT(*) FROM work_project_orgs WHERE project_id='WP-OWN'`).Scan(&ownN)
	if n != 1 {
		t.Fatalf("share_to 백필 %d", n)
	}
	if allN != 0 || ownN != 0 {
		t.Fatalf("전팀/비공개 백필 all=%d own=%d", allN, ownN)
	}
	applyWorkProjectOrgsV100(db)
	_ = db.QueryRow(`SELECT COUNT(*) FROM work_project_orgs WHERE project_id='WP-SH'`).Scan(&n)
	if n != 1 {
		t.Fatalf("재실행 %d", n)
	}
	down, err := workProjectOrgsDownSQL()
	if err != nil {
		t.Fatal(err)
	}
	if err := execWorkGroupSQL(db, down); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='work_project_orgs'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("down 후 테이블 %d %v", n, err)
	}
}

func TestReplaceRelatedOrgsRejectsOwnerAndInactive(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "p87-rel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O02','O02','미디어사업팀','business',1,20),('O03','O03','SI사업팀','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	repo := NewProjectRepo(db)
	p := &model.WorkProject{Name: "유관시험", Status: model.WBProjectActive, OrgID: "O01", ContractAmount: 1}
	if err := repo.Create(p); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceRelatedOrgs(p.ProjectID, "U1", []string{"O01"}, "O01"); err == nil {
		t.Fatal("주관을 유관으로 허용")
	}
	if err := repo.ReplaceRelatedOrgs(p.ProjectID, "U1", []string{"NOPE"}, "O01"); err == nil {
		t.Fatal("없는 조직 허용")
	}
	if _, err := db.Exec(`UPDATE orgs SET is_active=0 WHERE org_id='O03'`); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceRelatedOrgs(p.ProjectID, "U1", []string{"O03"}, "O01"); err == nil {
		t.Fatal("비활성 허용")
	}
	if _, err := db.Exec(`UPDATE orgs SET is_active=1 WHERE org_id='O03'`); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceRelatedOrgs(p.ProjectID, "U1", []string{"O03", "O02"}, "O01"); err != nil {
		t.Fatal(err)
	}
	got, err := repo.ListRelatedOrgs(p.ProjectID)
	if err != nil || len(got) != 2 {
		t.Fatalf("저장 %d %v", len(got), err)
	}
	if err := repo.ReplaceRelatedOrgs(p.ProjectID, "U1", []string{"O03"}, "O01"); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.ListRelatedOrgs(p.ProjectID)
	if len(got) != 1 || got[0].OrgID != "O03" {
		t.Fatalf("교체 %+v", got)
	}
}

func TestUpdateOrgMovesSalesAndKeepsPrevRelated(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "p87-mv.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	sr := NewSalesRepo(db)
	sp := &model.SalesProject{Name: "이관영업", OrgID: "O01"}
	if err := sr.Create(sp); err != nil {
		t.Fatal(err)
	}
	pr := NewProjectRepo(db)
	p := &model.WorkProject{Name: "이관사업", OrgID: "O01", SalesProjectID: sp.SalesID, Status: model.WBProjectActive, ContractAmount: 1}
	if err := pr.Create(p); err != nil {
		t.Fatal(err)
	}
	if err := pr.ReplaceRelatedOrgs(p.ProjectID, "U1", []string{"O03"}, "O01"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,due_date) VALUES
		('T-OPEN','admin','미완','waiting',?,'O01','2026-12-01'),
		('T-DONE','admin','완료','complete',?,'O01','2026-01-01')`, p.ProjectID, p.ProjectID); err != nil {
		t.Fatal(err)
	}
	if err := pr.UpdateOrg(p.ProjectID, ProjectOrgMoveOpts{}); err == nil {
		t.Fatal("사유 없이 이관")
	}
	if err := pr.UpdateOrg(p.ProjectID, ProjectOrgMoveOpts{ToOrg: "O01", Reason: "같음", ByUser: "U1"}); err == nil {
		t.Fatal("같은 팀 이관")
	}
	if err := pr.UpdateOrg(p.ProjectID, ProjectOrgMoveOpts{
		ToOrg: "O03", Reason: "SI로 이관", ByUser: "U1",
		MoveOpenTasks: true, KeepPrevAsRelated: true, DropForeignAssign: true,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := pr.Get(p.ProjectID)
	if err != nil || got.OrgID != "O03" {
		t.Fatalf("사업 org %q %v", got.OrgID, err)
	}
	var sorg, openOrg, doneOrg string
	_ = db.QueryRow(`SELECT org_id FROM sales_projects WHERE sales_id=?`, sp.SalesID).Scan(&sorg)
	_ = db.QueryRow(`SELECT org_id FROM work_tasks WHERE task_id='T-OPEN'`).Scan(&openOrg)
	_ = db.QueryRow(`SELECT org_id FROM work_tasks WHERE task_id='T-DONE'`).Scan(&doneOrg)
	if sorg != "O03" {
		t.Fatalf("영업 org %s", sorg)
	}
	if openOrg != "O03" || doneOrg != "O01" {
		t.Fatalf("업무 open=%s done=%s", openOrg, doneOrg)
	}
	if model.ProjectAccess(*got, "O01") != model.ProjectAccessRelated {
		t.Fatal("이전 주관이 유관이 아니다")
	}
	if model.ProjectAccess(*got, "O03") != model.ProjectAccessOwner {
		t.Fatal("새 주관")
	}
	for _, id := range got.RelatedOrgIDs {
		if id == "O03" {
			t.Fatal("새 주관이 유관에 남음")
		}
	}
}

func TestUpdateOrgRollbackOnDupContractNo(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "p87-rb.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	pr := NewProjectRepo(db)
	a := &model.WorkProject{Name: "원본", OrgID: "O01", ContractNo: "CN-87", Status: model.WBProjectActive, ContractAmount: 1}
	b := &model.WorkProject{Name: "대상팀기존", OrgID: "O03", ContractNo: "CN-87", Status: model.WBProjectActive, ContractAmount: 1}
	if err := pr.Create(a); err != nil {
		t.Fatal(err)
	}
	if err := pr.Create(b); err != nil {
		t.Fatal(err)
	}
	err = pr.UpdateOrg(a.ProjectID, ProjectOrgMoveOpts{ToOrg: "O03", Reason: "중복번호", ByUser: "U1"})
	if err == nil || !strings.Contains(err.Error(), "사업번호") {
		t.Fatalf("중복 거절 아님: %v", err)
	}
	got, _ := pr.Get(a.ProjectID)
	if got.OrgID != "O01" {
		t.Fatalf("롤백 안 됨 %s", got.OrgID)
	}
}

func TestSixTeamProjectAccessMatrix(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "p87-6.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O02','O02','미디어사업팀','business',1,20),('O03','O03','SI사업팀','business',1,30),
		('O04','O04','4팀','business',1,40),('O05','O05','5팀','business',1,50),('O06','O06','6팀','business',1,60)`); err != nil {
		t.Fatal(err)
	}
	pr := NewProjectRepo(db)
	ids := map[string]string{}
	for _, org := range []string{"O01", "O02", "O03", "O04", "O05", "O06"} {
		p := &model.WorkProject{Name: "팀" + org, OrgID: org, Status: model.WBProjectActive, ContractAmount: 1}
		if err := pr.Create(p); err != nil {
			t.Fatal(org, err)
		}
		ids[org] = p.ProjectID
	}
	owner := ids["O01"]
	if err := pr.ReplaceRelatedOrgs(owner, "U1", []string{"O02"}, "O01"); err != nil {
		t.Fatal(err)
	}
	p, _ := pr.Get(owner)
	for _, org := range []string{"O01", "O02", "O03", "O04", "O05", "O06"} {
		a := model.ProjectAccess(*p, org)
		switch org {
		case "O01":
			if a != model.ProjectAccessOwner {
				t.Fatalf("%s %s", org, a)
			}
		case "O02":
			if a != model.ProjectAccessRelated {
				t.Fatalf("%s %s", org, a)
			}
		default:
			if a != model.ProjectAccessViewer {
				t.Fatalf("%s %s", org, a)
			}
		}
		_, werr := pr.GetVisible(owner, org, true)
		if org == "O01" && werr != nil {
			t.Fatal("주관 쓰기")
		}
		if org != "O01" && werr == nil {
			t.Fatalf("%s 쓰기 허용", org)
		}
	}
}

func TestProjectAccessStaticRequired(t *testing.T) {
	if ProjectWritableInOrg("O03", "O03") != true {
		t.Fatal("owner")
	}
	if ProjectWritableInOrg("O03", "O01") {
		t.Fatal("타팀 쓰기")
	}
}

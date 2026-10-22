package repository

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func seedOrgAmtProject(t *testing.T, repo *ProjectRepo, db *sql.DB, name, org string) string {
	t.Helper()
	p := &model.WorkProject{
		Name: name, PlanYear: 2026, Status: model.WBProjectActive, ContractAmount: 2_000_000, OrgID: org,
	}
	if err := repo.Create(p); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE work_projects SET org_id=? WHERE project_id=?`, org, p.ProjectID); err != nil {
		t.Fatal(err)
	}
	return p.ProjectID
}

func namesOf(items []model.WorkProject) map[string]bool {
	out := map[string]bool{}
	for _, p := range items {
		out[p.Name] = true
	}
	return out
}

func TestListContractsOrgFilterAndShare(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "orgf.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O02','O02','미디어','business',1,20),('O03','O03','SI','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	repo := NewProjectRepo(db)
	id01a := seedOrgAmtProject(t, repo, db, "O01-A", "O01")
	seedOrgAmtProject(t, repo, db, "O01-B", "O01")
	seedOrgAmtProject(t, repo, db, "O02-A", "O02")
	seedOrgAmtProject(t, repo, db, "O02-B", "O02")
	seedOrgAmtProject(t, repo, db, "O03-A", "O03")
	seedOrgAmtProject(t, repo, db, "O03-B", "O03")

	o03, err := repo.ListContracts("O03", "", 0, "", false)
	if err != nil {
		t.Fatal(err)
	}
	n := namesOf(o03)
	if !n["O03-A"] || !n["O03-B"] {
		t.Fatal("O03 자기 사업")
	}
	if !n["O01-A"] || !n["O02-A"] {
		t.Fatal("95-2E 전 직원 조회인데 O03에 타 조직 사업이 없다")
	}
	scoped, err := repo.ListContractsInOrg("O03", "", 0, "", false)
	if err != nil {
		t.Fatal(err)
	}
	sn := namesOf(scoped)
	if sn["O01-A"] || !sn["O03-A"] {
		t.Fatal("집계용 ListContractsInOrg 가 조직 의미를 잃음")
	}
	all, err := repo.ListContracts(OrgAll, "", 0, "", false)
	if err != nil {
		t.Fatal(err)
	}
	an := namesOf(all)
	if !an["O01-A"] || !an["O02-B"] || !an["O03-A"] {
		t.Fatal("OrgAll")
	}

	if _, err := db.Exec(`UPDATE work_projects SET is_shared=1, share_to='O02' WHERE project_id=?`, id01a); err != nil {
		t.Fatal(err)
	}
	o02, err := repo.ListContracts("O02", "", 0, "", false)
	if err != nil {
		t.Fatal(err)
	}
	n2 := namesOf(o02)
	if !n2["O01-A"] || !n2["O01-B"] {
		t.Fatal("95-2E 전 직원 조회 — 비공개도 목록에 보인다. 쓰기는 GetVisible(write)")
	}
	o03b, err := repo.ListContracts("O03", "", 0, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if !namesOf(o03b)["O01-A"] {
		t.Fatal("95-2E 비대상 팀도 조회")
	}

	if _, err := repo.GetVisible(id01a, "O02", false); err != nil {
		t.Fatal("공개 읽기")
	}
	if _, err := repo.GetVisible(id01a, "O02", true); !errors.Is(err, ErrProjectForbidden) {
		t.Fatalf("공개 쓰기 %v", err)
	}
	if _, err := repo.GetVisible(id01a, "O03", false); err != nil {
		t.Fatal("95-2E 타 조직 읽기 GetVisible")
	}
	if _, err := repo.GetVisible(id01a, "O03", true); !errors.Is(err, ErrProjectForbidden) {
		t.Fatalf("타 조직 쓰기 %v", err)
	}
}

func TestListWorkPickProjectsIgnoresOrg(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "pick.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewProjectRepo(db)
	seedOrgAmtProject(t, repo, db, "픽O01", "O01")
	seedOrgAmtProject(t, repo, db, "픽O03", "O03")
	pick, err := NewWBRepo(db).ListWorkPickProjects("O03")
	if err != nil {
		t.Fatal(err)
	}
	n := namesOf(pick)
	if !n["픽O01"] || !n["픽O03"] {
		t.Fatal("선택 목록 조직 무관이 깨짐")
	}
	listed, err := repo.ListContracts("O03", "", 0, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if !namesOf(listed)["픽O01"] {
		t.Fatal("95-2E 등록 관리 전 직원 조회")
	}
	pickOnly, err := NewWBRepo(db).ListWorkPickProjects("O01")
	if err != nil {
		t.Fatal(err)
	}
	if !namesOf(pickOnly)["픽O03"] {
		t.Fatal("선택 목록 조직 무관 회귀")
	}
}

func TestListContracts47OrgOracle(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "c47.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O02','O02','미디어사업팀','business',1,20),('O03','O03','SI사업팀','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	raw := sampleContractXLSX(t)
	seedUniqueContractCustomers(t, db, raw, model.OrgIDLibrary)
	repo := NewProjectRepo(db)
	libBefore, err := repo.ListContractsInOrg("O01", "", 0, "", false)
	if err != nil {
		t.Fatal(err)
	}
	today := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	scope := CatImportScope{Allowed: map[string]bool{"O01": true, "O02": true, "O03": true}}
	app, err := repo.ApplyContractImport("U1", "최종47.xlsx", raw, scope, today, model.ContractImportApplyOpt{})
	if err != nil {
		t.Fatal(err)
	}
	if app.NewN != 47 {
		t.Fatalf("apply %d", app.NewN)
	}
	si, err := repo.ListContractsInOrg("O03", "", 0, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(si) != 14 {
		t.Fatalf("SI 소속 스탬프 %d", len(si))
	}
	media, err := repo.ListContractsInOrg("O02", "", 0, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(media) != 33 {
		t.Fatalf("미디어 소속 스탬프 %d", len(media))
	}
	lib, err := repo.ListContractsInOrg("O01", "", 0, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(lib) != len(libBefore) {
		t.Fatalf("도서관 추가 %d -> %d", len(libBefore), len(lib))
	}
	staff, err := repo.ListContracts("O03", "", 0, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(staff) < 47 {
		t.Fatalf("전 직원 조회 %d", len(staff))
	}
	pick, err := NewWBRepo(db).ListWorkPickProjects("O01")
	if err != nil {
		t.Fatal(err)
	}
	if len(pick) < 47 {
		t.Fatalf("pick %d", len(pick))
	}
}

package repository

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

type sitesFx struct {
	repo       *ProjectRepo
	pid, phase string
	pkgA, pkgQ string
	ids        map[string]string // 이름 → project_site_id
}

func sitesFixture(t *testing.T) *sitesFx {
	t.Helper()
	db, err := InitDB(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewProjectRepo(db)
	for _, p := range []string{"WPS", "WPT"} {
		if _, err := db.Exec(`INSERT INTO work_projects (project_id, name, status, org_id) VALUES (?,?, 'active','O01')`, p, p); err != nil {
			t.Fatal(err)
		}
	}
	fx := &sitesFx{repo: repo, pid: "WPS", ids: map[string]string{}}
	add := func(project, key, name, region, status string) {
		t.Helper()
		cid := "C-" + key
		if _, err := db.Exec(`INSERT INTO customers(customer_id, org_name, official_name, is_active, org_id) VALUES (?,?,?,1,'O01')`, cid, name, name); err != nil {
			t.Fatal(err)
		}
		id := "S-" + key
		if _, err := db.Exec(`INSERT INTO project_sites(id, project_id, site_name, region, status, org_id, customer_id) VALUES (?,?,?,?,?, 'O01', ?)`,
			id, project, name, region, status, cid); err != nil {
			t.Fatal(err)
		}
		fx.ids[name] = id
	}
	add("WPS", "hsh", "홍성고등학교", "충남 홍성군", "planned")
	add("WPS", "hsc", "홍성초등학교", "충남 홍성군", "planned")
	add("WPS", "cah", "천안고등학교", "충남 천안시", "planned")
	add("WPS", "yem", "예산중학교", "충남 예산군", "planned")
	add("WPS", "exc", "제외학교", "충남 홍성군", "excluded")
	add("WPT", "oth", "타사업학교", "충남 홍성군", "planned")
	ph := &model.ProjectPhase{ProjectID: "WPS", OrgID: "O01", Name: "전처리 업무", Status: model.PhaseActive}
	if err := repo.CreatePhase(ph); err != nil {
		t.Fatal(err)
	}
	fx.phase = ph.PhaseID
	if fx.pkgA, err = repo.CreatePackage(ph.PhaseID, "GBIC 수량 산정", model.TrackModeSite); err != nil {
		t.Fatal(err)
	}
	if fx.pkgQ, err = repo.CreatePackage(ph.PhaseID, "수량만", model.TrackModePhase); err != nil {
		t.Fatal(err)
	}
	return fx
}

func (fx *sitesFx) idsOf(names ...string) []string {
	var out []string
	for _, n := range names {
		out = append(out, fx.ids[n])
	}
	return out
}

func TestPhaseSiteCandidatesSearchRegionAndPaging(t *testing.T) {
	fx := sitesFixture(t)
	p, err := fx.repo.PhaseSiteCandidates(fx.pid, fx.phase, "", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	// 제외 상태·타 사업 사이트는 후보가 아니다
	if p.Total != 4 || p.Addable != 4 || p.Pool != 4 || p.Registered != 0 || len(p.Regions) != 3 || p.TargetName != "전처리 업무" {
		t.Fatalf("전체 %+v", p)
	}
	if p, _ = fx.repo.PhaseSiteCandidates(fx.pid, fx.phase, "홍성", "", 1); p.Total != 2 {
		t.Fatalf("검색 홍성 %d", p.Total)
	}
	if p, _ = fx.repo.PhaseSiteCandidates(fx.pid, fx.phase, "", "충남 천안시", 1); p.Total != 1 || p.Items[0].Name != "천안고등학교" {
		t.Fatalf("지역 필터 %+v", p)
	}
	if p, _ = fx.repo.PhaseSiteCandidates(fx.pid, fx.phase, "100%_", "", 1); p.Total != 0 {
		t.Fatalf("LIKE 특수문자가 와일드카드로 먹었다: %d", p.Total)
	}
	if _, err := fx.repo.PhaseSiteCandidates("WPT", fx.phase, "", "", 1); err == nil {
		t.Fatal("다른 사업의 단계는 거절")
	}
	// 100곳씩 쪽 나누기
	for i := 0; i < 230; i++ {
		cid := fmt.Sprintf("CP%03d", i)
		_, _ = fx.repo.db.Exec(`INSERT INTO customers(customer_id, org_name, official_name, is_active, org_id) VALUES (?,?,?,1,'O01')`, cid, cid, cid)
		if _, err := fx.repo.db.Exec(`INSERT INTO project_sites(id, project_id, site_name, region, status, org_id, customer_id) VALUES (?,?,?,?, 'planned','O01',?)`,
			"SP"+cid, "WPS", fmt.Sprintf("페이지학교%03d", i), "경북 어딘가", cid); err != nil {
			t.Fatal(err)
		}
	}
	p, _ = fx.repo.PhaseSiteCandidates(fx.pid, fx.phase, "페이지학교", "", 2)
	if p.Total != 230 || p.Pages != 3 || p.Page != 2 || len(p.Items) != 100 {
		t.Fatalf("쪽 나누기 %+v", p)
	}
	if p, _ = fx.repo.PhaseSiteCandidates(fx.pid, fx.phase, "페이지학교", "", 99); p.Page != 3 || len(p.Items) != 30 {
		t.Fatalf("범위 밖 쪽은 마지막 쪽: %d/%d", p.Page, len(p.Items))
	}
}

func TestAddRemovePhaseSites(t *testing.T) {
	fx := sitesFixture(t)
	if _, err := fx.repo.AddPhaseSites(fx.pid, fx.phase, nil, false, "", "", "u1"); err == nil {
		t.Fatal("비어 있으면 거절")
	}
	n, err := fx.repo.AddPhaseSites(fx.pid, fx.phase, fx.idsOf("홍성고등학교", "홍성고등학교", "천안고등학교"), false, "", "", "u1")
	if err != nil || n != 2 {
		t.Fatalf("선택 등록 n=%d err=%v", n, err)
	}
	if n, _ = fx.repo.AddPhaseSites(fx.pid, fx.phase, fx.idsOf("홍성고등학교"), false, "", "", "u1"); n != 0 {
		t.Fatalf("이미 등록된 것은 다시 세지 않는다: %d", n)
	}
	// 다른 사업·제외 상태 사이트는 등록할 수 없다
	if _, err := fx.repo.AddPhaseSites(fx.pid, fx.phase, fx.idsOf("타사업학교"), false, "", "", "u1"); err == nil {
		t.Fatal("타 사업 사이트 거절")
	}
	if _, err := fx.repo.AddPhaseSites(fx.pid, fx.phase, fx.idsOf("제외학교"), false, "", "", "u1"); err == nil {
		t.Fatal("제외 상태 사이트 거절")
	}
	// 조회 결과 전체 등록(필터 적용): 홍성 2곳 중 아직 안 된 홍성초만
	if n, err = fx.repo.AddPhaseSites(fx.pid, fx.phase, nil, true, "홍성", "", "u1"); err != nil || n != 1 {
		t.Fatalf("필터 전체 등록 n=%d err=%v", n, err)
	}
	// 사업 사이트 전체 가져오기(필터 없음): 남은 예산중 1곳
	if n, err = fx.repo.AddPhaseSites(fx.pid, fx.phase, nil, true, "", "", "u1"); err != nil || n != 1 {
		t.Fatalf("전체 가져오기 n=%d err=%v", n, err)
	}
	p, _ := fx.repo.PhaseSiteCandidates(fx.pid, fx.phase, "", "", 1)
	if p.Registered != 4 || p.Addable != 0 {
		t.Fatalf("등록 현황 %+v", p)
	}
	// 제외: 사유 필수, 하부 업무에 쓰이는 사이트는 거절
	if _, err := fx.repo.RemovePhaseSites(fx.pid, fx.phase, fx.idsOf("천안고등학교"), "", "u1"); err == nil || !strings.Contains(err.Error(), "사유") {
		t.Fatalf("사유 없는 제외: %v", err)
	}
	if _, err := fx.repo.AddPackageSites(fx.pid, fx.pkgA, fx.idsOf("홍성고등학교"), false, "", "", "u1"); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.repo.RemovePhaseSites(fx.pid, fx.phase, fx.idsOf("홍성고등학교"), "범위 밖", "u1"); err == nil || !strings.Contains(err.Error(), "하부 업무에 등록돼 있어") {
		t.Fatalf("하부 업무에 쓰이는 사이트: %v", err)
	}
	if n, err = fx.repo.RemovePhaseSites(fx.pid, fx.phase, fx.idsOf("천안고등학교"), "범위 밖", "u1"); err != nil || n != 1 {
		t.Fatalf("제외 n=%d err=%v", n, err)
	}
	if p, _ = fx.repo.PhaseSiteCandidates(fx.pid, fx.phase, "천안", "", 1); p.Items[0].Added {
		t.Fatal("제외했는데 등록됨으로 보인다")
	}
	// 제외했던 사이트를 다시 등록하면 되살아난다
	if n, err = fx.repo.AddPhaseSites(fx.pid, fx.phase, fx.idsOf("천안고등학교"), false, "", "", "u1"); err != nil || n != 1 {
		t.Fatalf("다시 등록 n=%d err=%v", n, err)
	}
}

func TestAddRemovePackageSites(t *testing.T) {
	fx := sitesFixture(t)
	if _, err := fx.repo.AddPhaseSites(fx.pid, fx.phase, fx.idsOf("홍성고등학교", "홍성초등학교", "천안고등학교"), false, "", "", "u1"); err != nil {
		t.Fatal(err)
	}
	// 후보는 1단계 업무에 등록된 사이트뿐이다(예산중은 1단계에 없다)
	p, err := fx.repo.PackageSiteCandidates(fx.pid, fx.pkgA, "", "", 1)
	if err != nil || p.Total != 3 || p.Pool != 3 || p.Registered != 0 {
		t.Fatalf("후보 %+v err=%v", p, err)
	}
	if _, err := fx.repo.AddPackageSites(fx.pid, fx.pkgA, fx.idsOf("예산중학교"), false, "", "", "u1"); err == nil || !strings.Contains(err.Error(), "1단계 업무에 등록된") {
		t.Fatalf("1단계 밖 사이트: %v", err)
	}
	if _, err := fx.repo.PackageSiteCandidates(fx.pid, fx.pkgQ, "", "", 1); err == nil {
		t.Fatal("수량만 하부 업무는 사이트 후보가 없다")
	}
	if _, err := fx.repo.AddPackageSites(fx.pid, fx.pkgQ, fx.idsOf("홍성고등학교"), false, "", "", "u1"); err == nil {
		t.Fatal("수량만 하부 업무에는 등록 거절")
	}
	n, err := fx.repo.AddPackageSites(fx.pid, fx.pkgA, nil, true, "홍성", "", "u1")
	if err != nil || n != 2 {
		t.Fatalf("필터 전체 등록 n=%d err=%v", n, err)
	}
	var plan string
	_ = fx.repo.db.QueryRow(`SELECT plan_id FROM package_sites WHERE package_id=? AND project_site_id=?`, fx.pkgA, fx.ids["홍성고등학교"]).Scan(&plan)
	if plan != "" {
		t.Fatalf("계획 없이 등록되어야 한다: %q", plan)
	}
	// 구조 트리: 계획 미배정으로 센다
	tree, _ := fx.repo.LoadStructure(fx.pid, "2026-11-12", model.WeekendBiz)
	var node PkgNode
	for _, pk := range tree.Phases[0].Packages {
		if pk.ID == fx.pkgA {
			node = pk
		}
	}
	if node.Unassigned != 2 || node.TotalN != 2 {
		t.Fatalf("계획 미배정 %d / 전체 %d", node.Unassigned, node.TotalN)
	}
	// 이어서 작업 계획에 담을 수 있다(계획 미배정 사이트 → 계획)
	if _, err := fx.repo.CreatePlan(fx.pid, PlanDraft{PackageID: fx.pkgA, Name: "홍성 지역", SiteIDs: fx.idsOf("홍성고등학교", "홍성초등학교")}); err != nil {
		t.Fatalf("계획에 담기: %v", err)
	}
	// 제외: 사유 필수, 계획에 속한 사이트는 거절, 계획 없는 사이트는 제외
	if n, err = fx.repo.AddPackageSites(fx.pid, fx.pkgA, fx.idsOf("천안고등학교"), false, "", "", "u1"); err != nil || n != 1 {
		t.Fatalf("천안 등록 n=%d err=%v", n, err)
	}
	if _, err := fx.repo.RemovePackageSites(fx.pid, fx.pkgA, fx.idsOf("천안고등학교"), " ", "u1"); err == nil {
		t.Fatal("사유 없는 제외")
	}
	if _, err := fx.repo.RemovePackageSites(fx.pid, fx.pkgA, fx.idsOf("홍성고등학교"), "빼기", "u1"); err == nil || !strings.Contains(err.Error(), "작업 계획에 속해") {
		t.Fatalf("계획 사이트: %v", err)
	}
	if n, err = fx.repo.RemovePackageSites(fx.pid, fx.pkgA, fx.idsOf("천안고등학교"), "범위 밖", "u1"); err != nil || n != 1 {
		t.Fatalf("제외 n=%d err=%v", n, err)
	}
	if p, _ = fx.repo.PackageSiteCandidates(fx.pid, fx.pkgA, "천안", "", 1); p.Items[0].Added {
		t.Fatal("제외했는데 등록됨")
	}
	if n, err = fx.repo.AddPackageSites(fx.pid, fx.pkgA, fx.idsOf("천안고등학교"), false, "", "", "u1"); err != nil || n != 1 {
		t.Fatalf("제외했던 사이트 다시 등록 n=%d err=%v", n, err)
	}
	// 완료된 사이트는 뺄 수 없다
	if _, err := fx.repo.db.Exec(`UPDATE package_sites SET status='done' WHERE package_id=? AND project_site_id=?`, fx.pkgA, fx.ids["천안고등학교"]); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.repo.RemovePackageSites(fx.pid, fx.pkgA, fx.idsOf("천안고등학교"), "빼기", "u1"); err == nil || !strings.Contains(err.Error(), "완료된 사이트") {
		t.Fatalf("완료 사이트: %v", err)
	}
	// 다른 사업의 하부 업무는 거절
	if _, err := fx.repo.AddPackageSites("WPT", fx.pkgA, fx.idsOf("홍성고등학교"), false, "", "", "u1"); err == nil {
		t.Fatal("다른 사업")
	}
}

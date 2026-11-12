package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

// 비중 정리(95-176): 사업 진행률은 하부 업무 비중과 무관하게 사이트 합계(완료 ÷ 대상)다.
// DB 의 weight 값이 하부 업무마다 달라도(옛 데이터) 결과가 같아야 한다.
func TestStructureProgressIgnoresPackageWeight(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "w.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewProjectRepo(db)
	if _, err := db.Exec(`INSERT INTO work_projects (project_id, name, status, org_id) VALUES ('WPW','비중','active','O01')`); err != nil {
		t.Fatal(err)
	}
	ph := &model.ProjectPhase{ProjectID: "WPW", OrgID: "O01", Name: "구축", Status: model.PhaseActive}
	if err := repo.CreatePhase(ph); err != nil {
		t.Fatal(err)
	}
	a, err := repo.CreatePackage(ph.PhaseID, "가", model.TrackModeSite)
	if err != nil {
		t.Fatal(err)
	}
	b, err := repo.CreatePackage(ph.PhaseID, "나", model.TrackModeSite)
	if err != nil {
		t.Fatal(err)
	}
	q, err := repo.CreatePackage(ph.PhaseID, "수량", model.TrackModePhase)
	if err != nil {
		t.Fatal(err)
	}
	// 새 하부 업무의 비중은 항상 1
	var w float64
	if err := db.QueryRow(`SELECT weight FROM phase_packages WHERE package_id=?`, a).Scan(&w); err != nil || w != 1 {
		t.Fatalf("새 하부 업무 비중 %v %v", w, err)
	}
	// 옛 데이터처럼 비중을 크게 달리해 둔다
	if _, err := db.Exec(`UPDATE phase_packages SET weight=5 WHERE package_id=?`, a); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE phase_packages SET weight=9 WHERE package_id=?`, q); err != nil {
		t.Fatal(err)
	}
	site := func(pkg, id, status string) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO customers(customer_id, org_name, official_name, is_active, org_id) VALUES (?,?,?,1,'O01')`, "C"+id, id, id); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO project_sites(id, project_id, site_name, status, org_id, customer_id) VALUES (?,?,?,'planned','O01',?)`, "S"+id, "WPW", id, "C"+id); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO package_sites(package_site_id, package_id, project_site_id, status, updated_at) VALUES (?,?,?,?, 't')`, "PS"+id, pkg, "S"+id, status); err != nil {
			t.Fatal(err)
		}
	}
	site(a, "1", model.SiteDone)
	site(a, "2", model.SitePlanned)
	site(b, "3", model.SiteDone)
	site(b, "4", model.SiteDone)

	got, err := repo.StructureProgress([]string{"WPW"})
	if err != nil {
		t.Fatal(err)
	}
	p := got["WPW"]
	// 사이트 합계 3/4 = 75%. (가중평균이었다면 (5×50% + 1×100%) ÷ 6 = 58%)
	if p.SiteDone != 3 || p.SiteTotal != 4 || p.Pct != 75 || p.Label != model.SiteCheckRateLabel(3, 4) {
		t.Fatalf("사업 진행률 %+v", p)
	}
	// 수량만 하부 업무는 사이트 진행률에서 빠진다(반영 2 / 전체 3)
	if p.Reflected != 2 || p.Total != 3 {
		t.Fatalf("반영 %d / 전체 %d", p.Reflected, p.Total)
	}
}

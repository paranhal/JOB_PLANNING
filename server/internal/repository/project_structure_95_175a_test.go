package repository

import (
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"

	"customer-support/internal/model"
)

func Test95_175A_PlanCRUDAndSearch(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewProjectRepo(db)
	if _, err := db.Exec(`INSERT INTO work_projects (project_id, name, status, org_id) VALUES ('WP-A','구조','active','O01')`); err != nil {
		t.Fatal(err)
	}
	ph := &model.ProjectPhase{ProjectID: "WP-A", OrgID: "O01", Name: "구축", Status: model.PhasePlanned}
	if err := repo.CreatePhase(ph); err != nil {
		t.Fatal(err)
	}
	pkg, err := repo.CreatePackage(ph.PhaseID, "현장", model.TrackModeSite, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO project_sites(id,project_id,site_name,region,status,org_id) VALUES
		('S1','WP-A','홍천초등학교','강원 홍천군','planned','O01'),
		('S2','WP-A','서울고등학교','서울','planned','O01')`); err != nil {
		t.Fatal(err)
	}
	var before int
	_ = db.QueryRow(`SELECT COUNT(*) FROM phase_sites`).Scan(&before)
	if _, err := repo.CreatePlan("WP-A", PlanDraft{PackageID: pkg, Name: "홍천", SiteIDs: []string{"S1", "없는곳"}}); err == nil {
		t.Fatal("없는 사이트는 롤백")
	}
	var after int
	_ = db.QueryRow(`SELECT COUNT(*) FROM phase_sites`).Scan(&after)
	if after != before {
		t.Fatalf("롤백 실패 %d→%d", before, after)
	}
	if _, err := repo.CreatePlan("WP-A", PlanDraft{
		PackageID: pkg, Name: "홍천 지역", PlanStart: "2026-11-01", PlanEnd: "2026-11-30",
		SiteIDs: []string{"S1"}, Basis: model.BasisSite,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreatePlan("WP-A", PlanDraft{PackageID: pkg, Name: "홍천 지역"}); err == nil {
		t.Fatal("이름 중복")
	}
	if _, err := repo.CreatePlan("WP-A", PlanDraft{PackageID: pkg, Name: "날짜", PlanStart: "2026-12-01", PlanEnd: "2026-11-01"}); err == nil {
		t.Fatal("날짜 순서")
	}
	if _, err := repo.CreatePlan("WP-A", PlanDraft{PackageID: pkg, Name: "다른", SiteIDs: []string{"S1"}}); err == nil {
		t.Fatal("한 사이트는 한 계획")
	}
	id2, err := repo.CreatePlan("WP-A", PlanDraft{PackageID: pkg, Name: "서울", SiteIDs: []string{"S2"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.MovePlanSites("WP-A", id2, []string{"S1"}, "u"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE package_sites SET status='done' WHERE project_site_id='S1'`); err != nil {
		t.Fatal(err)
	}
	id3, err := repo.CreatePlan("WP-A", PlanDraft{PackageID: pkg, Name: "다음"})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.MovePlanSites("WP-A", id3, []string{"S1"}, "u"); err == nil {
		t.Fatal("완료 사이트 이동")
	}
	if _, err := db.Exec(`UPDATE package_sites SET status='planned' WHERE project_site_id='S1'`); err != nil {
		t.Fatal(err)
	}
	if err := repo.MovePlanSites("WP-A", id3, []string{"S1"}, "u"); err != nil {
		t.Fatal(err)
	}
	if err := repo.ExcludePlanSite("WP-A", id3, "S1", "", false); err == nil {
		t.Fatal("제외 사유")
	}
	if err := repo.ExcludePlanSite("WP-A", id3, "S1", "범위 아님", false); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO project_sites(id,project_id,site_name,status,org_id) VALUES ('S4','WP-A','수량학교','planned','O01')`); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreatePlan("WP-A", PlanDraft{PackageID: pkg, Name: "수량", Basis: model.BasisQty, SiteIDs: []string{"S4"}}); err == nil {
		t.Fatal("수량 미입력")
	}
	if _, err := db.Exec(`INSERT INTO project_sites(id,project_id,site_name,region,status,org_id) VALUES ('S3','WP-A','미배정학교','강원','planned','O01')`); err != nil {
		t.Fatal(err)
	}
	loose, err := repo.CreatePlan("WP-A", PlanDraft{PackageID: pkg, Name: "미배정확인", SiteIDs: []string{"S3"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.DeletePlan("WP-A", loose); err != nil {
		t.Fatal(err)
	}
	var left, unassigned int
	_ = db.QueryRow(`SELECT COUNT(*) FROM package_plans WHERE plan_id=?`, loose).Scan(&left)
	_ = db.QueryRow(`SELECT COUNT(*) FROM package_sites WHERE project_site_id='S3' AND plan_id=''`).Scan(&unassigned)
	if left != 0 || unassigned != 1 {
		t.Fatalf("삭제 후 계획 %d 미배정 %d", left, unassigned)
	}
	if _, err := db.Exec(`UPDATE package_sites SET status='in_progress', plan_id=? WHERE project_site_id='S2'`, id2); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeletePlan("WP-A", id2); err != nil {
		t.Fatal(err)
	}
	var active int
	_ = db.QueryRow(`SELECT is_active FROM package_plans WHERE plan_id=?`, id2).Scan(&active)
	if active != 0 {
		t.Fatal("실적 있으면 비활성")
	}

	for i := 0; i < 800; i++ {
		name := "홍천학교"
		region := "강원 홍천군"
		if i%2 == 0 {
			name = "계룡학교"
			region = "충남 계룡시"
		}
		if _, err := db.Exec(`INSERT INTO project_sites(id,project_id,site_name,region,status,org_id) VALUES (?,?,?,?, 'planned','O01')`,
			"B"+strconv.Itoa(i), "WP-A", name+strconv.Itoa(i), region); err != nil {
			t.Fatal(err)
		}
	}
	hits, total, err := repo.SearchPlanSites("WP-A", pkg, "ㅎㅊ", "홍천", 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if total < 400 || len(hits) != 100 {
		t.Fatalf("검색 total %d page %d", total, len(hits))
	}
	hits2, _, err := repo.SearchPlanSites("WP-A", pkg, "ㅎㅊ", "홍천", 100, 100)
	if err != nil || len(hits2) == 0 || hits2[0].ID == hits[0].ID {
		t.Fatal("다음 페이지")
	}
	tree, err := repo.LoadStructure("WP-A", "2026-11-12", model.WeekendBiz)
	if err != nil || !tree.Has {
		t.Fatal(err)
	}
	prog, err := repo.StructureProgress([]string{"WP-A", "없는"})
	if err != nil {
		t.Fatal(err)
	}
	if prog["WP-A"].Label == "" {
		t.Fatal("진행률")
	}
	empty, err := repo.StructureProgress(nil)
	if err != nil || len(empty) != 0 {
		t.Fatal("빈 목록")
	}
}

func Test95_175A_MergePlansOnly(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewProjectRepo(db)
	for _, id := range []string{"KEEP", "DROP", "PLAIN"} {
		if _, err := db.Exec(`INSERT INTO work_projects (project_id, name, status, org_id) VALUES (?,?, 'active','O01')`, id, id); err != nil {
			t.Fatal(err)
		}
	}
	keepPh := &model.ProjectPhase{ProjectID: "KEEP", OrgID: "O01", Name: "구축", Status: model.PhasePlanned}
	dropPh := &model.ProjectPhase{ProjectID: "DROP", OrgID: "O01", Name: "구축", Status: model.PhaseActive}
	if err := repo.CreatePhase(keepPh); err != nil || repo.CreatePhase(dropPh) != nil {
		t.Fatal(err)
	}
	kp, _ := repo.CreatePackage(keepPh.PhaseID, "현장", "site", 1)
	dp, _ := repo.CreatePackage(dropPh.PhaseID, "현장", "site", 1)
	if _, err := db.Exec(`INSERT INTO project_sites(id,project_id,site_name,status,org_id) VALUES
		('K1','KEEP','같은학교','planned','O01'),
		('D1','DROP','같은학교','planned','O01'),
		('D2','DROP','다른학교','planned','O01'),
		('P1','PLAIN','그대로','planned','O01')`); err != nil {
		t.Fatal(err)
	}
	kplan, err := repo.CreatePlan("KEEP", PlanDraft{PackageID: kp, Name: "1차", SiteIDs: []string{"K1"}})
	if err != nil {
		t.Fatal(err)
	}
	dplan, err := repo.CreatePlan("DROP", PlanDraft{PackageID: dp, Name: "1차", SiteIDs: []string{"D2"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks(task_id, work_type, title, status, project_id, package_id, plan_id) VALUES ('WT-D','project_work','업무','waiting','DROP',?,?)`, dp, dplan); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MergeProjects("KEEP", "PLAIN", "plain"); err != nil {
		t.Fatal(err)
	}
	var plainSite string
	_ = db.QueryRow(`SELECT project_id FROM project_sites WHERE id='P1'`).Scan(&plainSite)
	if plainSite != "PLAIN" {
		t.Fatalf("계획 없는 통합이 사이트를 옮김 %s", plainSite)
	}
	if _, err := repo.MergeProjects("KEEP", "DROP", "drop"); err != nil {
		t.Fatal(err)
	}
	var taskPlan, taskProj string
	_ = db.QueryRow(`SELECT plan_id, project_id FROM work_tasks WHERE task_id='WT-D'`).Scan(&taskPlan, &taskProj)
	if taskProj != "KEEP" || taskPlan != kplan {
		t.Fatalf("계획 병합 %s %s", taskProj, taskPlan)
	}
	var moved string
	_ = db.QueryRow(`SELECT project_id FROM project_sites WHERE site_name='다른학교'`).Scan(&moved)
	if moved != "KEEP" {
		t.Fatalf("풀 이관 %s", moved)
	}
	mergeID, err := repo.LatestMergeFor("KEEP", "DROP")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.UndoMerge(mergeID); err != nil {
		t.Fatal(err)
	}
	_ = db.QueryRow(`SELECT plan_id FROM work_tasks WHERE task_id='WT-D'`).Scan(&taskPlan)
	if taskPlan != dplan {
		t.Fatalf("되돌린 계획 %s", taskPlan)
	}
}

func Test95_175A_ProgressQueryCap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "q.db")
	db, err := InitDB(path)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	var n int64
	db, err = openQueryCountDB(path, &n)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		clearQueryCount()
		db.Close()
	})
	repo := NewProjectRepo(db)
	atomic.StoreInt64(&n, 0)
	if _, err := repo.StructureProgress(nil); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt64(&n) != 0 {
		t.Fatalf("빈 조회 %d", n)
	}
	atomic.StoreInt64(&n, 0)
	ids := []string{"A", "B", "C", "D", "E"}
	if _, err := repo.StructureProgress(ids); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt64(&n); got > 4 {
		t.Fatalf("쿼리 %d", got)
	}
}

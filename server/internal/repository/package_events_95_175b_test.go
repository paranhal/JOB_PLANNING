package repository

import (
	"database/sql"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func Test95_175B_SiteCheckMatchesPlan(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "b.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewProjectRepo(db)
	wb := NewWBRepo(db)
	if _, err := db.Exec(`INSERT INTO work_projects (project_id, name, status, org_id) VALUES ('WP','구조','active','O01')`); err != nil {
		t.Fatal(err)
	}
	ph := &model.ProjectPhase{ProjectID: "WP", OrgID: "O01", Name: "구축", Status: model.PhaseActive}
	if err := repo.CreatePhase(ph); err != nil {
		t.Fatal(err)
	}
	pkg, err := repo.CreatePackage(ph.PhaseID, "현장", "site", 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []string{"C1", "C2", "C3"} {
		if _, err := db.Exec(`INSERT INTO customers(customer_id, org_name, official_name, is_active, org_id) VALUES (?,?,?,1,'O01')`, c, "홍천"+c, c); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO project_sites(id, project_id, site_name, region, status, org_id, customer_id) VALUES (?,?,?,?, 'planned','O01',?)`,
			"S"+c, "WP", "홍천"+c+"초등학교", "홍천", c); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := repo.CreatePlan("WP", PlanDraft{PackageID: pkg, Name: "홍천", SiteIDs: []string{"SC1", "SC2", "SC3"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE package_sites SET target_qty=10 WHERE plan_id=?`, plan); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks(task_id, work_type, title, status, project_id, package_id, plan_id, work_date, assignee) VALUES
		('WT1','project_work','오늘','waiting','WP',?,?,'2026-11-10','김담당'),
		('WT2','project_work','다음날','waiting','WP',?,?,'2026-11-11','이후')`,
		pkg, plan, pkg, plan); err != nil {
		t.Fatal(err)
	}
	for _, c := range []string{"C1", "C2", "C3"} {
		if _, err := db.Exec(`INSERT INTO work_task_sites(task_id, customer_id) VALUES ('WT1',?)`, c); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO work_task_sites(task_id, customer_id) VALUES ('WT2','C1')`); err != nil {
		t.Fatal(err)
	}
	if err := wb.UpsertSiteCheckQty("WT1", "C1", "2026-11-10", model.SiteCheckDone, "u1", -1); err != nil {
		t.Fatal(err)
	}
	if err := wb.UpsertSiteCheckQty("WT1", "C1", "2026-11-10", model.SiteCheckDone, "u1", -1); err != nil {
		t.Fatal(err)
	}
	var evN, delta int
	var evPlan string
	if err := db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(qty_delta),0), COALESCE(MAX(plan_id),'') FROM package_events WHERE event_type='complete'`).Scan(&evN, &delta, &evPlan); err != nil {
		t.Fatal(err)
	}
	if evN != 1 || delta != 10 || evPlan != plan {
		t.Fatalf("완료 이벤트 %d %d %s", evN, delta, evPlan)
	}
	assertSameRate(t, repo, wb, "WP", plan, "WT1", 1, 3)
	if err := wb.UpsertSiteCheckQty("WT1", "C2", "2026-11-10", model.SiteCheckPending, "u1", 4); err != nil {
		t.Fatal(err)
	}
	var doneQty, startN int
	_ = db.QueryRow(`SELECT done_qty FROM package_sites ps JOIN project_sites s ON s.id=ps.project_site_id WHERE ps.plan_id=? AND s.customer_id='C2'`, plan).Scan(&doneQty)
	_ = db.QueryRow(`SELECT COUNT(*) FROM package_events WHERE event_type='start' AND plan_id=?`, plan).Scan(&startN)
	if doneQty != 4 || startN != 1 {
		t.Fatalf("부분 수량 %d 이벤트 %d", doneQty, startN)
	}
	open, err := repo.ListOpenSites("WP", pkg, plan, "ㅎㅊ", "홍천", "2026-11-10", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(open.Sites) != 2 || !open.Sites[0].Continue || open.Sites[0].CustomerID != "C2" {
		t.Fatalf("이어서 정렬 %+v", open.Sites)
	}
	if open.Sites[0].Assigned == "" {
		t.Fatal("배정 경고가 없습니다")
	}
	if err := wb.UpsertSiteCheck("WT2", "C2", "2026-11-11", model.SiteCheckDone, "u2"); err != nil {
		t.Fatal(err)
	}
	var who string
	_ = db.QueryRow(`SELECT user_id FROM package_events WHERE event_type='complete' AND event_date='2026-11-11'`).Scan(&who)
	if who != "u2" {
		t.Fatalf("다음 날 완료 사용자 %s", who)
	}
	assertSameRate(t, repo, wb, "WP", plan, "WT1", 2, 3)
	if err := wb.UpsertSiteCheck("WT2", "C1", "2026-11-11", model.SiteCheckDone, "u2"); err != nil {
		t.Fatal(err)
	}
	var completes int
	_ = db.QueryRow(`SELECT COUNT(*) FROM package_events WHERE event_type='complete' AND work_task_id='WT1' AND event_date='2026-11-10'`).Scan(&completes)
	if completes != 1 {
		t.Fatalf("같은 사이트 완료가 중복 %d", completes)
	}
	assertSameRate(t, repo, wb, "WP", plan, "WT1", 2, 3)
	if err := wb.UpsertSiteCheck("WT1", "C3", "2026-11-10", model.SiteCheckNA, "u1"); err != nil {
		t.Fatal(err)
	}
	assertSameRate(t, repo, wb, "WP", plan, "WT1", 2, 2)
	var dayDone int
	if err := db.QueryRow(`SELECT done_sites FROM package_progress_daily WHERE plan_id=? AND snap_date='2026-11-11'`, plan).Scan(&dayDone); err != nil {
		t.Fatal(err)
	}
	if dayDone != 2 {
		t.Fatalf("일 스냅샷 %d", dayDone)
	}
	before := dumpSnaps(t, db, pkg)
	if err := wb.RebuildPackageProgress(pkg); err != nil {
		t.Fatal(err)
	}
	if after := dumpSnaps(t, db, pkg); before != after {
		t.Fatalf("재계산 불일치\n%s\n%s", before, after)
	}
}

func assertSameRate(t *testing.T, repo *ProjectRepo, wb *WBRepo, projectID, planID, taskID string, wantDone, wantTotal int) {
	t.Helper()
	done, total, _, _ := countPackageSites(repo.db, "", planID)
	rd, rt, label := wb.PlanSiteCheckRate(planID)
	view := wb.PlanProgressView(taskID)
	tree, err := repo.LoadStructure(projectID, "2026-11-12", nil)
	if err != nil {
		t.Fatal(err)
	}
	prog := ""
	for _, ph := range tree.Phases {
		for _, pk := range ph.Packages {
			for _, pl := range pk.Plans {
				if pl.ID == planID {
					prog = pl.Progress
				}
			}
		}
	}
	if done != wantDone || total != wantTotal || rd != wantDone || rt != wantTotal || view.Done != wantDone || view.Total != wantTotal {
		t.Fatalf("합계 불일치 구조 %d/%d 119 %d/%d 조치 %d/%d 기대 %d/%d", done, total, rd, rt, view.Done, view.Total, wantDone, wantTotal)
	}
	want := model.SiteCheckRateLabel(wantDone, wantTotal)
	if label != want || !strings.Contains(prog, want) || !strings.Contains(view.Chip, want) {
		t.Fatalf("표시 불일치 %s %s %s", label, prog, view.Chip)
	}
}

func dumpSnaps(t *testing.T, db *sql.DB, packageID string) string {
	t.Helper()
	rows, err := db.Query(`SELECT plan_id, snap_date, total_sites, done_sites, doing_sites, total_qty, done_qty, day_done_sites, day_done_qty
		FROM package_progress_daily WHERE package_id=? ORDER BY plan_id, snap_date`, packageID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var plan, date string
		var a, c, d, e, f, g, h int
		if err := rows.Scan(&plan, &date, &a, &c, &d, &e, &f, &g, &h); err != nil {
			t.Fatal(err)
		}
		b.WriteString(plan + "|" + date + "|" + strings.Join([]string{
			strconv.Itoa(a), strconv.Itoa(c), strconv.Itoa(d), strconv.Itoa(e), strconv.Itoa(f), strconv.Itoa(g), strconv.Itoa(h),
		}, ",") + "\n")
	}
	return b.String()
}

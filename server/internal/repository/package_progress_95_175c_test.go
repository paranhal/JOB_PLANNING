package repository

import (
	"database/sql"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"customer-support/internal/model"
)

func Test95_175C_ReportMatchesTreeAndChip(t *testing.T) {
	db, repo, wb, plan, pkg := progressFixture(t)
	if err := wb.UpsertSiteCheck("WT1", "C1", "2026-11-10", model.SiteCheckDone, "u1"); err != nil {
		t.Fatal(err)
	}
	tree, err := repo.LoadStructurePeriod("WP", "2026-11-12", "today", model.WeekendBiz)
	if err != nil {
		t.Fatal(err)
	}
	lines := model.BuildProjectProgressReport(tree.Report)
	joined := strings.Join(lines, "\n")
	prog := ""
	for _, ph := range tree.Phases {
		for _, pk := range ph.Packages {
			for _, pl := range pk.Plans {
				if pl.ID == plan {
					prog = pl.Progress
				}
			}
		}
	}
	view := wb.PlanProgressView("WT1")
	want := model.SiteCheckRateLabel(1, 3)
	for _, s := range []string{"3곳 중 1곳", want, "1/3"} {
		if s == "1/3" {
			if !strings.Contains(prog, s) || !strings.Contains(view.Chip, s) {
				t.Fatalf("분수 %s prog %s chip %s", s, prog, view.Chip)
			}
			continue
		}
		if !strings.Contains(joined, s) || !strings.Contains(prog, want) || !strings.Contains(view.Chip, want) {
			t.Fatalf("문장 불일치\n%s\n%s\n%s", joined, prog, view.Chip)
		}
	}
	if !strings.Contains(tree.ChartSVG, "<polyline") {
		t.Fatal("차트")
	}
	var reached, confirmed string
	if err := db.QueryRow(`SELECT reached_at, confirmed_at FROM package_milestones WHERE plan_id=? AND threshold_pct=25`, plan).Scan(&reached, &confirmed); err != nil {
		t.Fatal(err)
	}
	if reached == "" || confirmed != "" {
		t.Fatalf("25%% 도달 %s 확인 %s", reached, confirmed)
	}
	var n50 string
	_ = db.QueryRow(`SELECT reached_at FROM package_milestones WHERE plan_id=? AND threshold_pct=50`, plan).Scan(&n50)
	if n50 != "" {
		t.Fatalf("50%% 는 아직 %s", n50)
	}
	_ = pkg
}

func Test95_175C_MilestoneDropAndConfirm(t *testing.T) {
	db, repo, wb, plan, _ := progressFixture(t)
	if _, err := db.Exec(`UPDATE package_milestones SET billing_id='BILL-1' WHERE plan_id=? AND threshold_pct=25`, plan); err != nil {
		t.Fatal(err)
	}
	if err := wb.UpsertSiteCheck("WT1", "C1", "2026-11-10", model.SiteCheckDone, "u1"); err != nil {
		t.Fatal(err)
	}
	var reached, dropped, confirmed, bill, status string
	q := `SELECT reached_at, dropped_at, confirmed_at, billing_id FROM package_milestones WHERE plan_id=? AND threshold_pct=25`
	if err := db.QueryRow(q, plan).Scan(&reached, &dropped, &confirmed, &bill); err != nil {
		t.Fatal(err)
	}
	if reached == "" || dropped != "" || confirmed != "" || bill != "BILL-1" {
		t.Fatalf("도달 %s %s %s %s", reached, dropped, confirmed, bill)
	}
	if err := wb.UpsertSiteCheckQty("WT1", "C1", "2026-11-10", model.SiteCheckPending, "u1", -1); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(q, plan).Scan(&reached, &dropped, &confirmed, &bill); err != nil {
		t.Fatal(err)
	}
	if reached == "" || dropped == "" || confirmed != "" {
		t.Fatalf("재하락 %s %s %s", reached, dropped, confirmed)
	}
	if err := wb.UpsertSiteCheck("WT1", "C1", "2026-11-10", model.SiteCheckDone, "u1"); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(q, plan).Scan(&reached, &dropped, &confirmed, &bill); err != nil {
		t.Fatal(err)
	}
	if dropped != "" || confirmed != "" || bill != "BILL-1" {
		t.Fatalf("재도달 %s %s %s", dropped, confirmed, bill)
	}
	_ = db.QueryRow(`SELECT status FROM package_sites WHERE plan_id=?`, plan).Scan(&status)
	var id string
	if err := db.QueryRow(`SELECT milestone_id FROM package_milestones WHERE plan_id=? AND threshold_pct=25`, plan).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetMilestoneConfirm("WP", id, "u9", "메모", true); err != nil {
		t.Fatal(err)
	}
	var by, note, after string
	if err := db.QueryRow(`SELECT confirmed_by, note, billing_id FROM package_milestones WHERE milestone_id=?`, id).Scan(&by, &note, &bill); err != nil {
		t.Fatal(err)
	}
	_ = db.QueryRow(`SELECT status FROM package_sites WHERE plan_id=?`, plan).Scan(&after)
	if by != "u9" || note != "메모" || bill != "BILL-1" || after != status {
		t.Fatalf("확인 %s %s %s 상태 %s→%s", by, note, bill, status, after)
	}
	if err := repo.SetMilestoneConfirm("WP", id, "u9", "", false); err != nil {
		t.Fatal(err)
	}
	var when string
	_ = db.QueryRow(`SELECT confirmed_at FROM package_milestones WHERE milestone_id=?`, id).Scan(&when)
	if when != "" {
		t.Fatal("해제")
	}
}

func Test95_175C_QueryCap(t *testing.T) {
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
	if _, err := repo.ProgressLines(nil, "2026-11-12", "2026-11-12", "2026-11-12", model.WeekendBiz); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt64(&n) != 0 {
		t.Fatalf("빈 조회 %d", n)
	}
	ids := make([]string, 50)
	for i := range ids {
		ids[i] = "P" + model.FormatIntComma(i)
	}
	atomic.StoreInt64(&n, 0)
	if _, err := repo.ProgressLines(ids, "2026-11-01", "2026-11-12", "2026-11-12", model.WeekendBiz); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt64(&n); got > 8 {
		t.Fatalf("50건 쿼리 %d", got)
	}
	if _, err := db.Exec(`INSERT INTO work_projects(project_id, name, status, org_id) VALUES ('WP','q','active','O01')`); err != nil {
		t.Fatal(err)
	}
	ph := &model.ProjectPhase{ProjectID: "WP", OrgID: "O01", Name: "구축", Status: model.PhaseActive}
	if err := repo.CreatePhase(ph); err != nil {
		t.Fatal(err)
	}
	pkg, err := repo.CreatePackage(ph.PhaseID, "현장", "site")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		if _, err := db.Exec(`INSERT INTO package_plans(plan_id, package_id, seq, name, status, is_active, created_at, updated_at) VALUES (?,?,?,?,'auto',1,'t','t')`,
			"PLX-"+model.FormatIntComma(i), pkg, i, "계획"+model.FormatIntComma(i)); err != nil {
			t.Fatal(err)
		}
	}
	atomic.StoreInt64(&n, 0)
	if _, err := repo.ProgressLines([]string{"WP"}, "2026-11-01", "2026-11-12", "2026-11-12", model.WeekendBiz); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt64(&n); got > 8 {
		t.Fatalf("계획 100 쿼리 %d", got)
	}
}

func Test95_175C_RetroSnapAndExtraPlans(t *testing.T) {
	db, repo, wb, plan, pkg := progressFixture(t)
	if err := wb.UpsertSiteCheck("WT1", "C1", "2026-11-10", model.SiteCheckDone, "u1"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE package_events SET event_date='2026-11-01' WHERE plan_id=?`, plan); err != nil {
		t.Fatal(err)
	}
	if err := wb.RebuildPackageProgress(pkg); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM package_progress_daily WHERE plan_id=? AND snap_date='2026-11-01'`, plan).Scan(&n)
	if n != 1 {
		t.Fatalf("소급 스냅 %d", n)
	}
	var both int
	_ = db.QueryRow(`SELECT COUNT(*) FROM package_progress_daily WHERE package_id=? AND snap_date='2026-11-01' AND plan_id IN (?, '')`, pkg, plan).Scan(&both)
	if both != 2 {
		t.Fatalf("계획 행·전체 행 %d", both)
	}
	for i, name := range []string{"가", "나", "다"} {
		p, err := repo.CreatePlan("WP", PlanDraft{PackageID: pkg, Name: name})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO customers(customer_id, org_name, official_name, is_active, org_id) VALUES (?,?,?,1,'O01')`, "X"+name, name, name); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO project_sites(id, project_id, site_name, status, org_id, customer_id) VALUES (?,?,?,'planned','O01',?)`,
			"SX"+name, "WP", name, "X"+name); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO package_sites(package_site_id, package_id, project_site_id, status, plan_id, updated_at) VALUES (?,?,?,'in_progress',?, 't')`,
			"PSX"+name, pkg, "SX"+name, p); err != nil {
			t.Fatal(err)
		}
		_ = i
	}
	lines, err := repo.ProgressLines([]string{"WP"}, "2026-11-12", "2026-11-12", "2026-11-12", model.WeekendBiz)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(lines["WP"].Text, "외 1개 계획") {
		t.Fatalf("여러 계획 %s", lines["WP"].Text)
	}
}

func progressFixture(t *testing.T) (*sql.DB, *ProjectRepo, *WBRepo, string, string) {
	t.Helper()
	db, err := InitDB(filepath.Join(t.TempDir(), "c.db"))
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
	pkg, err := repo.CreatePackage(ph.PhaseID, "현장", "site")
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
	plan, err := repo.CreatePlan("WP", PlanDraft{
		PackageID: pkg, Name: "홍천 지역", PlanEnd: "2026-11-30", SiteIDs: []string{"SC1", "SC2", "SC3"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks(task_id, work_type, title, status, project_id, package_id, plan_id, work_date) VALUES ('WT1','project_work','오늘','waiting','WP',?,?,'2026-11-10')`, pkg, plan); err != nil {
		t.Fatal(err)
	}
	return db, repo, wb, plan, pkg
}

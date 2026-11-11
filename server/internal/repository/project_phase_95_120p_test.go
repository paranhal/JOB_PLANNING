package repository

import (
	"path/filepath"
	"sync/atomic"
	"testing"

	"customer-support/internal/model"
)

func Test95_120P_MigrationKeepsProjects(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "ph.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO work_projects (project_id, name, status) VALUES ('WP-KEEP-120','유지','active')`); err != nil {
		t.Fatal(err)
	}
	var before int
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_projects`).Scan(&before); err != nil || before < 1 {
		t.Fatalf("projects %d %v", before, err)
	}
	var codes int
	if err := db.QueryRow(`SELECT COUNT(*) FROM codes WHERE code_group='phase_tpl_build'`).Scan(&codes); err != nil || codes != 6 {
		t.Fatalf("build codes %d %v", codes, err)
	}
	repo := NewProjectRepo(db)
	row := &model.ProjectPhase{ProjectID: "WP-KEEP-120", OrgID: "O01", Name: "기존단계", Status: model.PhasePlanned}
	if err := repo.CreatePhase(row); err != nil {
		t.Fatal(err)
	}
	applyProjectPhasesV108(db)
	got, err := repo.GetPhase(row.PhaseID)
	if err != nil || got.Name != "기존단계" || got.Status != model.PhasePlanned {
		t.Fatalf("재적용 후 %+v %v", got, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM codes WHERE code_group='phase_tpl_build'`).Scan(&codes); err != nil || codes != 6 {
		t.Fatalf("시드 중복 %d", codes)
	}
	down, err := projectPhaseSQL("108_project_phases.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := execSQLIgnoreMissing(db, down); err != nil {
		t.Fatal(err)
	}
	var table int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='project_phases'`).Scan(&table); err != nil || table != 0 {
		t.Fatalf("down 후 표 %d", table)
	}
	var after int
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_projects`).Scan(&after); err != nil || after != before {
		t.Fatalf("사업 수 %d → %d", before, after)
	}
	var name string
	if err := db.QueryRow(`SELECT name FROM work_projects WHERE project_id='WP-KEEP-120'`).Scan(&name); err != nil || name != "유지" {
		t.Fatalf("사업 행 %q %v", name, err)
	}
	applyProjectPhasesV108(db)
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='project_phases'`).Scan(&table); err != nil || table != 1 {
		t.Fatal("up 재실행")
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM codes WHERE code_group='phase_tpl_develop'`).Scan(&codes); err != nil || codes != 6 {
		t.Fatalf("develop codes %d", codes)
	}
}

func Test95_120P_CRUDStatusMerge(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "crud.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO work_projects (project_id, name, status, org_id) VALUES
		('KEEP','남김','active','O01'), ('DROP','없앰','active','O01')`); err != nil {
		t.Fatal(err)
	}
	repo := NewProjectRepo(db)
	a := &model.ProjectPhase{ProjectID: "KEEP", OrgID: "O01", Name: "착수", Status: model.PhasePlanned}
	b := &model.ProjectPhase{ProjectID: "KEEP", OrgID: "O01", Name: "설계", PlanStart: "2026-03-02", PlanEnd: "2026-03-01"}
	if err := repo.CreatePhase(a); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreatePhase(b); err == nil {
		t.Fatal("날짜 순서가 통과했다")
	}
	b.PlanStart, b.PlanEnd = "2026-03-01", "2026-03-10"
	if err := repo.CreatePhase(b); err != nil {
		t.Fatal(err)
	}
	dup := &model.ProjectPhase{ProjectID: "KEEP", OrgID: "O01", Name: "착수"}
	if err := repo.CreatePhase(dup); err == nil {
		t.Fatal("중복 이름")
	}
	long := ""
	for i := 0; i < 101; i++ {
		long += "가"
	}
	if err := repo.CreatePhase(&model.ProjectPhase{ProjectID: "KEEP", OrgID: "O01", Name: long}); err == nil {
		t.Fatal("101자")
	}
	n, err := repo.SetPhaseStatus(b.PhaseID, model.PhaseActive, "2026-11-11", false)
	if err != ErrPhasePriorConfirm || n != 1 {
		t.Fatalf("확인 전 %d %v", n, err)
	}
	still, _ := repo.GetPhase(a.PhaseID)
	if still.Status != model.PhasePlanned {
		t.Fatal("확인 전에 저장됐다")
	}
	if _, err := repo.SetPhaseStatus(b.PhaseID, model.PhaseActive, "2026-11-11", true); err != nil {
		t.Fatal(err)
	}
	still, _ = repo.GetPhase(a.PhaseID)
	second, _ := repo.GetPhase(b.PhaseID)
	if still.Status != model.PhaseDone || still.ActualEnd != "2026-11-11" || second.Status != model.PhaseActive || second.ActualStart != "2026-11-11" {
		t.Fatalf("확인 후 %+v %+v", still, second)
	}
	if _, err := repo.SetPhaseStatus(second.PhaseID, model.PhaseDone, "2026-11-11", false); err != nil {
		t.Fatal(err)
	}
	second, _ = repo.GetPhase(b.PhaseID)
	if second.Status != model.PhaseDone || second.ActualEnd != "2026-11-11" {
		t.Fatalf("완료 %+v", second)
	}
	if _, err := repo.SetPhaseStatus(second.PhaseID, model.PhaseActive, "2026-11-11", false); err != nil {
		t.Fatal(err)
	}
	second, _ = repo.GetPhase(b.PhaseID)
	if second.Status != model.PhaseActive {
		t.Fatal("완료를 진행으로 되돌리지 못했다")
	}
	if err := repo.MovePhase(b.PhaseID, -1); err != nil {
		t.Fatal(err)
	}
	list, err := repo.ListPhases("KEEP")
	if err != nil || len(list) != 2 || list[0].PhaseID != b.PhaseID {
		t.Fatalf("순서 %+v %v", list, err)
	}
	if _, err := repo.DeletePhase(a.PhaseID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetPhase(a.PhaseID); err == nil {
		t.Fatal("삭제 후 남아 있다")
	}
	off := &model.ProjectPhase{ProjectID: "KEEP", OrgID: "O01", Name: "숨김", Status: model.PhasePlanned}
	if err := repo.CreatePhase(off); err != nil {
		t.Fatal(err)
	}
	off.IsActive = false
	if err := repo.UpdatePhase(off); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.GetPhase(off.PhaseID)
	if got.IsActive {
		t.Fatal("비활성")
	}
	if err := repo.SetPhaseActive(off.PhaseID, true); err != nil {
		t.Fatal(err)
	}

	same := &model.ProjectPhase{ProjectID: "DROP", OrgID: "O01", Name: "설 계", Status: model.PhaseDone, Note: "현장", PlanEnd: "2026-04-01"}
	other := &model.ProjectPhase{ProjectID: "DROP", OrgID: "O01", Name: "시공", Status: model.PhasePlanned}
	if err := repo.CreatePhase(same); err != nil || repo.CreatePhase(other) != nil {
		t.Fatal("drop 단계")
	}
	keepDesign, _ := repo.GetPhase(b.PhaseID)
	mergeID, err := repo.MergeProjects("KEEP", "DROP", "backup-120")
	if err != nil {
		t.Fatal(err)
	}
	merged, err := repo.ListPhases("KEEP")
	if err != nil {
		t.Fatal(err)
	}
	if len(merged) < 3 {
		t.Fatalf("합친 단계 %d", len(merged))
	}
	var design model.ProjectPhase
	for _, p := range merged {
		if p.PhaseID == keepDesign.PhaseID {
			design = p
		}
	}
	if design.Status != model.PhaseDone || design.PlanEnd != "2026-03-10" || design.Note != "현장" {
		t.Fatalf("같은 이름 병합 %+v", design)
	}
	var moves int
	if err := db.QueryRow(`SELECT COUNT(*) FROM project_merge_moves WHERE merge_id=? AND table_name='project_phases'`, mergeID).Scan(&moves); err != nil || moves != 1 {
		t.Fatalf("undo 행 %d %v", moves, err)
	}
	if err := repo.UndoMerge(mergeID); err != nil {
		t.Fatal(err)
	}
	back, err := repo.ListPhases("DROP")
	if err != nil || len(back) != 2 {
		t.Fatalf("되돌린 drop %+v %v", back, err)
	}
	restored, _ := repo.GetPhase(keepDesign.PhaseID)
	if restored.Status != model.PhaseActive || restored.ProjectID != "KEEP" {
		t.Fatalf("되돌린 keep %+v", restored)
	}
	if err := repo.Delete("KEEP"); err != nil {
		t.Fatal(err)
	}
	left, _ := repo.ListPhases("KEEP")
	if len(left) != 0 {
		t.Fatalf("삭제 후 단계 %d", len(left))
	}
}

func Test95_120P_PhaseSummaryQueryOnce(t *testing.T) {
	dir := t.TempDir()
	var n int64
	db, err := openQueryCountDB(filepath.Join(dir, "q.db"), &n)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		clearQueryCount()
		db.Close()
	})
	if _, err := db.Exec(`CREATE TABLE project_phases (
		phase_id TEXT PRIMARY KEY, project_id TEXT, org_id TEXT, seq INTEGER,
		name TEXT, plan_start TEXT, plan_end TEXT, actual_start TEXT, actual_end TEXT,
		status TEXT, note TEXT, is_active INTEGER, created_at TEXT, updated_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 30)
	for i := 0; i < 30; i++ {
		id := "P" + string(rune('A'+i/26)) + string(rune('a'+i%26))
		ids[i] = id
		if _, err := db.Exec(`INSERT INTO project_phases VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			"PH"+id, id, "O01", 0, "착수", "", "2026-01-01", "", "", "active", "", 1, "t", "t"); err != nil {
			t.Fatal(err)
		}
	}
	repo := NewProjectRepo(db)
	atomic.StoreInt64(&n, 0)
	empty, err := repo.PhaseSummaries(nil, "2026-11-11")
	if err != nil || len(empty) != 0 || atomic.LoadInt64(&n) != 0 {
		t.Fatalf("빈 목록 쿼리 %d %+v %v", atomic.LoadInt64(&n), empty, err)
	}
	sums, err := repo.PhaseSummaries(ids, "2026-11-11")
	if err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt64(&n) != 1 {
		t.Fatalf("쿼리 %d", atomic.LoadInt64(&n))
	}
	if sums[ids[0]].Current != "착수" || !sums[ids[0]].Late || sums["없는"].Current != "" && sums["없는"].Current != "—" {
		t.Fatalf("요약 %+v", sums[ids[0]])
	}
	missing, ok := sums["없는"]
	if ok && missing.Current != "—" && missing.Current != "" {
		t.Fatalf("없는 사업 %+v", missing)
	}
	asked := sums[ids[29]]
	if asked.Current != "착수" || asked.Label() != "완료 0 / 전체 1" {
		t.Fatalf("마지막 %+v %s", asked, asked.Label())
	}
}

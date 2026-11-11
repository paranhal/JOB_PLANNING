package repository

import (
	"database/sql"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"

	"customer-support/internal/model"
)

func seed176Project(t *testing.T, db *sql.DB, id, name, org, kind string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO work_projects (project_id, name, status, org_id, project_kind) VALUES (?,?, 'active', ?, ?)`,
		id, name, org, kind); err != nil {
		t.Fatal(err)
	}
}

func seed176PhasePkg(t *testing.T, repo *ProjectRepo, projectID, org, phase, pkg, track string, weight float64) string {
	t.Helper()
	row := &model.ProjectPhase{ProjectID: projectID, OrgID: org, Name: phase, Status: model.PhasePlanned}
	if err := repo.CreatePhase(row); err != nil {
		t.Fatal(err)
	}
	id, err := repo.CreatePackage(row.PhaseID, pkg, track, weight)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func Test95_176_CopyOrderSkipWeightNoPlans(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	seed176Project(t, db, "SRC", "원본", "O02", "build")
	seed176Project(t, db, "DST", "대상", "O03", "build")
	repo := NewProjectRepo(db)
	seed176PhasePkg(t, repo, "SRC", "O02", "착수", "현장", model.TrackModeSite, 1)
	seed176PhasePkg(t, repo, "SRC", "O02", "설계", "도면", model.TrackModePhase, 2)
	if _, err := db.Exec(`UPDATE phase_packages SET next_note='비밀' WHERE name='도면'`); err != nil {
		t.Fatal(err)
	}
	ph, _ := repo.ListPhases("SRC")
	if _, err := repo.CreatePlan("SRC", PlanDraft{PackageID: packageIDByName(t, db, "도면"), Name: "1차", PlanStart: "2026-01-01", PlanEnd: "2026-01-02"}); err != nil {
		t.Fatal(err)
	}
	_ = ph
	stat, err := repo.CopyStructure("DST", "SRC", "O03")
	if err != nil {
		t.Fatal(err)
	}
	if stat.PhasesAdded != 2 || stat.PackagesAdded != 2 || stat.PhasesSkipped != 0 {
		t.Fatalf("stat %+v", stat)
	}
	got, _ := repo.ListPhases("DST")
	if len(got) != 2 || got[0].Name != "착수" || got[1].Name != "설계" || got[0].PlanStart != "" || got[0].Status != model.PhasePlanned {
		t.Fatalf("순서·날짜 %+v", got)
	}
	var track string
	var weight float64
	var note string
	if err := db.QueryRow(`SELECT track_mode, weight, next_note FROM phase_packages WHERE name='도면'`).Scan(&track, &weight, &note); err != nil {
		t.Fatal(err)
	}
	// 원본과 복사본이 둘 다 있다. 대상 쪽만 본다.
	if err := db.QueryRow(`
		SELECT pk.track_mode, pk.weight, pk.next_note FROM phase_packages pk
		JOIN project_phases ph ON ph.phase_id=pk.phase_id
		WHERE ph.project_id='DST' AND pk.name='도면'`).Scan(&track, &weight, &note); err != nil {
		t.Fatal(err)
	}
	if track != model.TrackModePhase || weight != 1 || note != "" {
		t.Fatalf("복사본 %s %v %q", track, weight, note)
	}
	var plans int
	_ = db.QueryRow(`SELECT COUNT(*) FROM package_plans pl
		JOIN phase_packages pk ON pk.package_id=pl.package_id
		JOIN project_phases ph ON ph.phase_id=pk.phase_id WHERE ph.project_id='DST'`).Scan(&plans)
	if plans != 0 {
		t.Fatalf("계획이 복사됨 %d", plans)
	}
	var ms int
	_ = db.QueryRow(`SELECT COUNT(*) FROM package_milestones ms
		JOIN phase_packages pk ON pk.package_id=ms.package_id
		JOIN project_phases ph ON ph.phase_id=pk.phase_id WHERE ph.project_id='DST'`).Scan(&ms)
	if ms != 8 {
		t.Fatalf("마일스톤 %d", ms)
	}
	again, err := repo.CopyStructure("DST", "SRC", "O03")
	if err != nil || again.PhasesAdded != 0 || again.PackagesAdded != 0 || again.PhasesSkipped != 2 || again.PackagesSkipped != 2 {
		t.Fatalf("건너뜀 %+v %v", again, err)
	}
	if _, err := repo.CopyStructure("DST", "NONE", "O03"); !errors.Is(err, ErrNoStructureToCopy) {
		t.Fatalf("없는 원본 %v", err)
	}
}

func packageIDByName(t *testing.T, db *sql.DB, name string) string {
	t.Helper()
	var id string
	if err := db.QueryRow(`SELECT package_id FROM phase_packages WHERE name=?`, name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func Test95_176_CopyRollsBack(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "rb.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	seed176Project(t, db, "SRC", "원본", "O02", "build")
	seed176Project(t, db, "DST", "대상", "O03", "build")
	repo := NewProjectRepo(db)
	seed176PhasePkg(t, repo, "SRC", "O02", "착수", "현장", model.TrackModeSite, 1)
	structureWriteBeforeCommit = func() error { return errors.New("시험 중단") }
	t.Cleanup(func() { structureWriteBeforeCommit = nil })
	if _, err := repo.CopyStructure("DST", "SRC", "O03"); err == nil {
		t.Fatal("커밋됐다")
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM project_phases WHERE project_id='DST'`).Scan(&n)
	if n != 0 {
		t.Fatalf("롤백 안 됨 %d", n)
	}
}

func Test95_176_CopyQueryCap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "q.db")
	db, err := InitDB(path)
	if err != nil {
		t.Fatal(err)
	}
	seed176Project(t, db, "SRC", "원본", "O02", "build")
	seed176Project(t, db, "DST", "대상", "O03", "build")
	repo := NewProjectRepo(db)
	for i := 0; i < 20; i++ {
		row := &model.ProjectPhase{ProjectID: "SRC", OrgID: "O02", Name: "단계" + string(rune('A'+i)), Status: model.PhasePlanned}
		if err := repo.CreatePhase(row); err != nil {
			t.Fatal(err)
		}
		for j := 0; j < 5; j++ {
			if _, err := repo.CreatePackage(row.PhaseID, row.Name+"-"+string(rune('a'+j)), model.TrackModeSite, 1); err != nil {
				t.Fatal(err)
			}
		}
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
	repo = NewProjectRepo(db)
	atomic.StoreInt64(&n, 0)
	stat, err := repo.CopyStructure("DST", "SRC", "O03")
	if err != nil {
		t.Fatal(err)
	}
	if stat.PhasesAdded != 20 || stat.PackagesAdded != 100 {
		t.Fatalf("stat %+v", stat)
	}
	if got := atomic.LoadInt64(&n); got > 25 {
		t.Fatalf("질의 %d", got)
	}
}

func Test95_176_TemplateCompanyWide(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	seed176Project(t, db, "SRC", "원본", "O02", "build")
	seed176Project(t, db, "SI", "에스아이", "O03", "build")
	repo := NewProjectRepo(db)
	seed176PhasePkg(t, repo, "SRC", "O02", "착수", "현장", model.TrackModePhase, 2)
	id, err := repo.SaveStructureTemplate("SRC", "O02", "공통템플릿", "build", "", "u1")
	if err != nil || id == "" {
		t.Fatal(err)
	}
	if _, err := repo.SaveStructureTemplate("SRC", "O02", "공통템플릿", "build", "", "u1"); !errors.Is(err, ErrTemplateNameTaken) {
		t.Fatalf("중복 %v", err)
	}
	stat, err := repo.ApplyStructureTemplate("SI", "O03", id)
	if err != nil || stat.PhasesAdded != 1 || stat.PackagesAdded != 1 {
		t.Fatalf("적용 %+v %v", stat, err)
	}
	var track string
	var weight float64
	if err := db.QueryRow(`SELECT pk.track_mode, pk.weight FROM phase_packages pk
		JOIN project_phases ph ON ph.phase_id=pk.phase_id WHERE ph.project_id='SI'`).Scan(&track, &weight); err != nil {
		t.Fatal(err)
	}
	if track != model.TrackModePhase || weight != 1 {
		t.Fatalf("템플릿은 비중을 가져오지 않음 %s %v", track, weight)
	}
	var before int
	_ = db.QueryRow(`SELECT COUNT(*) FROM project_phases WHERE project_id='SI'`).Scan(&before)
	if err := repo.DeactivateStructureTemplate(id); err != nil {
		t.Fatal(err)
	}
	active, err := repo.ListStructureTemplates(true)
	if err != nil || len(active) != 0 {
		t.Fatalf("비활성 목록 %+v %v", active, err)
	}
	var after int
	_ = db.QueryRow(`SELECT COUNT(*) FROM project_phases WHERE project_id='SI'`).Scan(&after)
	if after != before {
		t.Fatal("이미 적용된 사업이 바뀌었다")
	}
	if _, err := repo.ApplyStructureTemplate("SI", "O03", id); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("비활성 적용 %v", err)
	}
	if _, err := repo.ApplyStructureTemplate("SI", "O03", "ST-없는"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("없는 템플릿 %v", err)
	}
}

func Test95_176_Migration112(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	seed176Project(t, db, "KEEP", "유지", "O02", "maintenance")
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='structure_templates'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("표 %d %v", n, err)
	}
	down, err := leaveLedgerSQL("112_structure_templates.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := execSQLIgnoreMissing(db, down); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='structure_templates'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("down %d", n)
	}
	var name string
	if err := db.QueryRow(`SELECT name FROM work_projects WHERE project_id='KEEP'`).Scan(&name); err != nil || name != "유지" {
		t.Fatal(name, err)
	}
	applyStructureTemplatesV112(db)
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='structure_templates'`).Scan(&n); err != nil || n != 1 {
		t.Fatal("재적용")
	}
}

func Test95_176_ProgressIsolationAndMerge(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "i.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	seed176Project(t, db, "A", "미디어", "O02", "maintenance")
	seed176Project(t, db, "B", "에스아이", "O03", "build")
	seed176Project(t, db, "C", "합칠", "O03", "build")
	repo := NewProjectRepo(db)
	seed176PhasePkg(t, repo, "A", "O02", "점검", "순회", model.TrackModeSite, 1)
	if _, err := repo.CopyStructure("B", "A", "O03"); err != nil {
		t.Fatal(err)
	}
	got, err := repo.StructureProgress([]string{"A", "B"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["A"]; !ok {
		t.Fatal("A 없음")
	}
	if _, ok := got["B"]; !ok {
		t.Fatal("B 없음")
	}
	if _, err := repo.MergeProjects("B", "C", "시험"); err != nil {
		t.Fatal(err)
	}
}

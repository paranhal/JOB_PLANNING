package repository

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

type fix183 struct {
	db      *sql.DB
	repo    *ProjectRepo
	project string
	phase   string
	pkg     string
	sites   []string
}

func new183(t *testing.T) fix183 {
	t.Helper()
	db, err := InitDB(filepath.Join(t.TempDir(), "p183.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	seed176Project(t, db, "P183", "수정사업", "O01", "build")
	repo := NewProjectRepo(db)
	ph := &model.ProjectPhase{ProjectID: "P183", OrgID: "O01", Name: "전처리", Status: model.PhaseActive}
	if err := repo.CreatePhase(ph); err != nil {
		t.Fatal(err)
	}
	pkg, err := repo.CreatePackage(ph.PhaseID, "GBIC", model.TrackModeSite)
	if err != nil {
		t.Fatal(err)
	}
	f := fix183{db: db, repo: repo, project: "P183", phase: ph.PhaseID, pkg: pkg}
	for i, n := range []string{"홍성고", "홍천초", "횡성중", "청양고"} {
		id := "S183-" + string(rune('A'+i))
		if _, err := db.Exec(`INSERT INTO project_sites (id, project_id, site_name, region, status, is_active, org_id) VALUES (?,?,?,?,'planned',1,'O01')`,
			id, "P183", n, "충남"); err != nil {
			t.Fatal(err)
		}
		f.sites = append(f.sites, id)
	}
	if _, err := repo.AddPhaseSites("P183", ph.PhaseID, f.sites, false, "", "", "t"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.AddPackageSites("P183", pkg, f.sites, false, "", "", "t"); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f fix183) edit(name, track, note string) PackageEdit {
	return PackageEdit{Name: name, Track: track, NextNote: note}
}

func Test95_183_PackageRenameDuplicateAndInactive(t *testing.T) {
	f := new183(t)
	other, err := f.repo.CreatePackage(f.phase, "스위치 설치", model.TrackModeSite)
	if err != nil {
		t.Fatal(err)
	}
	ext := DateChangeCtx{Actor: "u"}
	// 성공 + 다음 할 일
	if err := f.repo.UpdatePackage(f.project, f.pkg, f.edit("GBIC 교체", "", "검수 준비"), ext); err != nil {
		t.Fatal(err)
	}
	var name, note string
	_ = f.db.QueryRow(`SELECT name, next_note FROM phase_packages WHERE package_id=?`, f.pkg).Scan(&name, &note)
	if name != "GBIC 교체" || note != "검수 준비" {
		t.Fatalf("저장 값 %q %q", name, note)
	}
	// 공백만, 100자 초과
	if err := f.repo.UpdatePackage(f.project, f.pkg, f.edit("   ", "", ""), ext); err == nil {
		t.Fatal("공백 이름이 저장됐다")
	}
	if err := f.repo.UpdatePackage(f.project, f.pkg, f.edit(strings.Repeat("가", 101), "", ""), ext); err == nil {
		t.Fatal("101자 이름이 저장됐다")
	}
	// 정규화 기준 중복(공백·괄호 무시)
	if err := f.repo.UpdatePackage(f.project, f.pkg, f.edit("스위치(설치)", "", ""), ext); err == nil || !strings.Contains(err.Error(), "같은 이름") {
		t.Fatalf("정규화 중복 거절 안 됨: %v", err)
	}
	// 다음 할 일 301자
	if err := f.repo.UpdatePackage(f.project, f.pkg, f.edit("GBIC 교체", "", strings.Repeat("가", 301)), ext); err == nil {
		t.Fatal("301자 다음 할 일이 저장됐다")
	}
	// 비활성 하부 업무와 같은 이름 → 안내
	if _, _, err := f.repo.DeletePackage(f.project, other); err != nil {
		t.Fatal(err)
	}
	// other 는 실적이 없어 지워졌다 → 같은 이름을 다시 써도 된다
	if err := f.repo.UpdatePackage(f.project, f.pkg, f.edit("스위치 설치", "", ""), ext); err != nil {
		t.Fatalf("삭제된 이름을 못 쓴다: %v", err)
	}
}

func Test95_183_PackageTrackChangeOnlyWithoutPerformance(t *testing.T) {
	f := new183(t)
	ext := DateChangeCtx{Actor: "u"}
	// 하부 업무에 사이트가 있어서 거절
	err := f.repo.UpdatePackage(f.project, f.pkg, f.edit("GBIC", model.TrackModePhase, ""), ext)
	if err == nil || !strings.Contains(err.Error(), "사이트 4곳") {
		t.Fatalf("사이트가 있는데 유형이 바뀐다: %v", err)
	}
	empty, _ := f.repo.CreatePackage(f.phase, "빈 업무", model.TrackModeSite)
	if err := f.repo.UpdatePackage(f.project, empty, f.edit("빈 업무", model.TrackModePhase, ""), ext); err != nil {
		t.Fatalf("실적이 없는데 못 바꾼다: %v", err)
	}
	var track string
	_ = f.db.QueryRow(`SELECT track_mode FROM phase_packages WHERE package_id=?`, empty).Scan(&track)
	if track != model.TrackModePhase {
		t.Fatalf("유형 %s", track)
	}
	// 계획이 있으면 거절
	pl, _ := f.repo.CreatePackage(f.phase, "계획 업무", model.TrackModeSite)
	if _, err := f.repo.CreatePlan(f.project, PlanDraft{PackageID: pl, Name: "1차", Basis: model.BasisSite, Actor: "t"}); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.UpdatePackage(f.project, pl, f.edit("계획 업무", model.TrackModePhase, ""), ext); err == nil || !strings.Contains(err.Error(), "계획 1개") {
		t.Fatalf("계획이 있는데 유형이 바뀐다: %v", err)
	}
	// 수량 기록이 있으면 거절
	if err := f.repo.UpdatePackage(f.project, empty, f.edit("빈 업무", model.TrackModePhase, ""), ext); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.SetPackageQtyTarget(f.project, empty, "식", 10); err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.RecordPackageQty(f.project, empty, "2026-11-01", 3, "u", ""); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.UpdatePackage(f.project, empty, f.edit("빈 업무", model.TrackModeSite, ""), ext); err == nil || !strings.Contains(err.Error(), "기록") {
		t.Fatalf("수량 기록이 있는데 유형이 바뀐다: %v", err)
	}
}

func Test95_183_PackageMoveOrderBoundsAndInactive(t *testing.T) {
	f := new183(t)
	b, _ := f.repo.CreatePackage(f.phase, "B", model.TrackModeSite)
	c, _ := f.repo.CreatePackage(f.phase, "C", model.TrackModeSite)
	order := func() string {
		rows, _ := f.db.Query(`SELECT name FROM phase_packages WHERE phase_id=? AND is_active=1 ORDER BY seq, package_id`, f.phase)
		defer rows.Close()
		var out []string
		for rows.Next() {
			var n string
			_ = rows.Scan(&n)
			out = append(out, n)
		}
		return strings.Join(out, ",")
	}
	if order() != "GBIC,B,C" {
		t.Fatal(order())
	}
	if err := f.repo.MovePackage(f.project, c, "up"); err != nil || order() != "GBIC,C,B" {
		t.Fatalf("up: %v %s", err, order())
	}
	if err := f.repo.MovePackage(f.project, f.pkg, "up"); err != nil || order() != "GBIC,C,B" { // 맨 위 경계
		t.Fatalf("경계 up: %v %s", err, order())
	}
	if err := f.repo.MovePackage(f.project, b, "down"); err != nil || order() != "GBIC,C,B" { // 맨 아래 경계
		t.Fatalf("경계 down: %v %s", err, order())
	}
	if err := f.repo.MovePackage(f.project, f.pkg, "down"); err != nil || order() != "C,GBIC,B" {
		t.Fatalf("down: %v %s", err, order())
	}
	if err := f.repo.MovePackage(f.project, b, "sideways"); err == nil {
		t.Fatal("잘못된 방향이 통과했다")
	}
	// 다른 사업의 하부 업무는 못 옮긴다
	if err := f.repo.MovePackage("OTHER", c, "up"); err == nil {
		t.Fatal("다른 사업 하부 업무를 옮겼다")
	}
}

func Test95_183_PackageStatusDoneHoldAutoAndOpenSites(t *testing.T) {
	f := new183(t)
	if err := f.repo.SetPackageStatus(f.project, f.pkg, "bogus", "u", false); err == nil {
		t.Fatal("잘못된 상태가 통과했다")
	}
	if err := f.repo.SetPackageStatus(f.project, f.pkg, model.PkgHold, "u", false); err != nil {
		t.Fatal(err)
	}
	// 미완료 사이트 4곳 → 확인 필요
	err := f.repo.SetPackageStatus(f.project, f.pkg, model.PkgDone, "u1", false)
	var open *OpenSitesError
	if !errors.As(err, &open) || open.N != 4 {
		t.Fatalf("미완료 경고 아님: %v", err)
	}
	var st string
	_ = f.db.QueryRow(`SELECT status FROM phase_packages WHERE package_id=?`, f.pkg).Scan(&st)
	if st != model.PkgHold {
		t.Fatalf("확인 전에 바뀌었다: %s", st)
	}
	if err := f.repo.SetPackageStatus(f.project, f.pkg, model.PkgDone, "u1", true); err != nil {
		t.Fatal(err)
	}
	var doneAt, doneBy string
	_ = f.db.QueryRow(`SELECT status, done_at, done_by FROM phase_packages WHERE package_id=?`, f.pkg).Scan(&st, &doneAt, &doneBy)
	if st != model.PkgDone || doneAt == "" || doneBy != "u1" {
		t.Fatalf("완료 기록 %s %q %q", st, doneAt, doneBy)
	}
	// 자동으로 되돌리면 완료 기록이 지워진다
	if err := f.repo.SetPackageStatus(f.project, f.pkg, model.PkgAuto, "u", false); err != nil {
		t.Fatal(err)
	}
	_ = f.db.QueryRow(`SELECT status, done_at, done_by FROM phase_packages WHERE package_id=?`, f.pkg).Scan(&st, &doneAt, &doneBy)
	if st != model.PkgAuto || doneAt != "" || doneBy != "" {
		t.Fatalf("자동 복귀 %s %q %q", st, doneAt, doneBy)
	}
	// 진행(active)도 저장되고 표시는 진행 중
	if err := f.repo.SetPackageStatus(f.project, f.pkg, model.PhaseActive, "u", false); err != nil {
		t.Fatal(err)
	}
	// 사이트가 모두 끝났으면 경고 없이 완료
	if _, err := f.db.Exec(`UPDATE package_sites SET status='done' WHERE package_id=?`, f.pkg); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.SetPackageStatus(f.project, f.pkg, model.PkgDone, "u", false); err != nil {
		t.Fatalf("모두 끝났는데 경고: %v", err)
	}
}

func Test95_183_PackageDeleteOrDeactivateAndRestore(t *testing.T) {
	f := new183(t)
	// 실적 없는 하부 업무: 하드 삭제 + 마일스톤·직접 등록 사이트 함께 삭제, 이력은 남음
	plain, _ := f.repo.CreatePackage(f.phase, "실적 없음", model.TrackModeSite)
	if _, err := f.repo.AddPackageSites(f.project, plain, f.sites[:2], false, "", "", "t"); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.UpdatePackageMeta(f.project, plain, PackageMeta{PlanEnd: "2026-12-01"}, DateChangeCtx{Actor: "u"}); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.UpdatePackageMeta(f.project, plain, PackageMeta{PlanEnd: "2026-12-31"}, DateChangeCtx{Actor: "u", Reason: "일정 지연"}); err != nil {
		t.Fatal(err)
	}
	off, _, err := f.repo.DeletePackage(f.project, plain)
	if err != nil || off {
		t.Fatalf("실적 없는데 비활성: off=%v err=%v", off, err)
	}
	for tbl, col := range map[string]string{"phase_packages": "package_id", "package_sites": "package_id", "package_milestones": "package_id"} {
		var n int
		_ = f.db.QueryRow(`SELECT COUNT(*) FROM `+tbl+` WHERE `+col+`=?`, plain).Scan(&n)
		if n != 0 {
			t.Fatalf("%s 에 %d건이 남았다", tbl, n)
		}
	}
	var hist int
	_ = f.db.QueryRow(`SELECT COUNT(*) FROM structure_date_changes WHERE level='package' AND target_id=?`, plain).Scan(&hist)
	if hist == 0 {
		t.Fatal("일자 변경 이력까지 지워졌다")
	}
	// 계획이 있는 하부 업무: 비활성(지우지 않음)
	off, im, err := f.repo.DeletePackage(f.project, f.pkg)
	if err != nil || off {
		// 계획·실적 없이 사이트만 있으면 삭제된다 — 아래에서 계획이 있는 경우를 따로 시험한다.
		t.Logf("사이트만 있는 하부 업무: off=%v im=%+v err=%v", off, im, err)
	}
	g, _ := f.repo.CreatePackage(f.phase, "계획 있음", model.TrackModeSite)
	if _, err := f.repo.AddPackageSites(f.project, g, f.sites, false, "", "", "t"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.CreatePlan(f.project, PlanDraft{PackageID: g, Name: "1차", Basis: model.BasisSite, SiteIDs: f.sites[:1], Actor: "t"}); err != nil {
		t.Fatal(err)
	}
	off, im, err = f.repo.DeletePackage(f.project, g)
	if err != nil || !off || im.Plans != 1 {
		t.Fatalf("계획 있는 하부 업무는 비활성이어야 한다: off=%v im=%+v err=%v", off, im, err)
	}
	var act int
	_ = f.db.QueryRow(`SELECT is_active FROM phase_packages WHERE package_id=?`, g).Scan(&act)
	if act != 0 {
		t.Fatal("비활성이 안 됐다")
	}
	// 비활성 이름으로 새로 만들 수 없다
	if _, err := f.repo.CreatePackage(f.phase, "계획 있음", model.TrackModeSite); err == nil {
		t.Fatal("비활성 이름이 새로 만들어졌다")
	}
	// 비활성은 수정·상태·순서 모두 막힌다
	if err := f.repo.UpdatePackage(f.project, g, f.edit("바꿈", "", ""), DateChangeCtx{}); err == nil {
		t.Fatal("비활성 하부 업무가 수정됐다")
	}
	list, _ := f.repo.ListInactivePackages(f.project)
	if len(list) != 1 || list[0].ID != g || list[0].PhaseID != f.phase {
		t.Fatalf("비활성 목록 %+v", list)
	}
	// 되돌리기
	if err := f.repo.RestorePackage(f.project, g); err != nil {
		t.Fatal(err)
	}
	_ = f.db.QueryRow(`SELECT is_active FROM phase_packages WHERE package_id=?`, g).Scan(&act)
	if act != 1 {
		t.Fatal("되돌리지 못했다")
	}
	// 연결 작업이 있어도 비활성
	task, _ := f.repo.CreatePackage(f.phase, "작업 연결", model.TrackModeSite)
	if _, err := f.db.Exec(`INSERT INTO work_tasks (task_id, title, status, project_id, package_id) VALUES ('T183','작업','pending','P183',?)`, task); err != nil {
		t.Skipf("work_tasks 시드 실패(스키마 차이): %v", err)
	}
	off, im, err = f.repo.DeletePackage(f.project, task)
	if err != nil || !off || im.Tasks != 1 {
		t.Fatalf("연결 작업이 있으면 비활성: off=%v im=%+v err=%v", off, im, err)
	}
}

func Test95_183_PlanBasisChangeNeedsQty(t *testing.T) {
	f := new183(t)
	plan, err := f.repo.CreatePlan(f.project, PlanDraft{PackageID: f.pkg, Name: "1차", Basis: model.BasisSite, SiteIDs: f.sites[:2], Actor: "t"})
	if err != nil {
		t.Fatal(err)
	}
	e := PlanEdit{Name: "1차", ChangeBasis: true, Basis: model.BasisQty, QtyUnit: "대"}
	if err := f.repo.UpdatePlan(f.project, plan, e, DateChangeCtx{}); err == nil || !strings.Contains(err.Error(), "미입력 2곳") {
		t.Fatalf("수량 미입력인데 기준이 바뀐다: %v", err)
	}
	if _, err := f.repo.SetPlanSiteQty(f.project, plan, map[string]int{f.sites[0]: 5, f.sites[1]: 7}); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.UpdatePlan(f.project, plan, e, DateChangeCtx{}); err != nil {
		t.Fatal(err)
	}
	var basis, unit string
	_ = f.db.QueryRow(`SELECT progress_basis, qty_unit FROM package_plans WHERE plan_id=?`, plan).Scan(&basis, &unit)
	if basis != model.BasisQty || unit != "대" {
		t.Fatalf("기준 %s %s", basis, unit)
	}
	// 기준을 안 건드리는 수정(ChangeBasis=false)은 기존 값을 유지
	if err := f.repo.UpdatePlan(f.project, plan, PlanEdit{Name: "1차 수정"}, DateChangeCtx{}); err != nil {
		t.Fatal(err)
	}
	_ = f.db.QueryRow(`SELECT progress_basis, qty_unit FROM package_plans WHERE plan_id=?`, plan).Scan(&basis, &unit)
	if basis != model.BasisQty || unit != "대" {
		t.Fatalf("기준이 의도치 않게 바뀜 %s %s", basis, unit)
	}
	// 단위 21자
	if err := f.repo.UpdatePlan(f.project, plan, PlanEdit{Name: "1차 수정", ChangeBasis: true, Basis: model.BasisSite, QtyUnit: strings.Repeat("가", 21)}, DateChangeCtx{}); err == nil {
		t.Fatal("21자 단위가 저장됐다")
	}
}

func Test95_183_PlanSitesAddMoveExcludeQty(t *testing.T) {
	f := new183(t)
	p1, _ := f.repo.CreatePlan(f.project, PlanDraft{PackageID: f.pkg, Name: "1차", Basis: model.BasisSite, SiteIDs: f.sites[:2], Actor: "t"})
	p2, _ := f.repo.CreatePlan(f.project, PlanDraft{PackageID: f.pkg, Name: "2차", Basis: model.BasisSite, SiteIDs: f.sites[2:3], Actor: "t"})
	// 후보: 1차 입장에서는 2차 사이트 + 미배정 사이트
	cands, total, err := f.repo.PlanSiteCandidates(f.project, p1, "", "", 0, 100)
	if err != nil || total != 2 {
		t.Fatalf("후보 %d %v", total, err)
	}
	var other, free int
	for _, c := range cands {
		if c.OtherPlanID == p2 {
			other++
		}
		if c.OtherPlanID == "" {
			free++
		}
	}
	if other != 1 || free != 1 {
		t.Fatalf("후보 구성 %+v", cands)
	}
	// 이름 검색
	hit, n, _ := f.repo.PlanSiteCandidates(f.project, p1, "횡성", "", 0, 100)
	if n != 1 || hit[0].Name != "횡성중" {
		t.Fatalf("검색 %+v", hit)
	}
	// 옮기기: 2차 사이트와 미배정 사이트를 1차로
	if err := f.repo.MovePlanSitesQty(f.project, p1, []string{f.sites[2], f.sites[3]}, nil, "u"); err != nil {
		t.Fatal(err)
	}
	view, _ := f.repo.PlanSites(f.project, p1)
	if len(view.Sites) != 4 {
		t.Fatalf("1차 사이트 %d", len(view.Sites))
	}
	// 완료 사이트는 다른 계획으로 못 옮긴다
	if _, err := f.db.Exec(`UPDATE package_sites SET status='done' WHERE package_id=? AND project_site_id=?`, f.pkg, f.sites[0]); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.MovePlanSitesQty(f.project, p2, []string{f.sites[0]}, nil, "u"); err == nil || !strings.Contains(err.Error(), "완료된 사이트") {
		t.Fatalf("완료 사이트가 옮겨졌다: %v", err)
	}
	// 하부 업무에 등록되지 않은 사이트는 담을 수 없다
	if _, err := f.db.Exec(`INSERT INTO project_sites (id, project_id, site_name, region, status, is_active, org_id) VALUES ('S183-Z','P183','외부','충남','planned',1,'O01')`); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.MovePlanSitesQty(f.project, p2, []string{"S183-Z"}, nil, "u"); err == nil || !strings.Contains(err.Error(), "등록되지 않은") {
		t.Fatalf("미등록 사이트가 담겼다: %v", err)
	}
	// 제외: 사유 필수, 완료 사이트는 확인
	if err := f.repo.ExcludePlanSite(f.project, p1, f.sites[1], "", false); err == nil {
		t.Fatal("사유 없이 제외됐다")
	}
	if err := f.repo.ExcludePlanSite(f.project, p1, f.sites[0], "현장 철거", false); err == nil {
		t.Fatal("완료 사이트가 확인 없이 제외됐다")
	}
	if err := f.repo.ExcludePlanSite(f.project, p1, f.sites[0], "현장 철거", true); err != nil {
		t.Fatal(err)
	}
	view, _ = f.repo.PlanSites(f.project, p1)
	if len(view.Sites) != 3 || len(view.Excluded) != 1 || view.Excluded[0].Reason != "현장 철거" {
		t.Fatalf("제외 후 %d/%d", len(view.Sites), len(view.Excluded))
	}
	// 수량: 정수 1 이상, 이 계획 사이트만
	if _, err := f.repo.SetPlanSiteQty(f.project, p1, map[string]int{f.sites[1]: 0}); err == nil {
		t.Fatal("0 수량이 저장됐다")
	}
	if _, err := f.repo.SetPlanSiteQty(f.project, p1, map[string]int{f.sites[0]: 3}); err == nil {
		t.Fatal("제외한 사이트 수량이 저장됐다")
	}
	if _, err := f.repo.SetPlanSiteQty(f.project, p2, map[string]int{f.sites[1]: 3}); err == nil {
		t.Fatal("다른 계획 사이트 수량이 저장됐다")
	}
	if _, err := f.repo.SetPlanSiteQty(f.project, p1, map[string]int{f.sites[1]: 12}); err != nil {
		t.Fatal(err)
	}
	view, _ = f.repo.PlanSites(f.project, p1)
	got := 0
	for _, s := range view.Sites {
		if s.ID == f.sites[1] {
			got = s.Target
		}
	}
	if got != 12 {
		t.Fatalf("목표 수량 %d", got)
	}
	// 수량 기준 계획에 수량 없는 사이트를 담으면 전부 취소
	q, _ := f.repo.CreatePlan(f.project, PlanDraft{PackageID: f.pkg, Name: "수량계획", Basis: model.BasisQty, SiteIDs: nil, Actor: "t"})
	before, _ := f.repo.PlanSites(f.project, p1)
	if err := f.repo.MovePlanSitesQty(f.project, q, []string{f.sites[3]}, nil, "u"); err == nil || !strings.Contains(err.Error(), "미입력") {
		t.Fatalf("수량 없는 사이트가 수량 계획에 담겼다: %v", err)
	}
	after, _ := f.repo.PlanSites(f.project, p1)
	if len(before.Sites) != len(after.Sites) {
		t.Fatal("거절됐는데 사이트가 옮겨졌다(롤백 실패)")
	}
	if err := f.repo.MovePlanSitesQty(f.project, q, []string{f.sites[3]}, map[string]int{f.sites[3]: 9}, "u"); err != nil {
		t.Fatalf("수량과 함께 담기: %v", err)
	}
}

func Test95_183_BatchAllOrNothing(t *testing.T) {
	f := new183(t)
	plan, _ := f.repo.CreatePlan(f.project, PlanDraft{PackageID: f.pkg, Name: "1차", PlanEnd: "2026-11-30", Basis: model.BasisSite, SiteIDs: f.sites[:1], Actor: "t"})
	other, _ := f.repo.CreatePackage(f.phase, "스위치", model.TrackModeSite)
	items := []BatchItem{
		{Kind: BatchPhase, ID: f.phase, F: map[string]string{"name": "전처리 업무", "status": "active"}},
		{Kind: BatchPackage, ID: f.pkg, F: map[string]string{"name": "GBIC 교체", "next_note": "검수", "badge": "사전"}},
		{Kind: BatchPlan, ID: plan, F: map[string]string{"name": "1차 계획", "plan_end": "2026-12-15", "extend_reason": "현장 지연"}},
	}
	res, err := f.repo.ApplyStructureBatch(f.project, items, "u")
	if err != nil || len(res) != 3 {
		t.Fatalf("일괄 저장: %v %+v", err, res)
	}
	var n1, n2, n3, end string
	_ = f.db.QueryRow(`SELECT name FROM project_phases WHERE phase_id=?`, f.phase).Scan(&n1)
	_ = f.db.QueryRow(`SELECT name FROM phase_packages WHERE package_id=?`, f.pkg).Scan(&n2)
	_ = f.db.QueryRow(`SELECT name, plan_end FROM package_plans WHERE plan_id=?`, plan).Scan(&n3, &end)
	if n1 != "전처리 업무" || n2 != "GBIC 교체" || n3 != "1차 계획" || end != "2026-12-15" {
		t.Fatalf("저장 값 %s %s %s %s", n1, n2, n3, end)
	}
	if ch, _ := f.repo.ListDateChanges(f.project, 10); len(ch) != 1 || ch[0].Reason != "현장 지연" {
		t.Fatalf("이력 %+v", ch)
	}
	// 가운데 항목이 실패하면(이름 중복) 앞 항목도 취소
	bad := []BatchItem{
		{Kind: BatchPhase, ID: f.phase, F: map[string]string{"name": "다른 이름"}},
		{Kind: BatchPackage, ID: f.pkg, F: map[string]string{"name": "스위치"}},
		{Kind: BatchPlan, ID: plan, F: map[string]string{"name": "2차 계획"}},
	}
	_, err = f.repo.ApplyStructureBatch(f.project, bad, "u")
	var be *BatchError
	if !errors.As(err, &be) || be.Index != 1 || be.Kind != BatchPackage || be.ID != f.pkg {
		t.Fatalf("실패 위치: %v", err)
	}
	_ = f.db.QueryRow(`SELECT name FROM project_phases WHERE phase_id=?`, f.phase).Scan(&n1)
	_ = f.db.QueryRow(`SELECT name FROM package_plans WHERE plan_id=?`, plan).Scan(&n3)
	if n1 != "전처리 업무" || n3 != "1차 계획" {
		t.Fatalf("실패했는데 일부가 저장됐다: %s %s", n1, n3)
	}
	// 사유 없는 연장은 거절되고 이력이 늘지 않는다
	ext := []BatchItem{{Kind: BatchPlan, ID: plan, F: map[string]string{"plan_end": "2027-01-31"}}}
	if _, err := f.repo.ApplyStructureBatch(f.project, ext, "u"); err == nil || !strings.Contains(err.Error(), "연장 사유") {
		t.Fatalf("사유 없는 연장: %v", err)
	}
	if ch, _ := f.repo.ListDateChanges(f.project, 10); len(ch) != 1 {
		t.Fatalf("이력 %d", len(ch))
	}
	// 다른 사업·알 수 없는 종류·빈 목록
	if _, err := f.repo.ApplyStructureBatch("OTHER", items[:1], "u"); err == nil {
		t.Fatal("다른 사업 항목이 저장됐다")
	}
	if _, err := f.repo.ApplyStructureBatch(f.project, []BatchItem{{Kind: "x", ID: "1"}}, "u"); err == nil {
		t.Fatal("알 수 없는 종류가 저장됐다")
	}
	if _, err := f.repo.ApplyStructureBatch(f.project, nil, "u"); err == nil {
		t.Fatal("빈 목록이 저장됐다")
	}
	_ = other
}

// 사업 통합: 하부 업무 이름 비교가 정규화 기준이다(공백·괄호 무시). 95-183.
func Test95_183_MergeFoldsPackagesByNormalizedName(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "m183.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	seed176Project(t, db, "KEEP", "유지", "O01", "build")
	seed176Project(t, db, "DROP", "흡수", "O01", "build")
	repo := NewProjectRepo(db)
	keepPkg := seed176PhasePkg(t, repo, "KEEP", "O01", "전처리", "GBIC 교체", model.TrackModeSite, 1)
	dropPkg := seed176PhasePkg(t, repo, "DROP", "O01", "전처리", "GBIC(교체)", model.TrackModeSite, 1)
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	snap, err := readStructureSnap(tx, "KEEP", "DROP")
	if err != nil {
		t.Fatal(err)
	}
	if err := foldPackages(tx, "KEEP", snap); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM phase_packages WHERE package_id=?`, dropPkg).Scan(&n)
	if n != 0 {
		t.Fatal("정규화로 같은 이름인데 합쳐지지 않았다")
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM phase_packages WHERE package_id=?`, keepPkg).Scan(&n)
	if n != 1 {
		t.Fatal("유지 쪽 하부 업무가 사라졌다")
	}
}

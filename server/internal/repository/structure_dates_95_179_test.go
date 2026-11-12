package repository

import (
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func datesFixture(t *testing.T) (*ProjectRepo, string, string, string) {
	t.Helper()
	db, err := InitDB(filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewProjectRepo(db)
	for _, id := range []string{"WPD", "WPX"} {
		if _, err := db.Exec(`INSERT INTO work_projects (project_id, name, status, org_id) VALUES (?,?, 'active','O01')`, id, id); err != nil {
			t.Fatal(err)
		}
	}
	ph := &model.ProjectPhase{ProjectID: "WPD", OrgID: "O01", Name: "전처리 업무", Status: model.PhaseActive, PlanEnd: "2026-12-01"}
	if err := repo.CreatePhase(ph); err != nil {
		t.Fatal(err)
	}
	pk, err := repo.CreatePackage(ph.PhaseID, "GBIC 수량 산정", model.TrackModeSite)
	if err != nil {
		t.Fatal(err)
	}
	return repo, "WPD", ph.PhaseID, pk
}

func TestUpdatePackageMetaExtendRequiresReasonAndRecords(t *testing.T) {
	repo, pid, _, pkg := datesFixture(t)
	actor := DateChangeCtx{Actor: "u-admin"}
	// 처음 정하는 것은 이력에 남기지 않고 사유도 필요 없다. 1단계 마감(12-01)을 넘는 하부 업무 마감도 허용한다.
	if err := repo.UpdatePackageMeta(pid, pkg, PackageMeta{PlanStart: "2026-11-01", PlanEnd: "2026-12-20", Badge: "사전GB"}, actor); err != nil {
		t.Fatalf("처음 정함: %v", err)
	}
	if ch, _ := repo.ListDateChanges(pid, 10); len(ch) != 0 {
		t.Fatalf("처음 정한 것이 이력에 남았다: %+v", ch)
	}
	// 연장: 사유 없으면 거절하고 값이 바뀌지 않는다
	err := repo.UpdatePackageMeta(pid, pkg, PackageMeta{PlanStart: "2026-11-01", PlanEnd: "2027-01-10", Badge: "사전GB"}, actor)
	if err == nil || !strings.Contains(err.Error(), "연장 사유") {
		t.Fatalf("사유 없는 연장: %v", err)
	}
	if m, _, _ := repo.PackageMetaOf(pid, pkg); m.PlanEnd != "2026-12-20" {
		t.Fatalf("거절됐는데 바뀜: %+v", m)
	}
	// 연장 + 사유: 이력(이전→이후·사유·ID·시각)
	if err := repo.UpdatePackageMeta(pid, pkg, PackageMeta{PlanStart: "2026-11-01", PlanEnd: "2027-01-10", Badge: "사전GB"}, DateChangeCtx{Actor: "u-admin", Reason: "발주처 일정 변경"}); err != nil {
		t.Fatal(err)
	}
	// 앞당김: 사유 없이도 되고 이력은 남는다
	if err := repo.UpdatePackageMeta(pid, pkg, PackageMeta{PlanStart: "2026-11-01", PlanEnd: "2026-12-31", Badge: "사전GB"}, actor); err != nil {
		t.Fatalf("앞당김: %v", err)
	}
	// 마감 해제도 이력만
	if err := repo.UpdatePackageMeta(pid, pkg, PackageMeta{PlanStart: "2026-11-01", PlanEnd: "", Badge: "사전GB"}, actor); err != nil {
		t.Fatalf("해제: %v", err)
	}
	ch, err := repo.ListDateChanges(pid, 10)
	if err != nil || len(ch) != 3 {
		t.Fatalf("이력 %d건 err=%v: %+v", len(ch), err, ch)
	}
	got := map[string]DateChange{}
	for _, c := range ch {
		got[c.Kind] = c
	}
	ext := got[model.EndChangeExtend]
	if ext.OldValue != "2026-12-20" || ext.NewValue != "2027-01-10" || ext.Reason != "발주처 일정 변경" || ext.ChangedBy != "u-admin" ||
		ext.Level != "package" || ext.TargetName != "GBIC 수량 산정" || !strings.HasPrefix(ext.ID, "DC-") || len(ext.ChangedAt) < 16 {
		t.Fatalf("연장 이력 %+v", ext)
	}
	if got[model.EndChangeShorten].Reason != "" || got[model.EndChangeClear].NewValue != "" {
		t.Fatalf("앞당김·해제 이력 %+v", got)
	}
	// 최근 순
	if ch[0].Kind != model.EndChangeClear {
		t.Fatalf("최근 순이 아니다: %s", ch[0].Kind)
	}
}

func TestUpdatePackageMetaValidationAndScope(t *testing.T) {
	repo, pid, phase, pkg := datesFixture(t)
	if err := repo.UpdatePackageMeta(pid, pkg, PackageMeta{Badge: "아홉글자배지이름임"}, DateChangeCtx{}); err == nil {
		t.Fatal("8자 초과 배지는 거절")
	}
	if err := repo.UpdatePackageMeta(pid, pkg, PackageMeta{PlanStart: "2026-12-10", PlanEnd: "2026-12-01"}, DateChangeCtx{}); err == nil {
		t.Fatal("시작이 마감보다 늦으면 거절")
	}
	if err := repo.UpdatePackageMeta("WPX", pkg, PackageMeta{PlanEnd: "2026-12-01"}, DateChangeCtx{}); err == nil || !strings.Contains(err.Error(), "찾을 수 없습니다") {
		t.Fatalf("다른 사업은 거절: %v", err)
	}
	// 배지는 중복 허용: 같은 배지를 두 하부 업무가 가질 수 있다
	other, err := repo.CreatePackage(phase, "방화벽/L2 SW 수량 산정", model.TrackModeSite)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{pkg, other} {
		if err := repo.UpdatePackageMeta(pid, id, PackageMeta{Badge: "사전"}, DateChangeCtx{}); err != nil {
			t.Fatalf("중복 배지: %v", err)
		}
	}
	tree, err := repo.LoadStructure(pid, "2026-11-12", model.WeekendBiz)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, pk := range tree.Phases[0].Packages {
		if pk.Badge == "사전" && pk.BadgeTone == model.BadgeTone("사전") {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("트리에 배지가 안 실렸다: %d", n)
	}
}

func TestUpdatePhaseExtExtensionNeedsReason(t *testing.T) {
	repo, pid, phaseID, _ := datesFixture(t)
	cur, err := repo.GetPhase(phaseID)
	if err != nil || cur == nil {
		t.Fatal(err)
	}
	cur.PlanEnd = "2026-12-31"
	if err := repo.UpdatePhaseExt(cur, &DateChangeCtx{Actor: "u1"}); err == nil || !strings.Contains(err.Error(), "연장 사유") {
		t.Fatalf("사유 없는 연장: %v", err)
	}
	cur, _ = repo.GetPhase(phaseID)
	if cur.PlanEnd != "2026-12-01" {
		t.Fatalf("거절됐는데 바뀜: %s", cur.PlanEnd)
	}
	cur.PlanEnd = "2026-12-31"
	if err := repo.UpdatePhaseExt(cur, &DateChangeCtx{Actor: "u1", Reason: "검수 지연"}); err != nil {
		t.Fatal(err)
	}
	ch, _ := repo.ListDateChanges(pid, 10)
	if len(ch) != 1 || ch[0].Level != "phase" || ch[0].Reason != "검수 지연" || ch[0].OldValue != "2026-12-01" {
		t.Fatalf("단계 연장 이력 %+v", ch)
	}
	// 내부 경로(UpdatePhase)는 이력·사유를 건너뛴다(통합·복원)
	cur, _ = repo.GetPhase(phaseID)
	cur.PlanEnd = "2027-02-01"
	if err := repo.UpdatePhase(cur); err != nil {
		t.Fatalf("내부 경로: %v", err)
	}
	if ch, _ = repo.ListDateChanges(pid, 10); len(ch) != 1 {
		t.Fatalf("내부 경로가 이력을 남겼다: %d", len(ch))
	}
}

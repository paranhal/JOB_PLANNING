package repository

import (
	"testing"

	"customer-support/internal/model"
)

// 실제 시작일: 수동 입력값이 있으면 그 값, 없으면 연결 작업의 첫 완료일(자동). 1단계 업무는 하부 업무 시작일 중 가장 이른 날. 95-179C.
func TestActualStartAutoFromCompletedTasks(t *testing.T) {
	repo, pid, phaseID, pkgA := datesFixture(t)
	pkgB, err := repo.CreatePackage(phaseID, "방화벽/L2 SW 수량 산정", model.TrackModeSite)
	if err != nil {
		t.Fatal(err)
	}
	task := func(id, pkg, status, done string) {
		t.Helper()
		if _, err := repo.db.Exec(`INSERT INTO work_tasks(task_id, work_type, title, status, project_id, org_id, package_id, complete_date, work_date)
			VALUES (?, 'project_work', ?, ?, ?, 'O01', ?, ?, '2026-11-01')`, id, id, status, pid, pkg, done); err != nil {
			t.Fatal(err)
		}
	}
	get := func() (PkgNode, PkgNode, EffStart) {
		t.Helper()
		tree, err := repo.LoadStructure(pid, "2026-11-12", model.WeekendBiz)
		if err != nil {
			t.Fatal(err)
		}
		var a, b PkgNode
		for _, pk := range tree.Phases[0].Packages {
			switch pk.ID {
			case pkgA:
				a = pk
			case pkgB:
				b = pk
			}
		}
		return a, b, tree.PhaseStart[phaseID]
	}

	// 작업이 없으면 시작일도 없다
	if a, b, ph := get(); a.ActualStart != "" || b.ActualStart != "" || ph.Value != "" {
		t.Fatalf("작업 없음: %+v %+v %+v", a, b, ph)
	}
	// 완료되지 않은 작업은 세지 않는다(완료일 칸이 채워져 있어도)
	task("W0", pkgA, "waiting", "2026-10-01")
	if a, _, _ := get(); a.ActualStart != "" {
		t.Fatalf("미완료 작업이 시작일이 됐다: %+v", a)
	}
	task("W1", pkgA, "complete", "2026-11-10")
	task("W2", pkgA, "complete", "2026-11-05") // 더 이른 완료일
	task("W3", pkgB, "complete", "2026-11-08")
	a, b, ph := get()
	if a.ActualStart != "2026-11-05" || !a.ActualAuto || b.ActualStart != "2026-11-08" || !b.ActualAuto {
		t.Fatalf("하부 업무 자동 시작일: %+v / %+v", a, b)
	}
	if ph.Value != "2026-11-05" || !ph.Auto {
		t.Fatalf("1단계 업무 = 하부 업무 중 가장 이른 날: %+v", ph)
	}
	// 수동 입력값이 우선한다(자동 아님)
	if err := repo.UpdatePackageMeta(pid, pkgA, PackageMeta{ActualStart: "2026-11-01"}, DateChangeCtx{Actor: "u1"}); err != nil {
		t.Fatal(err)
	}
	a, _, ph = get()
	if a.ActualStart != "2026-11-01" || a.ActualAuto || ph.Value != "2026-11-01" {
		t.Fatalf("수동 우선: %+v %+v", a, ph)
	}
	// 수동값을 비우면 다시 자동
	if err := repo.UpdatePackageMeta(pid, pkgA, PackageMeta{}, DateChangeCtx{Actor: "u1"}); err != nil {
		t.Fatal(err)
	}
	if a, _, _ = get(); a.ActualStart != "2026-11-05" || !a.ActualAuto {
		t.Fatalf("비우면 자동: %+v", a)
	}
	// 1단계 업무의 수동값은 하부 업무와 무관하게 우선(자동 아님)
	cur, _ := repo.GetPhase(phaseID)
	cur.ActualStart = "2026-10-20"
	if err := repo.UpdatePhase(cur); err != nil {
		t.Fatal(err)
	}
	if _, _, ph = get(); ph.Value != "2026-10-20" || ph.Auto {
		t.Fatalf("단계 수동 우선: %+v", ph)
	}
}

package repository

import (
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func qtyFixture(t *testing.T) (*ProjectRepo, *WBRepo, string, string, string) {
	t.Helper()
	db, err := InitDB(filepath.Join(t.TempDir(), "q.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewProjectRepo(db)
	wb := NewWBRepo(db)
	if _, err := db.Exec(`INSERT INTO work_projects (project_id, name, status, org_id) VALUES ('WPQ','수량사업','active','O01')`); err != nil {
		t.Fatal(err)
	}
	ph := &model.ProjectPhase{ProjectID: "WPQ", OrgID: "O01", Name: "전처리 업무", Status: model.PhaseActive}
	if err := repo.CreatePhase(ph); err != nil {
		t.Fatal(err)
	}
	qty, err := repo.CreatePackage(ph.PhaseID, "환경조사", model.TrackModePhase, 1)
	if err != nil {
		t.Fatal(err)
	}
	site, err := repo.CreatePackage(ph.PhaseID, "현장작업", model.TrackModeSite, 1)
	if err != nil {
		t.Fatal(err)
	}
	return repo, wb, "WPQ", qty, site
}

func qtyNode(t *testing.T, repo *ProjectRepo, project, pkg string) PkgNode {
	t.Helper()
	tree, err := repo.LoadStructure(project, "2026-11-12", model.WeekendBiz)
	if err != nil {
		t.Fatal(err)
	}
	for _, ph := range tree.Phases {
		for _, pk := range ph.Packages {
			if pk.ID == pkg {
				return pk
			}
		}
	}
	t.Fatalf("하부 업무 %s 없음", pkg)
	return PkgNode{}
}

func TestPackageQtyTargetRecordAndLimits(t *testing.T) {
	repo, _, pid, qty, site := qtyFixture(t)

	if _, err := repo.RecordPackageQty(pid, qty, "2026-11-12", 10, "u1", ""); err == nil || !strings.Contains(err.Error(), "목표 수량을 먼저") {
		t.Fatalf("목표 전 입력은 거절: %v", err)
	}
	n := qtyNode(t, repo, pid, qty)
	if !n.QtyMode || n.QtyLabel != "목표 수량 미설정" || n.Display != "예정" {
		t.Fatalf("미설정 표시 %+v", n)
	}
	if err := repo.SetPackageQtyTarget(pid, qty, "식", 0); err == nil {
		t.Fatal("목표 0 은 거절")
	}
	if err := repo.SetPackageQtyTarget(pid, qty, "가나다라마바사아자차카타파하가나다라마바사", 10); err == nil {
		t.Fatal("단위 20자 초과는 거절")
	}
	if err := repo.SetPackageQtyTarget(pid, qty, "식", 120); err != nil {
		t.Fatal(err)
	}
	done, err := repo.RecordPackageQty(pid, qty, "2026-11-12", 30, "u1", "홍천")
	if err != nil || done != 30 {
		t.Fatalf("30 입력 done=%d err=%v", done, err)
	}
	n = qtyNode(t, repo, pid, qty)
	if n.QtyLabel != "30/120식 (25%)" || n.Display != "진행 중" || n.Progress != "30/120식 (25%)" || n.QtyDone != 30 || n.QtyTotal != 120 {
		t.Fatalf("진행 표시 %+v", n)
	}
	// 한도: 목표 초과·0 미만·0 입력·잘못된 날짜
	for _, c := range []struct {
		delta int
		date  string
		want  string
	}{{91, "2026-11-12", "넘습니다"}, {-31, "2026-11-12", "0보다 작아집니다"}, {0, "2026-11-12", "0은 저장하지"}, {5, "2026/11/12", "YYYY-MM-DD"}} {
		if _, err := repo.RecordPackageQty(pid, qty, c.date, c.delta, "u1", ""); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("%+v → %v", c, err)
		}
	}
	// 정정(음수)과 목표 변경: 누적보다 작게는 못 줄이고, 늘리는 것은 된다
	if done, err = repo.RecordPackageQty(pid, qty, "2026-11-12", -10, "u1", "정정"); err != nil || done != 20 {
		t.Fatalf("정정 done=%d err=%v", done, err)
	}
	if err := repo.SetPackageQtyTarget(pid, qty, "식", 19); err == nil || !strings.Contains(err.Error(), "줄일 수 없습니다") {
		t.Fatalf("누적보다 작은 목표: %v", err)
	}
	if err := repo.SetPackageQtyTarget(pid, qty, "대", 200); err != nil {
		t.Fatal(err)
	}
	if n = qtyNode(t, repo, pid, qty); n.QtyLabel != "20/200대 (10%)" {
		t.Fatalf("목표 변경 뒤 %q", n.QtyLabel)
	}
	// 전부 채우면 완료 후보
	if _, err = repo.RecordPackageQty(pid, qty, "2026-11-13", 180, "u1", ""); err != nil {
		t.Fatal(err)
	}
	if n = qtyNode(t, repo, pid, qty); n.Display != "완료 후보" || n.QtyLabel != "200/200대 (100%)" {
		t.Fatalf("완료 후보 %+v", n)
	}
	// 사이트 하부 업무·다른 사업의 하부 업무에는 입력할 수 없다
	if _, err := repo.RecordPackageQty(pid, site, "2026-11-12", 1, "u1", ""); err == nil || !strings.Contains(err.Error(), "수량만 하부 업무에서만") {
		t.Fatalf("사이트 하부 업무: %v", err)
	}
	if err := repo.SetPackageQtyTarget("WP-OTHER", qty, "식", 10); err == nil || !strings.Contains(err.Error(), "찾을 수 없습니다") {
		t.Fatalf("다른 사업: %v", err)
	}
	if _, err := repo.RecordPackageQty("WP-OTHER", qty, "2026-11-12", 1, "u1", ""); err == nil {
		t.Fatal("다른 사업은 거절")
	}
}

// 수량 이벤트는 사이트 스냅샷 재계산에 섞이지 않고, 사이트 이벤트 조회에도 나오지 않는다.
func TestPackageQtyEventsStayOutOfSiteSnapshots(t *testing.T) {
	repo, wb, pid, qty, _ := qtyFixture(t)
	if err := repo.SetPackageQtyTarget(pid, qty, "식", 100); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.RecordPackageQty(pid, qty, "2026-11-12", 40, "u1", ""); err != nil {
		t.Fatal(err)
	}
	if err := wb.RebuildPackageProgress(qty); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = repo.db.QueryRow(`SELECT COUNT(*) FROM package_progress_daily WHERE package_id=?`, qty).Scan(&n)
	if n != 0 {
		t.Fatalf("수량 이벤트가 스냅샷을 만들었다: %d", n)
	}
	evs, err := repo.eventsFor([]string{pid}, "2026-01-01")
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 0 {
		t.Fatalf("사이트 이벤트 조회에 수량 이벤트가 섞였다: %d", len(evs))
	}
	var ev, id string
	_ = repo.db.QueryRow(`SELECT event_type, event_id FROM package_events WHERE package_id=?`, qty).Scan(&ev, &id)
	if ev != "qty" || !strings.HasPrefix(id, "EV-") {
		t.Fatalf("이벤트 %s %s", ev, id)
	}
}

// 주간 보고·회의가 쓰는 ProgressLines 에 수량만 하부 업무 문장이 나온다.
func TestProgressLinesIncludeQtyOnlyPackage(t *testing.T) {
	repo, _, pid, qty, _ := qtyFixture(t)
	if err := repo.SetPackageQtyTarget(pid, qty, "식", 120); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.RecordPackageQty(pid, qty, "2026-11-12", 30, "u1", ""); err != nil {
		t.Fatal(err)
	}
	lines, err := repo.ProgressLines([]string{pid}, "2026-11-09", "2026-11-15", "2026-11-12", model.WeekendBiz)
	if err != nil {
		t.Fatal(err)
	}
	text := lines[pid].Text
	if !strings.Contains(text, "환경조사는 120식 중 30식 완료(25%)했습니다.") {
		t.Fatalf("보고 문장 %q", text)
	}
	// 구조 화면의 진행 보고(LoadStructurePeriod)도 같은 문장을 쓴다
	tree, err := repo.LoadStructurePeriod(pid, "2026-11-12", "week", model.WeekendBiz)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(model.BuildProjectProgressReport(tree.Report), " ")
	if !strings.Contains(got, "환경조사는 120식 중 30식 완료(25%)했습니다.") {
		t.Fatalf("구조 화면 보고 %q", got)
	}
}

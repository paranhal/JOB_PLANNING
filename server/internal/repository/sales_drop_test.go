package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestSalesDropAndClosedFilter(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "s44f.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewSalesRepo(db)

	p := &model.SalesProject{Name: "드롭 발굴"}
	_ = repo.Create(p)
	if err := repo.Drop(p.SalesID, "budget", "", "u1", "테스터"); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.Get(p.SalesID)
	if got.CloseReason != model.SalesCloseDropped || got.DroppedFromStage != model.SalesStage4Discover {
		t.Fatalf("드롭 %+v from=%s", got.CloseReason, got.DroppedFromStage)
	}

	pr := &model.SalesProject{Name: "드롭 제안"}
	_ = repo.Create(pr)
	_ = repo.ChangeStage(pr.SalesID, model.SalesStage4Propose, "", "u1", "t", false)
	_ = repo.Drop(pr.SalesID, "postponed", "", "u1", "t")
	pg, _ := repo.Get(pr.SalesID)
	if pg.DroppedFromStage != model.SalesStage4Propose {
		t.Fatalf("제안 드롭 from=%s", pg.DroppedFromStage)
	}

	bid := &model.SalesProject{Name: "드롭 입찰", BidEvalMethod: "lowest"}
	_ = repo.Create(bid)
	_ = repo.ChangeStage(bid.SalesID, model.SalesStage4Propose, "", "u1", "t", false)
	_ = repo.ChangeStage(bid.SalesID, model.SalesStage4Bid, "", "u1", "t", false)
	if err := repo.Drop(bid.SalesID, "etc", "중복", "u1", "t"); err != nil {
		t.Fatal(err)
	}

	won := &model.SalesProject{Name: "수주 후 드롭"}
	_ = repo.Create(won)
	_ = repo.ChangeStage(won.SalesID, model.SalesDirectWin, "", "u1", "t", false)
	if err := repo.Drop(won.SalesID, "budget", "", "u1", "t"); err == nil {
		t.Fatal("수주 뒤 드롭이 통과했다")
	}
	if err := repo.Drop(p.SalesID, "", "", "u1", "t"); err == nil {
		t.Fatal("사유 없이 드롭")
	}

	vis, _ := repo.ListFilter(SalesListFilter{})
	for _, it := range vis {
		if it.CloseReason == model.SalesCloseDropped {
			t.Fatal("기본 목록에 포기가 있다")
		}
	}
	all, _ := repo.ListFilter(SalesListFilter{IncludeClosed: true, CloseReason: model.SalesCloseDropped})
	if len(all) < 3 {
		t.Fatalf("포기 보기 n=%d", len(all))
	}
}

func TestSalesDropCancelsOpenTasks(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "s44f2.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewSalesRepo(db)
	p := &model.SalesProject{Name: "업무 취소"}
	_ = repo.Create(p)
	a1 := &model.SalesActivity{SalesID: p.SalesID, ActivityDate: "2026-09-01", ActivityType: "call", Title: "콜"}
	_ = repo.CreateActivity(a1, "t", nil)
	if err := repo.CreateNextActionTask(a1.ActivityID, "2026-09-10", "테스터", "u1"); err != nil {
		t.Log(err)
	}
	if err := repo.Drop(p.SalesID, "resource", "", "u1", "t"); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM work_tasks WHERE source_type=? AND status=?`,
		model.WBSourceSalesActivity, model.WBTaskCancelled).Scan(&n)
	if n < 1 {
		t.Fatalf("취소된 업무=%d", n)
	}
}

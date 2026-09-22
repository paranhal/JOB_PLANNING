package repository

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestSalesChangeStageAllowDenyAndFreeze(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "s44d.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewSalesRepo(db)

	p := &model.SalesProject{Name: "규칙 사업", BidEvalMethod: "lowest"}
	if err := repo.Create(p); err != nil {
		t.Fatal(err)
	}
	if p.Stage != model.SalesStage4Discover {
		t.Fatalf("등록 단계=%s", p.Stage)
	}
	if err := repo.ChangeStage(p.SalesID, model.SalesStage4Propose, "", "u1", "테스터", false); err != nil {
		t.Fatal(err)
	}
	if err := repo.ChangeStage(p.SalesID, model.SalesStage4Bid, "", "u1", "테스터", false); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.Get(p.SalesID)
	if got.BidStatus != model.SalesBidPending || got.ProbabilityFinal == nil || *got.ProbabilityFinal != 50 {
		t.Fatalf("입찰 동결 %+v fin=%v", got.BidStatus, got.ProbabilityFinal)
	}
	if err := repo.SetWinProb(p.SalesID, 40, "u1", "테스터"); err == nil {
		t.Fatal("입찰 후 확도 변경이 통과했다")
	}
	if err := repo.ChangeStage(p.SalesID, model.SalesStage4Propose, "", "u1", "테스터", false); err == nil {
		t.Fatal("사유 없이 되돌렸다")
	}
	if err := repo.ChangeStage(p.SalesID, model.SalesStage4Propose, "재검토", "u1", "테스터", false); err != nil {
		t.Fatal(err)
	}
	back, _ := repo.Get(p.SalesID)
	if back.ProbabilityFinal != nil || back.BidStatus != "" {
		t.Fatal("되돌린 뒤 동결이 남았다")
	}

	noEval := &model.SalesProject{Name: "평가 없음"}
	if err := repo.Create(noEval); err != nil {
		t.Fatal(err)
	}
	_ = repo.ChangeStage(noEval.SalesID, model.SalesStage4Propose, "", "u1", "테스터", false)
	if err := repo.ChangeStage(noEval.SalesID, model.SalesStage4Bid, "", "u1", "테스터", false); err == nil {
		t.Fatal("평가방법 없이 입찰로 갔다")
	}
	priv := &model.SalesProject{Name: "민간", ProcurementRoute: "private"}
	if err := repo.Create(priv); err != nil {
		t.Fatal(err)
	}
	_ = repo.ChangeStage(priv.SalesID, model.SalesStage4Propose, "", "u1", "테스터", false)
	if err := repo.ChangeStage(priv.SalesID, model.SalesStage4Bid, "", "u1", "테스터", false); err != nil {
		t.Fatal(err)
	}

	direct := &model.SalesProject{Name: "바로 수주"}
	if err := repo.Create(direct); err != nil {
		t.Fatal(err)
	}
	if err := repo.ChangeStage(direct.SalesID, model.SalesDirectWin, "", "u1", "테스터", false); err != nil {
		t.Fatal(err)
	}
	dw, _ := repo.Get(direct.SalesID)
	if dw.Stage != model.SalesStage4Bid || dw.BidStatus != model.SalesBidWon {
		t.Fatalf("바로 수주 %+v %s", dw.Stage, dw.BidStatus)
	}

	closed := &model.SalesProject{Name: "종료 거부"}
	if err := repo.Create(closed); err != nil {
		t.Fatal(err)
	}
	if err := repo.ChangeStage(closed.SalesID, model.SalesStage4Closed, "끝", "u1", "테스터", false); err == nil {
		t.Fatal("ChangeStage 로 종료됐다")
	}
}

func TestSalesBidResultClosePromoteAndWinRate(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "s44d2.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewSalesRepo(db)

	p := &model.SalesProject{Name: "입찰 결과", BidEvalMethod: "lowest"}
	_ = repo.Create(p)
	_ = repo.ChangeStage(p.SalesID, model.SalesStage4Propose, "", "u1", "t", false)
	_ = repo.ChangeStage(p.SalesID, model.SalesStage4Bid, "", "u1", "t", false)
	if err := repo.SetBidResult(p.SalesID, "won", "", "1200000", "u1", "t"); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.Get(p.SalesID)
	if got.BidStatus != model.SalesBidWon || got.AwardedAmount != 1200000 {
		t.Fatalf("수주 %+v %d", got.BidStatus, got.AwardedAmount)
	}
	if err := repo.CloseContracted(p.SalesID, "2026-09-01", 1500000, "u1", "t"); err != nil {
		t.Fatal(err)
	}
	done, _ := repo.Get(p.SalesID)
	if !model.CanPromoteSales(done) || done.DealType == model.SalesDealSupply && !model.CanPromoteSales(done) {
		t.Fatal("계약 종료 후 승격 불가")
	}
	supply := &model.SalesProject{Name: "단품 계약", DealType: model.SalesDealSupply}
	_ = repo.Create(supply)
	_ = repo.ChangeStage(supply.SalesID, model.SalesDirectWin, "", "u1", "t", false)
	_ = repo.CloseContracted(supply.SalesID, "2026-09-02", 100, "u1", "t")
	sg, _ := repo.Get(supply.SalesID)
	if !model.CanPromoteSales(sg) {
		t.Fatal("단품도 승격돼야 한다")
	}

	fail := &model.SalesProject{Name: "결렬", BidEvalMethod: "lowest"}
	_ = repo.Create(fail)
	_ = repo.ChangeStage(fail.SalesID, model.SalesDirectWin, "", "u1", "t", false)
	if err := repo.CloseNegotiationFailed(fail.SalesID, "단가", "u1", "t"); err != nil {
		t.Fatal(err)
	}
	fg, _ := repo.Get(fail.SalesID)
	if fg.CloseReason != model.SalesCloseLost || fg.WonAt == "" {
		t.Fatalf("결렬 won_at 없음 %+v", fg)
	}

	items := []model.SalesProject{
		{WonAt: "2026-01-01"}, {WonAt: "2026-01-02"}, {WonAt: fg.WonAt, CloseReason: model.SalesCloseLost},
		{CloseReason: model.SalesCloseLost},
		{CloseReason: model.SalesCloseDropped}, {CloseReason: model.SalesCloseDropped},
	}
	w, l := model.SalesWinSample(items)
	if w != 3 || l != 1 || model.FormatSalesRate(w, w+l) != "75%" {
		t.Fatalf("수주율 won=%d lost=%d", w, l)
	}

	old := time.Now().AddDate(0, 0, -40).Format("2006-01-02")
	unsigned := &model.SalesProject{Name: "미계약"}
	_ = repo.Create(unsigned)
	_ = repo.ChangeStage(unsigned.SalesID, model.SalesDirectWin, "", "u1", "t", false)
	_, _ = db.Exec(`UPDATE sales_projects SET won_at=? WHERE sales_id=?`, old, unsigned.SalesID)
	ug, _ := repo.Get(unsigned.SalesID)
	if !model.SalesContractUnsigned(ug, time.Now()) {
		t.Fatal("30일 미계약이 아니다")
	}
}

func TestSalesOrderWonDeliveryDoesNotClose(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "s44d3.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewSalesRepo(db)
	p := &model.SalesProject{Name: "발주연동", BidEvalMethod: "lowest"}
	_ = repo.Create(p)
	_ = repo.ChangeStage(p.SalesID, model.SalesStage4Propose, "", "u1", "t", false)
	_ = repo.ChangeStage(p.SalesID, model.SalesStage4Bid, "", "u1", "t", false)
	if err := repo.SetBidResult(p.SalesID, "won", "", "", "u1", "t"); err != nil {
		t.Fatal(err)
	}
	before, _ := repo.Get(p.SalesID)
	// 납품은 핸들러 afterDelivery 가 단계를 안 바꾼다. 저장소도 그대로.
	after, _ := repo.Get(p.SalesID)
	if after.Stage != before.Stage || after.BidStatus != model.SalesBidWon {
		t.Fatalf("납품 후 단계가 바뀌었다 %s %s", after.Stage, after.BidStatus)
	}
}

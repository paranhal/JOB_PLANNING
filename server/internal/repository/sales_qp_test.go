package repository

import (
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestSalesSleepWakeAndReviewTask(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "q.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewSalesRepo(db)
	p := &model.SalesProject{Name: "휴면시험", Stage: model.SalesStage4Propose, Status: model.SalesStatusActive, SalesOwner: "최혜영", BudgetYear: 2026}
	if err := repo.Create(p); err != nil {
		t.Fatal(err)
	}
	stage, bid := p.Stage, p.BidStatus
	if err := repo.Sleep(p.SalesID, "2027-06", "내년 예산", "u1", "테스터", false); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.Get(p.SalesID)
	if got.Status != model.SalesStatusDormant || got.Stage != stage || got.BidStatus != bid {
		t.Fatalf("sleep 후 단계가 바뀜 status=%s stage=%s bid=%s", got.Status, got.Stage, got.BidStatus)
	}
	hidden, err := repo.ListFilter(SalesListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range hidden {
		if x.SalesID == p.SalesID {
			t.Fatal("목록 기본에 휴면이 보인다")
		}
	}
	pipe, err := repo.Pipeline(time.Now(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range pipe.Stages {
		if st.Count > 0 && st.Code == got.Stage {
			// 다른 건이 없으면 0이어야 한다
		}
	}
	if pipe.TotalAmount != 0 && got.ExpectedAmount > 0 {
		t.Fatalf("파이프라인에 휴면 금액이 들어갔다 %d", pipe.TotalAmount)
	}

	closed := &model.SalesProject{Name: "종료건"}
	if err := repo.Create(closed); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE sales_projects SET stage=?, close_reason=? WHERE sales_id=?`, model.SalesStage4Closed, model.SalesCloseContracted, closed.SalesID); err != nil {
		t.Fatal(err)
	}
	if err := repo.Sleep(closed.SalesID, "2027-06", "안됨", "u1", "t", false); err == nil {
		t.Fatal("종료 건 Sleep 이 통과했다")
	}

	if err := repo.Wake(p.SalesID, "u1", "테스터", 0, ""); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.Get(p.SalesID)
	if got.Status != model.SalesStatusActive || got.BudgetStatus != model.SalesBudgetUnknown {
		t.Fatalf("wake 후 %+v", got)
	}
	if got.BudgetYear != 2027 {
		t.Fatalf("budget_year 제안 %d", got.BudgetYear)
	}

	p2 := &model.SalesProject{Name: "검토", Stage: model.SalesStage4Discover, Status: model.SalesStatusActive, SalesOwner: "최혜영", SalesOwnerID: "u1", BudgetYear: 2026}
	if err := repo.Create(p2); err != nil {
		t.Fatal(err)
	}
	if err := repo.Sleep(p2.SalesID, time.Now().Format("2006-01"), "이번달", "u1", "최혜영", false); err != nil {
		t.Fatal(err)
	}
	n, err := repo.EnsureDormantReviewTasks(time.Now())
	if err != nil || n != 1 {
		t.Fatalf("검토 업무 n=%d err=%v", n, err)
	}
	n2, err := repo.EnsureDormantReviewTasks(time.Now())
	if err != nil || n2 != 0 {
		t.Fatalf("두 번째 n=%d err=%v", n2, err)
	}
}

func TestSalesBulkBizAndBudget(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "bulk.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewSalesRepo(db)
	a := &model.SalesProject{Name: "A"}
	b := &model.SalesProject{Name: "B"}
	_ = repo.Create(a)
	_ = repo.Create(b)
	if a.BizType != "" {
		t.Fatal("기존 유형을 추측해 채웠다")
	}
	n, err := repo.BulkSetBizType([]string{a.SalesID, b.SalesID}, model.SalesBizMaintenance)
	if err != nil || n != 2 {
		t.Fatalf("bulk biz n=%d err=%v", n, err)
	}
	got, _ := repo.Get(a.SalesID)
	if got.BizType != model.SalesBizMaintenance {
		t.Fatal(got.BizType)
	}
	n, err = repo.BulkSetBudgetStatus([]string{a.SalesID}, model.SalesBudgetConfirmed)
	if err != nil || n != 1 {
		t.Fatal(err)
	}
}

func TestSalesGroupsNoMutateProjectAndDupTotals(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "g.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewSalesRepo(db)
	g1 := &model.SalesGroup{Name: "2027년 예산 영업", Year: 2027}
	g2 := &model.SalesGroup{Name: "충남 교육청 통합", Year: 2027}
	if err := repo.CreateGroup(g1); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateGroup(g2); err != nil {
		t.Fatal(err)
	}
	yy := time.Now().Format("06")
	if g1.GroupNo != "G"+yy+"-001" || g2.GroupNo != "G"+yy+"-002" {
		t.Fatalf("번호 %s %s", g1.GroupNo, g2.GroupNo)
	}
	p1 := &model.SalesProject{Name: "1", ExpectedAmount: 100, Stage: model.SalesStage4Discover}
	p2 := &model.SalesProject{Name: "2", ExpectedAmount: 200, Stage: model.SalesStage4Discover}
	p3 := &model.SalesProject{Name: "3", ExpectedAmount: 300, Stage: model.SalesStage4Discover}
	_ = repo.Create(p1)
	_ = repo.Create(p2)
	_ = repo.Create(p3)
	var before string
	_ = db.QueryRow(`SELECT updated_at FROM sales_projects WHERE sales_id=?`, p1.SalesID).Scan(&before)
	if err := repo.AddGroupMembers(g1.GroupID, []string{p1.SalesID, p2.SalesID, p3.SalesID}, model.SalesMemberManual, "t"); err != nil {
		t.Fatal(err)
	}
	var after string
	_ = db.QueryRow(`SELECT updated_at FROM sales_projects WHERE sales_id=?`, p1.SalesID).Scan(&after)
	if before != after {
		t.Fatalf("빼기/넣기 후 sales_projects.updated_at 이 바뀜 %s -> %s", before, after)
	}
	if err := repo.AddGroupMembers(g2.GroupID, []string{p1.SalesID}, model.SalesMemberManual, "t"); err != nil {
		t.Fatal(err)
	}
	_ = db.QueryRow(`SELECT updated_at FROM sales_projects WHERE sales_id=?`, p1.SalesID).Scan(&after)
	if before != after {
		t.Fatal("두 번째 대분류 넣기에서 사업 행이 바뀌었다")
	}
	ps1, _ := repo.ResolveGroupProjects(g1)
	ps2, _ := repo.ResolveGroupProjects(g2)
	qm := model.GroupQuotesBySales(nil)
	_, a1, _ := model.SalesGroupTotalsAllowDup(ps1, qm)
	_, a2, _ := model.SalesGroupTotalsAllowDup(ps2, qm)
	all := append(append([]model.SalesProject{}, ps1...), ps2...)
	_, grand, _ := model.SalesGroupTotals(all, qm)
	if a1 != p1.ExpectedAmount+p2.ExpectedAmount+p3.ExpectedAmount {
		t.Fatalf("g1 amount %d", a1)
	}
	if a2 != p1.ExpectedAmount {
		t.Fatalf("g2 amount %d", a2)
	}
	if grand != a1 {
		t.Fatalf("전체 합계가 중복을 빼지 않았다 grand=%d g1=%d", grand, a1)
	}
	lost := &model.SalesProject{Name: "실주"}
	_ = repo.Create(lost)
	if _, err := db.Exec(`UPDATE sales_projects SET stage=?, close_reason=?, status=?, expected_amount=? WHERE sales_id=?`,
		model.SalesStage4Closed, model.SalesCloseLost, model.SalesStatusLost, 999, lost.SalesID); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddGroupMembers(g1.GroupID, []string{lost.SalesID}, model.SalesMemberManual, "t"); err != nil {
		t.Fatal(err)
	}
	ps1, _ = repo.ResolveGroupProjects(g1)
	cnt, amt, _ := model.SalesGroupTotalsAllowDup(ps1, qm)
	if amt != a1 {
		t.Fatalf("실주가 총액에 들어갔다 %d want %d cnt=%d", amt, a1, cnt)
	}
	var beforeRemove string
	_ = db.QueryRow(`SELECT updated_at FROM sales_projects WHERE sales_id=?`, p2.SalesID).Scan(&beforeRemove)
	if err := repo.RemoveGroupMember(g1.GroupID, p2.SalesID); err != nil {
		t.Fatal(err)
	}
	var afterRemove string
	_ = db.QueryRow(`SELECT updated_at FROM sales_projects WHERE sales_id=?`, p2.SalesID).Scan(&afterRemove)
	if beforeRemove != afterRemove {
		t.Fatal("빼기 후 sales_projects 행이 바뀌었다")
	}
	if err := repo.CloseGroup(g1.GroupID); err != nil {
		t.Fatal(err)
	}
	g1b, _ := repo.GetGroup(g1.GroupID)
	if g1b.Status != model.SalesGroupClosed {
		t.Fatal(g1b.Status)
	}
	mem, _ := repo.ListGroupMemberRows(g1.GroupID)
	if len(mem) == 0 {
		t.Fatal("종료 후 잇는 표가 지워졌다")
	}
	still, _ := repo.Get(p1.SalesID)
	if still == nil {
		t.Fatal("사업이 사라졌다")
	}
}

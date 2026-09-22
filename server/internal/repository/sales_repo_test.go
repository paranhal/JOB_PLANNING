package repository

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestSalesProjectNameOnlyAndStageRules(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "sales.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewSalesRepo(db)

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM codes WHERE code_group='sales_stage4'`).Scan(&n); err != nil || n < 4 {
		t.Fatalf("sales_stage4 codes n=%d err=%v", n, err)
	}
	var active int
	if err := db.QueryRow(`SELECT COUNT(*) FROM codes WHERE code_group='sales_stage4' AND is_active=1`).Scan(&active); err != nil || active != 4 {
		t.Fatalf("active sales_stage4=%d err=%v", active, err)
	}

	p := &model.SalesProject{Name: "세종 RFID 증설(가칭)", IsTentativeName: true}
	if err := repo.Create(p); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(p.SalesID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Stage != model.SalesStage4Discover || got.Probability != 10 {
		t.Fatalf("기본 단계/확도: stage=%s prob=%d", got.Stage, got.Probability)
	}
	if got.DealType != model.SalesDealBuild {
		t.Fatalf("기존 건 deal_type=%q", got.DealType)
	}
	if got.CustomerID != "" || got.ExpectedYM != "" || got.ExpectedAmount != 0 {
		t.Fatalf("빈 칸이 채워졌다: %+v", got)
	}

	if err := repo.ChangeStage(p.SalesID, model.SalesStage4Propose, "", "u1", "최혜영", false); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.Get(p.SalesID)
	if got.Probability != 20 || got.Stage != model.SalesStage4Propose {
		t.Fatalf("단계 변경 후 확도: stage=%s prob=%d", got.Stage, got.Probability)
	}

	if err := repo.ChangeStage(p.SalesID, model.SalesStage4Discover, "", "u1", "최혜영", false); err == nil {
		t.Fatal("후퇴에 사유 없이 통과했다")
	}
	if err := repo.ChangeStage(p.SalesID, model.SalesStage4Discover, "내년으로 이연", "u1", "최혜영", false); err != nil {
		t.Fatal(err)
	}
	hist, err := repo.ListHistory(p.SalesID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, h := range hist {
		if h.ToStage == model.SalesStage4Discover && strings.Contains(h.Reason, "내년으로 이연") {
			found = true
		}
	}
	if !found {
		t.Fatalf("후퇴 이력이 없다: %+v", hist)
	}

	if err := repo.ChangeStage(p.SalesID, model.SalesDirectWin, "", "u1", "최혜영", false); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.Get(p.SalesID)
	if got.Stage != model.SalesStage4Bid || got.BidStatus != model.SalesBidWon || got.Probability != 100 {
		t.Fatalf("바로 수주: stage=%s bid=%s prob=%d", got.Stage, got.BidStatus, got.Probability)
	}
	if strings.TrimSpace(got.WonAt) == "" {
		t.Fatal("won_at 이 비었다")
	}
}

func TestSalesProjectDoesNotAffectStatsKPI(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "sales_stats.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c1','도서관','도서관',1)`); err != nil {
		t.Fatal(err)
	}
	setMetricsPolicy(t, db, "2026-08-01", "as,maintenance")
	if _, err := db.Exec(`
		INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date,
			start_datetime, status, assigned_to, complete_datetime, process_type, visit_date, data_origin)
		VALUES ('a1','R1','c1','2026-08-01','2026-08-03','2026-08-03','completed','양기헌','2026-08-05','visit','2026-08-03','app')`); err != nil {
		t.Fatal(err)
	}
	if err := NewSalesRepo(db).Create(&model.SalesProject{
		Name: "영업 건은 지표에 안 탄다", IsTentativeName: true, ExpectedAmount: 180000000,
	}); err != nil {
		t.Fatal(err)
	}

	repo := NewStatsRepo(db)
	anchor := time.Date(2026, 8, 7, 0, 0, 0, 0, time.Local)
	cols := BuildStatsPeriodColumns(model.StatsViewMonth, anchor)
	f := ParseMeetingFilter(model.StatsScopeTeam, "", "")
	if err := repo.FillPeriodOverview(cols, f); err != nil {
		t.Fatal(err)
	}
	kpi, err := repo.LoadStatsKPI(model.StatsViewMonth, cols, f)
	if err != nil {
		t.Fatal(err)
	}
	an, err := repo.LoadStatsWorkAnalysis("2026-08-01", "2026-09-01", f)
	if err != nil {
		t.Fatal(err)
	}
	events, err := repo.listWeeklyASEvents("2026-08-01", "2026-09-01")
	if err != nil {
		t.Fatal(err)
	}
	got := fmt.Sprintf("visit=%.6f complete=%.6f receipt=%d completed=%d events=%d",
		kpi.VisitAvgDays, kpi.CompleteAvgDays, an.AS.Receipt, an.AS.Completed, len(events))
	const want = "visit=2.000000 complete=4.000000 receipt=1 completed=1 events=2"
	if got != want {
		t.Fatalf("영업 건이 통계에 섞였다\n got %s\nwant %s", got, want)
	}
	if err := NewSalesRepo(db).Create(&model.SalesProject{
		Name: "단품 토너", DealType: model.SalesDealSupply, ExpectedAmount: 850000, PONo: "PO-1",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.FillPeriodOverview(cols, f); err != nil {
		t.Fatal(err)
	}
	kpi2, err := repo.LoadStatsKPI(model.StatsViewMonth, cols, f)
	if err != nil {
		t.Fatal(err)
	}
	an2, err := repo.LoadStatsWorkAnalysis("2026-08-01", "2026-09-01", f)
	if err != nil {
		t.Fatal(err)
	}
	events2, err := repo.listWeeklyASEvents("2026-08-01", "2026-09-01")
	if err != nil {
		t.Fatal(err)
	}
	got2 := fmt.Sprintf("visit=%.6f complete=%.6f receipt=%d completed=%d events=%d",
		kpi2.VisitAvgDays, kpi2.CompleteAvgDays, an2.AS.Receipt, an2.AS.Completed, len(events2))
	if got2 != want {
		t.Fatalf("단품 건이 통계에 섞였다\n got %s\nwant %s", got2, want)
	}
}

func TestSalesActivityCreatesWorkTaskAndExecToggle(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "sales_act_stats.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c1','도서관','도서관',1)`); err != nil {
		t.Fatal(err)
	}
	setMetricsPolicy(t, db, "2026-08-01", "as,maintenance")
	if _, err := db.Exec(`
		INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date,
			start_datetime, status, assigned_to, complete_datetime, process_type, visit_date, data_origin)
		VALUES ('a1','R1','c1','2026-08-01','2026-08-03','2026-08-03','completed','양기헌','2026-08-05','visit','2026-08-03','app')`); err != nil {
		t.Fatal(err)
	}

	sales := NewSalesRepo(db)
	p := &model.SalesProject{Name: "영업 활동 연동", IsTentativeName: true}
	if err := sales.Create(p); err != nil {
		t.Fatal(err)
	}

	kpiOf := func(f model.StatsMeetingFilter) (model.StatsKPICard, error) {
		repo := NewStatsRepo(db)
		anchor := time.Date(2026, 8, 7, 0, 0, 0, 0, time.Local)
		cols := BuildStatsPeriodColumns(model.StatsViewMonth, anchor)
		if err := repo.FillPeriodOverview(cols, f); err != nil {
			return model.StatsKPICard{}, err
		}
		return repo.LoadStatsKPI(model.StatsViewMonth, cols, f)
	}

	baseF := ParseMeetingFilter(model.StatsScopeTeam, "", "")
	base, err := kpiOf(baseF)
	if err != nil {
		t.Fatal(err)
	}

	act := &model.SalesActivity{
		SalesID: p.SalesID, ActivityDate: "2026-08-19", StartTime: "10:00",
		DurationMin: 60, ActivityType: "visit", Title: "방문미팅",
		OurMembers: "최혜영", NextAction: "견적 요청",
	}
	if err := sales.CreateActivity(act, "관리자", nil); err != nil {
		t.Fatal(err)
	}
	task, err := NewWBRepo(db).GetTaskBySource(model.WBSourceSalesActivity, act.ActivityID)
	if err != nil || task == nil {
		t.Fatalf("work_tasks 미생성: err=%v", err)
	}
	if task.WorkDate != "2026-08-19" || task.Assignee != "최혜영" {
		t.Fatalf("동기화 필드: date=%s assignee=%s", task.WorkDate, task.Assignee)
	}

	inc, err := kpiOf(baseF)
	if err != nil {
		t.Fatal(err)
	}
	if inc.VisitAvgDays != base.VisitAvgDays || inc.CompleteAvgDays != base.CompleteAvgDays {
		t.Fatalf("3일·7일이 변했다 visit %.6f→%.6f complete %.6f→%.6f",
			base.VisitAvgDays, inc.VisitAvgDays, base.CompleteAvgDays, inc.CompleteAvgDays)
	}

	exF := baseF
	exF.ExcludeSalesActivity = true
	ex, err := kpiOf(exF)
	if err != nil {
		t.Fatal(err)
	}
	if ex.VisitAvgDays != base.VisitAvgDays || ex.CompleteAvgDays != base.CompleteAvgDays {
		t.Fatalf("토글 꺼도 방문·완료가 변하면 안 됨 visit %.6f→%.6f complete %.6f→%.6f",
			base.VisitAvgDays, ex.VisitAvgDays, base.CompleteAvgDays, ex.CompleteAvgDays)
	}
}

func TestSalesUnplannedNoFollowAndStale(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "sales_unplanned.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	sales := NewSalesRepo(db)
	p := &model.SalesProject{Name: "후속 없는 건", IsTentativeName: true, ExpectedYM: "2020-01"}
	if err := sales.Create(p); err != nil {
		t.Fatal(err)
	}
	items, counts, err := NewWorkBoardRepo(db).ListUnplanned("", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	var found *model.UnplannedItem
	for i := range items {
		if items[i].RefID == p.SalesID {
			found = &items[i]
			break
		}
	}
	if found == nil {
		t.Fatal("미계획함에 영업 건이 없다")
	}
	if !found.HasKind(model.UnplannedSalesFollow) {
		t.Fatalf("후속없음 없음 kinds=%v", found.Kinds)
	}
	if !found.HasKind(model.UnplannedReview) {
		t.Fatalf("재검토 없음 kinds=%v", found.Kinds)
	}
	if counts.SalesFollow < 1 || counts.Review < 1 {
		t.Fatalf("counts follow=%d review=%d", counts.SalesFollow, counts.Review)
	}
}

func TestSalesStageProbabilityReadsFromCodes(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "sales_codes.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`UPDATE codes SET code_name='18' WHERE code_group='sales_stage4_prob' AND code_value='propose'`); err != nil {
		t.Fatal(err)
	}
	LoadLookupCache(db)
	stages, err := NewSalesRepo(db).Stages()
	if err != nil {
		t.Fatal(err)
	}
	d := model.FindSalesStage(stages, model.SalesStage4Propose)
	if d == nil || d.Probability != 18 {
		t.Fatalf("codes 확도 미반영: %+v", d)
	}
}

func TestSalesAmountChangeAndPartyReplaceKeepOld(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "sales_party.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewSalesRepo(db)

	p := &model.SalesProject{Name: "세종 RFID", IsTentativeName: true, ExpectedAmount: 30_000_000}
	if err := repo.Create(p); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(p.SalesID)
	if err != nil {
		t.Fatal(err)
	}
	got.ExpectedAmount = 180_000_000
	if err := repo.Update(got, "최혜영"); err != nil {
		t.Fatal(err)
	}
	changes, err := repo.ListChanges(p.SalesID)
	if err != nil {
		t.Fatal(err)
	}
	foundAmt := false
	for _, c := range changes {
		if c.FieldKey != model.SalesChangeAmount {
			continue
		}
		if strings.Contains(c.OldValue, "30,000,000") && strings.Contains(c.NewValue, "180,000,000") {
			foundAmt = true
		}
	}
	if !foundAmt {
		t.Fatalf("금액 변경 이력이 없다: %+v", changes)
	}

	oldP := &model.SalesParty{
		SalesID: p.SalesID, PartyType: model.SalesPartyCustomer,
		OrgName: "세종시립도서관", PersonName: "김담당", PartyRole: "working",
	}
	if err := repo.CreateParty(oldP, "최혜영"); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceParty(oldP.PartyID, &model.SalesParty{
		PersonName: "이후임", OrgName: "세종시립도서관", PartyRole: "working",
	}, "", "최혜영"); err == nil {
		t.Fatal("교체 사유 없이 통과했다")
	}
	neu := &model.SalesParty{
		PersonName: "이후임", OrgName: "세종시립도서관", PartyRole: "working",
	}
	if err := repo.ReplaceParty(oldP.PartyID, neu, "전배", "최혜영"); err != nil {
		t.Fatal(err)
	}

	kept, err := repo.GetParty(oldP.PartyID)
	if err != nil {
		t.Fatal(err)
	}
	if kept.IsActive {
		t.Fatal("옛 관계자가 활성으로 남았다")
	}
	if kept.PersonName != "김담당" {
		t.Fatalf("옛 이름이 바뀌었다: %s", kept.PersonName)
	}
	if kept.ReplacedBy != neu.PartyID {
		t.Fatalf("replaced_by=%s want %s", kept.ReplacedBy, neu.PartyID)
	}
	if kept.ReplacedReason != "전배" {
		t.Fatalf("교체 사유=%s", kept.ReplacedReason)
	}

	active, err := repo.ListParties(p.SalesID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].PersonName != "이후임" {
		t.Fatalf("활성 관계자: %+v", active)
	}
	all, err := repo.ListParties(p.SalesID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("교체 후 행 수=%d (옛 사람이 지워졌다)", len(all))
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sales_parties WHERE sales_id=? AND person_name='김담당' AND is_active=0`, p.SalesID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("옛 사람 비활성 잔존 n=%d err=%v", n, err)
	}
}

func TestSalesPipelineAndActivityMove(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "sales_pipe.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewSalesRepo(db)

	lead := &model.SalesProject{Name: "리드", IsTentativeName: true, ExpectedAmount: 30_000_000, ExpectedYM: "2026-09"}
	if err := repo.Create(lead); err != nil {
		t.Fatal(err)
	}
	won := &model.SalesProject{
		Name: "계약", ExpectedAmount: 100_000_000, ExpectedYM: "2026-10",
		CustomerConfirmed: true, ExpectedYMConfirmed: true, ExpectedAmountConfirmed: true,
	}
	if err := repo.Create(won); err != nil {
		t.Fatal(err)
	}
	if err := repo.ChangeStage(won.SalesID, model.SalesDirectWin, "", "u1", "t", false); err != nil {
		t.Fatal(err)
	}
	lost := &model.SalesProject{Name: "실패", IsTentativeName: true, LostReason: "예산"}
	if err := repo.Create(lost); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE sales_projects SET stage=?, close_reason=?, status=? WHERE sales_id=?`,
		model.SalesStage4Closed, model.SalesCloseLost, model.SalesStatusLost, lost.SalesID); err != nil {
		t.Fatal(err)
	}

	act := &model.SalesActivity{
		SalesID: lead.SalesID, ActivityDate: "2026-08-19", StartTime: "10:00",
		DurationMin: 30, ActivityType: "visit", Title: "방문", OurMembers: "최혜영",
	}
	if err := repo.CreateActivity(act, "최혜영", nil); err != nil {
		t.Fatal(err)
	}

	pipe, err := repo.Pipeline(time.Date(2026, 8, 21, 0, 0, 0, 0, time.Local), []string{"최혜영", "태자운"})
	if err != nil {
		t.Fatal(err)
	}
	if pipe.WinRateLabel != "50%" {
		t.Fatalf("수주율=%s", pipe.WinRateLabel)
	}
	if pipe.WeightedTotal <= 0 {
		t.Fatalf("가중 파이프라인=%d", pipe.WeightedTotal)
	}
	var hy, tae int
	for _, p := range pipe.People {
		if p.Name == "최혜영" {
			hy = p.Count
		}
		if p.Name == "태자운" {
			tae = p.Count
		}
	}
	if hy < 1 || tae != 0 {
		t.Fatalf("담당자 활동 최혜영=%d 태자운=%d", hy, tae)
	}

	if err := repo.MoveActivity(act.ActivityID, "type", "call", "최혜영"); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetActivity(act.ActivityID)
	if err != nil || got.ActivityType != "call" {
		t.Fatalf("유형 이동: %+v err=%v", got, err)
	}
	if err := repo.MoveActivity(act.ActivityID, "stage", model.SalesStage4Propose, "최혜영"); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.GetActivity(act.ActivityID)
	if got.StageAtTime != model.SalesStage4Propose {
		t.Fatalf("단계 스냅샷=%s", got.StageAtTime)
	}
	leadGot, _ := repo.Get(lead.SalesID)
	if leadGot.Stage != model.SalesStage4Discover {
		t.Fatalf("활동 칸반이 사업 단계를 바꿨다: %s", leadGot.Stage)
	}
}

func TestSalesDealTypeSupplyStagesAndAutoAdvance(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "sales_deal.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewSalesRepo(db)

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM codes WHERE code_group='sales_stage4' AND is_active=1`).Scan(&n); err != nil || n != 4 {
		t.Fatalf("4단계 codes n=%d err=%v", n, err)
	}
	build, err := repo.Stages()
	if err != nil || len(build) != 4 {
		t.Fatalf("단계 n=%d err=%v", len(build), err)
	}
	supply, err := repo.StagesFor(model.SalesDealSupply)
	if err != nil || len(supply) != 4 {
		t.Fatalf("단품도 4단계 n=%d err=%v", len(supply), err)
	}

	buildP := &model.SalesProject{Name: "구축 건", IsTentativeName: true}
	if err := repo.Create(buildP); err != nil {
		t.Fatal(err)
	}
	p := &model.SalesProject{Name: "토너 납품", DealType: model.SalesDealSupply, ExpectedAmount: 850000}
	if err := repo.Create(p); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(p.SalesID)
	if err != nil {
		t.Fatal(err)
	}
	if got.DealType != model.SalesDealSupply || got.Stage != model.SalesStage4Discover || got.Probability != 10 {
		t.Fatalf("등록 후: deal=%s stage=%s prob=%d", got.DealType, got.Stage, got.Probability)
	}

	onlyBuild, err := repo.ListFilter(SalesListFilter{DealType: model.SalesDealBuild})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range onlyBuild {
		if it.DealType == model.SalesDealSupply {
			t.Fatal("기본 목록에 단품이 보였다")
		}
	}
	onlySupply, err := repo.ListFilter(SalesListFilter{DealType: model.SalesDealSupply})
	if err != nil || len(onlySupply) != 1 || onlySupply[0].SalesID != p.SalesID {
		t.Fatalf("단품 목록: n=%d", len(onlySupply))
	}

	if err := repo.ChangeStage(p.SalesID, model.SalesStageProposal, "", "u1", "최혜영", false); err == nil {
		t.Fatal("옛 단계 코드로 옮겼다")
	}

	got.PONo = "PO-2026-01"
	if err := repo.Update(got, "최혜영"); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.Get(p.SalesID)
	if got.Stage != model.SalesStage4Discover {
		t.Fatalf("발주 저장이 단계를 바꿨다=%s", got.Stage)
	}
	got.DeliveredAt = "2026-09-04"
	if err := repo.Update(got, "최혜영"); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.Get(p.SalesID)
	if got.Stage != model.SalesStage4Discover {
		t.Fatalf("납품일 저장이 단계를 바꿨다=%s", got.Stage)
	}
	if err := repo.ChangeStage(p.SalesID, model.SalesStage4Closed, "끝", "u1", "최혜영", false); err == nil {
		t.Fatal("ChangeStage 로 종료됐다")
	}
}

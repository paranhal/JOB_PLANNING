package repository

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestSalesNextYMUnplannedInbox(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "sales_next_ym.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if model.UnplannedKindLabel(model.UnplannedSalesNext) != "영업 다음행동" {
		t.Fatalf("이름 %q", model.UnplannedKindLabel(model.UnplannedSalesNext))
	}
	b := model.UnplannedBadgeOf(model.UnplannedSalesNext, 0)
	if b.Label != "날짜미정" || b.Class != "bg-orange-100 text-orange-800" {
		t.Fatalf("배지 %+v", b)
	}

	sales := NewSalesRepo(db)
	p := &model.SalesProject{Name: "월만 다음행동", IsTentativeName: true}
	if err := sales.Create(p); err != nil {
		t.Fatal(err)
	}
	today := time.Now().Format("2006-01-02")
	act := &model.SalesActivity{
		SalesID: p.SalesID, ActivityDate: today, StartTime: "10:00",
		DurationMin: 30, ActivityType: "visit", Title: "방문",
		OurMembers: "최혜영", NextAction: "협의 계속", NextActionDate: "2026-10",
	}
	if err := sales.CreateActivity(act, "관리자", nil); err != nil {
		t.Fatal(err)
	}

	inOct, err := sales.ListNextActionYMGaps("2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if !hasNextGap(inOct, act.ActivityID) {
		t.Fatal("2026-10 이면 미계획함에 떠야 한다")
	}

	inSep, err := sales.ListNextActionYMGaps("2026-09")
	if err != nil {
		t.Fatal(err)
	}
	if hasNextGap(inSep, act.ActivityID) {
		t.Fatal("미래 달은 미계획함에 뜨면 안 된다")
	}

	inNov, err := sales.ListNextActionYMGaps("2026-11")
	if err != nil {
		t.Fatal(err)
	}
	if !hasNextGap(inNov, act.ActivityID) {
		t.Fatal("지난 달은 계속 떠야 한다")
	}

	wb := NewWorkBoardRepo(db)
	thisYM := time.Now().Format("2006-01")
	current := &model.SalesActivity{
		SalesID: p.SalesID, ActivityDate: today, StartTime: "10:30",
		DurationMin: 30, ActivityType: "visit", Title: "이번달",
		OurMembers: "최혜영", NextAction: "이번달 협의", NextActionDate: thisYM,
	}
	if err := sales.CreateActivity(current, "관리자", nil); err != nil {
		t.Fatal(err)
	}
	itemsNow, _, err := wb.ListUnplanned("", nil, model.UnplannedSalesNext)
	if err != nil {
		t.Fatal(err)
	}
	var foundNow *model.UnplannedItem
	for i := range itemsNow {
		if itemsNow[i].ItemKey == "salesact:"+current.ActivityID {
			foundNow = &itemsNow[i]
			break
		}
	}
	if foundNow == nil || !foundNow.HasKind(model.UnplannedSalesNext) || foundNow.HasKind(model.UnplannedReview) {
		t.Fatalf("이번 달 월만은 미계획함(날짜미정)에만 떠야 한다 %+v", foundNow)
	}
	if !foundNow.NeedPlanKind() || foundNow.OverdueKind() || !foundNow.CanAssignDate {
		t.Fatalf("이번 달 분류 NeedPlan/Overdue/CanAssign %+v", foundNow)
	}

	overdue := &model.SalesActivity{
		SalesID: p.SalesID, ActivityDate: today, StartTime: "11:00",
		DurationMin: 30, ActivityType: "visit", Title: "지난달",
		OurMembers: "최혜영", NextAction: "재검토 대상", NextActionDate: "2026-08",
	}
	if err := sales.CreateActivity(overdue, "관리자", nil); err != nil {
		t.Fatal(err)
	}
	items, _, err := wb.ListUnplanned("", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	var foundOverdue *model.UnplannedItem
	for i := range items {
		if items[i].ItemKey == "salesact:"+overdue.ActivityID {
			foundOverdue = &items[i]
			break
		}
	}
	if thisYM >= "2026-09" {
		if foundOverdue == nil || !foundOverdue.HasKind(model.UnplannedSalesNext) || !foundOverdue.HasKind(model.UnplannedReview) {
			t.Fatalf("지난 달 월만은 재검토가 같이 있어야 한다 %+v", foundOverdue)
		}
		if !foundOverdue.CanAssignDate || foundOverdue.CanNoDate {
			t.Fatalf("날짜 지정 가능해야 한다 %+v", foundOverdue)
		}
		if foundOverdue.NeedPlanKind() || !foundOverdue.OverdueKind() {
			t.Fatalf("지난 달은 재검토(밀린 건)여야 한다 need=%v overdue=%v", foundOverdue.NeedPlanKind(), foundOverdue.OverdueKind())
		}
	}

	dated := &model.SalesActivity{
		SalesID: p.SalesID, ActivityDate: today, StartTime: "12:00",
		DurationMin: 30, ActivityType: "visit", Title: "날짜 있음",
		OurMembers: "최혜영", NextAction: "협의 계속", NextActionDate: "2026-09-30",
	}
	if err := sales.CreateActivity(dated, "관리자", nil); err != nil {
		t.Fatal(err)
	}
	if hasNextGapMust(t, sales, "2026-10", dated.ActivityID) {
		t.Fatal("날짜가 있는 건은 미계획함에 뜨면 안 된다")
	}

	empty := &model.SalesActivity{
		SalesID: p.SalesID, ActivityDate: today, StartTime: "13:00",
		DurationMin: 30, ActivityType: "visit", Title: "다음행동 없음",
		OurMembers: "최혜영", NextAction: "", NextActionDate: "2026-10",
	}
	if err := sales.CreateActivity(empty, "관리자", nil); err != nil {
		t.Fatal(err)
	}
	if hasNextGapMust(t, sales, "2026-10", empty.ActivityID) {
		t.Fatal("next_action 이 비면 뜨면 안 된다")
	}

	assignDay := "2026-10-15"
	if err := wb.AssignUnplannedDate("salesact:"+act.ActivityID, assignDay, "김담당", ""); err != nil {
		t.Fatal(err)
	}
	nxt, err := NewWBRepo(db).GetTaskBySource(model.WBSourceSalesActivity, act.ActivityID, model.WBSourceRoleNext)
	if err != nil || nxt == nil {
		t.Fatalf("다음 업무가 안 생겼다 %v", err)
	}
	if nxt.SourceRole != model.WBSourceRoleNext || nxt.WorkType != model.WBWorkSales || nxt.Status != model.WBTaskWaiting {
		t.Fatalf("다음 업무 모양 %+v", nxt)
	}
	if nxt.WorkDate != assignDay || nxt.DueDate != assignDay {
		t.Fatalf("지정일 work=%s due=%s", nxt.WorkDate, nxt.DueDate)
	}
	if nxt.Assignee != "김담당" {
		t.Fatalf("담당자 %q", nxt.Assignee)
	}
	gotAct, err := sales.GetActivity(act.ActivityID)
	if err != nil {
		t.Fatal(err)
	}
	if gotAct.NextActionDate != "2026-10" {
		t.Fatalf("원본 날짜를 덮었다 %q", gotAct.NextActionDate)
	}
	if hasNextGapMust(t, sales, "2026-10", act.ActivityID) {
		t.Fatal("업무가 생기면 미계획함에서 빠져야 한다")
	}

	if err := wb.AssignUnplannedDate("salesact:"+act.ActivityID, "2026-10-20", "김담당", ""); err != nil {
		t.Fatal(err)
	}
	if n := countSourceRoleTasks(t, db, act.ActivityID, model.WBSourceRoleNext); n != 1 {
		t.Fatalf("날짜 지정 두 번 → 다음 업무 %d건", n)
	}
}

func hasNextGap(gaps []model.SalesNextGap, activityID string) bool {
	for _, g := range gaps {
		if g.ActivityID == activityID {
			return true
		}
	}
	return false
}

func hasNextGapMust(t *testing.T, sales *SalesRepo, todayYM, activityID string) bool {
	t.Helper()
	gaps, err := sales.ListNextActionYMGaps(todayYM)
	if err != nil {
		t.Fatal(err)
	}
	return hasNextGap(gaps, activityID)
}

func countSourceRoleTasks(t *testing.T, db *sql.DB, activityID, role string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_tasks WHERE source_type=? AND source_id=? AND source_role=?`,
		model.WBSourceSalesActivity, activityID, role).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

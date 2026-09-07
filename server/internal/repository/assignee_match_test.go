package repository

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

type assigneeMatchFix struct {
	db     *sql.DB
	date   string
	userID string
	taskID string
}

func TestAssigneeMatchSQLFragments(t *testing.T) {
	asSQL, asArgs := assigneeMatchSQL(AssigneeKindAS, "ar", "양기헌")
	if asSQL == "" || len(asArgs) == 0 {
		t.Fatal("AS 매칭 SQL이 비었다")
	}
	if !strings.Contains(asSQL, "assigned_to") || !strings.Contains(asSQL, "assigned_user_id") {
		t.Fatalf("AS 컬럼 없음: %s", asSQL)
	}
	if !strings.Contains(asSQL, "work_task_members") {
		t.Fatalf("AS 다중 담당자 없음: %s", asSQL)
	}

	mntSQL, _ := assigneeMatchSQL(AssigneeKindMaintenance, "v", "양기헌")
	if !strings.Contains(mntSQL, "v.assignee") {
		t.Fatalf("정기점검 assignee 없음: %s", mntSQL)
	}

	taskSQL, _ := assigneeMatchSQL(AssigneeKindTask, "t", "양기헌")
	if !strings.Contains(taskSQL, "t.assignee") || !strings.Contains(taskSQL, "work_task_members") {
		t.Fatalf("행정 멤버 없음: %s", taskSQL)
	}

	both, args := assigneeMatchSQLID(AssigneeKindAS, "ar", "양기헌", "USR-1")
	if both == "" || len(args) < 2 {
		t.Fatal("이름·ID 둘 다 받아야 한다")
	}
	empty, emptyArgs := assigneeMatchSQL(AssigneeKindAS, "ar", "  ")
	if empty != "" || emptyArgs != nil {
		t.Fatalf("빈 키는 조건이 없어야 한다: %q %v", empty, emptyArgs)
	}
}

func TestAssigneeMatch_AssignedUserIDOnlyAS(t *testing.T) {
	fx := seedAssigneeMatchFixture(t)
	items := mustListScheduled(t, fx)
	if !hasWorkRef(items, "as-id-only") {
		t.Fatalf("assigned_user_id 전용 AS가 안 걸렸다: %v", refsOf(items))
	}

	c := mustAssigneeDayCounts(t, fx.db)
	if c.AS.Planned != 1 {
		t.Fatalf("통계 AS 예정=%d want 1", c.AS.Planned)
	}
}

func TestAssigneeMatch_SupportMemberSeesTask(t *testing.T) {
	fx := seedAssigneeMatchFixture(t)
	items := mustListScheduled(t, fx)
	if !hasWorkRef(items, fx.taskID) {
		t.Fatalf("지원 담당자 화면에 업무가 없다: %v", refsOf(items))
	}

	c := mustAssigneeDayCounts(t, fx.db)
	if c.Admin.Planned != 1 {
		t.Fatalf("통계 행정 예정=%d want 1 (지원 담당자)", c.Admin.Planned)
	}
}

func TestAssigneeMatch_MaintenanceAssignee(t *testing.T) {
	fx := seedAssigneeMatchFixture(t)
	items := mustListScheduled(t, fx)
	if !hasWorkRef(items, "mv-yang") {
		t.Fatalf("정기점검이 담당자 필터에 안 걸렸다: %v", refsOf(items))
	}

	c := mustAssigneeDayCounts(t, fx.db)
	if c.Mnt.Planned != 1 {
		t.Fatalf("통계 점검 예정=%d want 1", c.Mnt.Planned)
	}
}

func TestAssigneeMatch_StatsAndListSameCount(t *testing.T) {
	fx := seedAssigneeMatchFixture(t)
	items := mustListScheduled(t, fx)
	var asN, mntN, adminN int
	for _, it := range items {
		switch it.Prefix {
		case model.WorkPrefixAS:
			asN++
		case model.WorkPrefixMaintenance:
			mntN++
		default:
			adminN++
		}
	}
	c := mustAssigneeDayCounts(t, fx.db)
	if c.AS.Planned != asN || c.Mnt.Planned != mntN || c.Admin.Planned != adminN {
		t.Fatalf("통계 AS/점검/행정=%d/%d/%d 목록=%d/%d/%d refs=%v",
			c.AS.Planned, c.Mnt.Planned, c.Admin.Planned, asN, mntN, adminN, refsOf(items))
	}
}

func mustListScheduled(t *testing.T, fx assigneeMatchFix) []model.WorkListItem {
	t.Helper()
	items, err := NewWorkBoardRepo(fx.db).ListScheduledOn(fx.date, fx.userID, []string{"양기헌"}, 100)
	if err != nil {
		t.Fatal(err)
	}
	return items
}

func mustAssigneeDayCounts(t *testing.T, db *sql.DB) model.StatsBucketCounts {
	t.Helper()
	cols := BuildStatsPeriodColumns(model.StatsViewDay, time.Date(2026, 8, 11, 0, 0, 0, 0, time.Local))
	if err := NewStatsRepo(db).FillPeriodOverview(cols, ParseMeetingFilter(model.StatsScopeAssignee, "양기헌", "")); err != nil {
		t.Fatal(err)
	}
	return cols[1].Counts
}

func seedAssigneeMatchFixture(t *testing.T) assigneeMatchFix {
	t.Helper()
	db, err := InitDB(filepath.Join(t.TempDir(), "assignee_match.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	date := "2026-08-11"
	u := &model.User{Username: "yang", PasswordHash: "x", FullName: "양기헌", Role: model.RoleTech, IsActive: true}
	if err := NewUserRepo(db).Create(u); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c-am','매칭도서관','매칭도서관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date,
		schedule_confirmed, status, assigned_to, assigned_user_id, symptom, updated_at, data_origin
	) VALUES (
		'as-id-only','R2608-ID01','c-am','2026-08-10',?,
		1,'assigned','',?,'ID만 배정',?,'app')`, date, u.UserID, date+" 09:00:00"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_plans (plan_id, plan_year, title, status)
		VALUES ('mp-am', 2026, '매칭계획', 'approved')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits (
		visit_id, plan_id, visit_date, customer_id, sort_order, completed, assignee, product_type, data_origin
	) VALUES ('mv-yang','mp-am',?,'c-am',1,0,'양기헌','KLAS','app')`, date); err != nil {
		t.Fatal(err)
	}

	wb := NewWBRepo(db)
	task := &model.WorkTask{
		WorkType:    model.WBWorkAdmin,
		Title:       "지원 배정 행정",
		DueDate:     date,
		WorkDate:    date,
		ReceiptDate: date,
		Status:      model.WBTaskWaiting,
		Assignee:    "최혜영",
	}
	if err := wb.CreateTask(task); err != nil {
		t.Fatal(err)
	}
	if err := wb.ReplaceSupportMembers(task.TaskID, []model.WorkTaskMember{
		{Assignee: "양기헌", Role: model.WBMemberSupport},
	}); err != nil {
		t.Fatal(err)
	}

	return assigneeMatchFix{db: db, date: date, userID: u.UserID, taskID: task.TaskID}
}

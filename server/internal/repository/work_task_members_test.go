package repository

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

func TestWorkTaskMembersTableAndBackfill(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	mustHaveAssigneeColumn(t, db)

	dropWorkTaskMemberTriggers(db)
	if _, err := db.Exec(`DROP TABLE IF EXISTS work_task_members`); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(`
		INSERT INTO work_tasks (task_id, work_type, title, status, assignee, duration_min)
		VALUES
		('WT-1','admin','업무1','waiting','최혜영',30),
		('WT-2','admin','업무2','waiting','양기헌',45),
		('WT-3','admin','업무3','waiting','',60)`); err != nil {
		t.Fatal(err)
	}

	applyWorkTaskMembers(db)

	var tasks, members, owners, emptyOwners int
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_tasks`).Scan(&tasks); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_task_members`).Scan(&members); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_task_members WHERE member_role='owner'`).Scan(&owners); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_task_members WHERE member_role='owner' AND assignee=''`).Scan(&emptyOwners); err != nil {
		t.Fatal(err)
	}
	if tasks != members {
		t.Fatalf("이관 건수 members=%d tasks=%d (운영 135건과 같이 1:1)", members, tasks)
	}
	if owners != tasks || emptyOwners != 1 {
		t.Fatalf("owner=%d empty=%d tasks=%d", owners, emptyOwners, tasks)
	}

	var taskAssignee, memberAssignee string
	if err := db.QueryRow(`SELECT COALESCE(assignee,'') FROM work_tasks WHERE task_id='WT-3'`).Scan(&taskAssignee); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT assignee FROM work_task_members WHERE task_id='WT-3' AND member_role='owner'`).Scan(&memberAssignee); err != nil {
		t.Fatal(err)
	}
	if taskAssignee != "" || memberAssignee != "" {
		t.Fatalf("빈 값은 미배정으로 남겨야 한다 task=%q member=%q", taskAssignee, memberAssignee)
	}
}

func TestWorkTaskMembersOwnerConstraints(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "own.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`
		INSERT INTO work_tasks (task_id, work_type, title, status, assignee, duration_min)
		VALUES ('WT-A','admin','주담당 검사','waiting','최혜영',30)`); err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`INSERT INTO work_task_members (task_id, assignee, member_role, sort_order)
		VALUES ('WT-A','양기헌','owner',1)`)
	mustAbortOwner(t, err)

	_, err = db.Exec(`DELETE FROM work_task_members WHERE task_id='WT-A' AND member_role='owner'`)
	mustAbortOwner(t, err)

	_, err = db.Exec(`UPDATE work_task_members SET member_role='support' WHERE task_id='WT-A' AND member_role='owner'`)
	mustAbortOwner(t, err)

	if _, err := db.Exec(`INSERT INTO work_task_members (task_id, assignee, member_role, duration_min, sort_order)
		VALUES ('WT-A','양기헌','support',120,1)`); err != nil {
		t.Fatalf("지원 1명 추가는 허용: %v", err)
	}

	if _, err := db.Exec(`DELETE FROM work_tasks WHERE task_id='WT-A'`); err != nil {
		t.Fatalf("업무 삭제(멤버 정리 포함) 실패: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_task_members WHERE task_id='WT-A'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("삭제 후 잔여 members=%d", n)
	}
}

func TestWorkTaskAssigneeStaysOwnerSync(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "sync.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	wb := NewWBRepo(db)
	task := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "동기화", Assignee: "최혜영"}
	if err := wb.CreateTask(task); err != nil {
		t.Fatal(err)
	}
	assertOwnerMatches(t, db, task.TaskID, "최혜영")

	if err := wb.SetTaskAssignee(task.TaskID, "양기헌"); err != nil {
		t.Fatal(err)
	}
	assertOwnerMatches(t, db, task.TaskID, "양기헌")

	task.Assignee = "태자운"
	if err := wb.UpdateTask(task); err != nil {
		t.Fatal(err)
	}
	assertOwnerMatches(t, db, task.TaskID, "태자운")
}

func TestWorkTaskMembersStatsUnchangedAfterBackfill(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDB(filepath.Join(dir, "stats.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	seedWorkTaskMemberStats(t, db)

	beforeMembers := snapshotStatsScreen(t, db)
	assigneesBefore := dumpTaskAssignees(t, db)

	dropWorkTaskMemberTriggers(db)
	if _, err := db.Exec(`DROP TABLE IF EXISTS work_task_members`); err != nil {
		t.Fatal(err)
	}

	applyWorkTaskMembers(db)

	afterMigrate := snapshotStatsScreen(t, db)
	assigneesAfter := dumpTaskAssignees(t, db)

	if assigneesBefore != assigneesAfter {
		t.Fatalf("work_tasks.assignee 가 이관 중 바뀌었다\n전:\n%s\n후:\n%s", assigneesBefore, assigneesAfter)
	}
	if beforeMembers != afterMigrate {
		t.Fatalf("멤버 테이블 유무로 통계가 바뀌면 안 된다\n\n멤버 있음:\n%s\n\n이관 후:\n%s", beforeMembers, afterMigrate)
	}

	var tasks, members int
	_ = db.QueryRow(`SELECT COUNT(*) FROM work_tasks`).Scan(&tasks)
	_ = db.QueryRow(`SELECT COUNT(*) FROM work_task_members`).Scan(&members)
	if tasks != members || tasks == 0 {
		t.Fatalf("이관 건수 members=%d tasks=%d", members, tasks)
	}

	if _, err := db.Exec(`INSERT INTO work_task_members (task_id, assignee, member_role, duration_min, sort_order)
		SELECT task_id, '지원자', 'support', 15, 1 FROM work_tasks LIMIT 1`); err != nil {
		t.Fatal(err)
	}
	afterSupport := snapshotStatsScreen(t, db)
	if afterSupport != afterMigrate {
		t.Fatalf("지원자 추가 후 통계가 바뀌면 안 된다(이 단계는 주담당 집계)\n\n이관 후:\n%s\n\n지원 추가:\n%s", afterMigrate, afterSupport)
	}

	t.Logf("통계 화면 숫자 비교 (이관 전 = 이관 후)\n%s", afterMigrate)
}

func mustHaveAssigneeColumn(t *testing.T, db *sql.DB) {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info(work_tasks)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt interface{}
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		if name == "assignee" {
			found = true
		}
	}
	if !found {
		t.Fatal("work_tasks.assignee 컬럼이 없다")
	}
}

func mustAbortOwner(t *testing.T, err error) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), workTaskMembersOwnerMsg) {
		t.Fatalf("주담당 0/2명 저장을 거부해야 한다 err=%v", err)
	}
}

func assertOwnerMatches(t *testing.T, db *sql.DB, taskID, want string) {
	t.Helper()
	var taskAsg, memAsg, role string
	var n int
	if err := db.QueryRow(`SELECT COALESCE(assignee,''), (SELECT COUNT(*) FROM work_task_members WHERE task_id=?) FROM work_tasks WHERE task_id=?`, taskID, taskID).Scan(&taskAsg, &n); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT assignee, member_role FROM work_task_members WHERE task_id=? AND member_role='owner'`, taskID).Scan(&memAsg, &role); err != nil {
		t.Fatal(err)
	}
	if taskAsg != want || memAsg != want || role != model.WBMemberOwner {
		t.Fatalf("주담당 불일치 task=%q member=%q role=%q want=%q", taskAsg, memAsg, role, want)
	}
	if n < 1 {
		t.Fatalf("참여자 행이 없다 n=%d", n)
	}
}

func seedWorkTaskMemberStats(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES ('c1','도서관','도서관',1)`); err != nil {
		t.Fatal(err)
	}
	_, err := db.Exec(`
		INSERT INTO as_receipts (
			as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date,
			start_datetime, complete_datetime, status, assigned_to, data_origin
		) VALUES
		('a1','R1','c1','2026-08-10 09:00:00','2026-08-11','2026-08-11 10:00:00','2026-08-11 16:00:00','completed','최혜경','app'),
		('a2','R2','c1','2026-08-10 09:00:00','2026-08-11','2026-08-11 10:00:00','2026-08-11 16:00:00','completed','최혜경','app'),
		('a3','R3','c1','2026-08-10 09:00:00','2026-08-11','2026-08-11 10:00:00','2026-08-11 16:00:00','completed','최혜경','app'),
		('b1','R4','c1','2026-08-10 09:00:00','2026-08-12','2026-08-13 10:00:00','2026-08-13 16:00:00','completed','양기헌','app'),
		('u1','R5','c1','2026-08-10 09:00:00',NULL,'2026-08-11 10:00:00','2026-08-11 12:00:00','completed','','app'),
		('u2','R6','c1','2026-08-12 09:00:00',NULL,NULL,NULL,'in_progress','','app')`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO work_tasks (task_id, work_type, title, work_date, due_date, duration_min, status, assignee, complete_date, receipt_date)
		VALUES
		('T1','admin','행정-배정','2026-08-11','2026-08-11',30,'complete','양기헌','2026-08-11','2026-08-11'),
		('T2','admin','행정-미배정','2026-08-12','2026-08-12',30,'complete','','2026-08-12','2026-08-12')`); err != nil {
		t.Fatal(err)
	}
}

func dumpTaskAssignees(t *testing.T, db *sql.DB) string {
	t.Helper()
	rows, err := db.Query(`SELECT task_id, COALESCE(assignee,'') FROM work_tasks ORDER BY task_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var id, a string
		if err := rows.Scan(&id, &a); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&b, "%s=%q\n", id, a)
	}
	return b.String()
}

func snapshotStatsScreen(t *testing.T, db *sql.DB) string {
	t.Helper()
	repo := NewStatsRepo(db)
	anchor := time.Date(2026, 8, 12, 0, 0, 0, 0, time.Local)
	f := ParseMeetingFilter(model.StatsScopeTeam, "", "")
	cols := BuildStatsPeriodColumns(model.StatsViewWeek, anchor)
	if err := repo.FillPeriodOverview(cols, f); err != nil {
		t.Fatal(err)
	}
	kpi, err := repo.LoadStatsKPI(model.StatsViewWeek, cols, f)
	if err != nil {
		t.Fatal(err)
	}
	an, err := repo.LoadStatsWorkAnalysis(cols[1].From, cols[1].ToExclusive, f)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := repo.BuildWeeklyReport(anchor)
	if err != nil {
		t.Fatal(err)
	}

	cur := cols[1]
	var b strings.Builder
	fmt.Fprintf(&b, "[KPI] 방문=%.4f/%d 완료=%.4f/%d 계획률=%.4f (%d/%d)\n",
		kpi.VisitAvgDays, kpi.VisitSample, kpi.CompleteAvgDays, kpi.CompleteSample,
		kpi.PlanningRate, kpi.PlanningOpen, kpi.PlanningPlanned)
	fmt.Fprintf(&b, "[금주 %s] 예정=%d 접수=%d 처리=%d 변경=%d\n",
		cur.RangeLabel, cur.Counts.PlannedTotal(), cur.Counts.ReceiptTotal(), cur.Counts.ProcessTotal(),
		cur.Counts.ModifiedTotal())
	fmt.Fprintf(&b, "[분석] 접수=%d 방문=%d 완료=%d 이월입=%d 이월출=%d AS완료=%d 점검완료=%d 행정완료=%d\n",
		an.Receipt, an.Visit, an.Completed, an.CarryIn, an.CarryOut,
		an.AS.Completed, an.Mnt.Completed, an.Admin.Completed)
	fmt.Fprintf(&b, "[주간보고] 워킹데이=%d 계획률=%.4f\n", rep.WorkingDays, rep.PlanningRate)
	for _, p := range rep.PersonRows {
		kind := "담당"
		if p.IsTeam {
			kind = "팀"
		}
		if p.Unassigned {
			kind = "미배정"
		}
		fmt.Fprintf(&b, "  %s %s 접수=%d 완료=%d 이월=%d 일최대=%d %s\n",
			kind, p.Label, p.Receipt, p.Completed, p.CarryOut, p.DayMax, p.DayMaxDate)
	}
	return b.String()
}

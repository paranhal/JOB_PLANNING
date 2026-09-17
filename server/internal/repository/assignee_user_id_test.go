package repository

import (
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func TestAssigneeUserID_ColumnsAndUnmatchedReport(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "a50.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	for _, pair := range [][2]string{
		{"work_task_members", "user_id"},
		{"work_tasks", "assignee_user_id"},
		{"maintenance_visits", "assignee_user_id"},
		{"work_actions", "assignee_user_id"},
		{"as_processes", "worker_user_id"},
		{"work_activities", "actor_user_id"},
		{"as_receipts", "external_assignee"},
	} {
		if !tableHasColumn(db, pair[0], pair[1]) {
			t.Fatalf("050 열 없음 %s.%s", pair[0], pair[1])
		}
	}

	u := &model.User{Username: "choi", PasswordHash: "x", FullName: "최혜영", Role: model.RoleTech, IsActive: true}
	if err := NewUserRepo(db).Create(u); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c-ext','외부도서관','외부도서관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, status, assigned_to, symptom, data_origin
	) VALUES ('as-ext','R2609-EXT','c-ext','2026-09-01','assigned','최영성(앤로보틱스)','외부','app')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks (task_id, work_type, title, status, assignee)
		VALUES ('WT-STAFF','admin','직원업무','waiting','최혜영')`); err != nil {
		t.Fatal(err)
	}
	applyAssigneeUserIDs(db)

	var staffUID, extUID, extName string
	if err := db.QueryRow(`SELECT COALESCE(assignee_user_id,'') FROM work_tasks WHERE task_id='WT-STAFF'`).Scan(&staffUID); err != nil {
		t.Fatal(err)
	}
	if staffUID != u.UserID {
		t.Fatalf("직원 백필 user_id=%q want %q", staffUID, u.UserID)
	}
	if err := db.QueryRow(`SELECT COALESCE(assigned_user_id,''), COALESCE(external_assignee,'') FROM as_receipts WHERE as_id='as-ext'`).
		Scan(&extUID, &extName); err != nil {
		t.Fatal(err)
	}
	if extUID != "" {
		t.Fatalf("외부인을 users 에 맞추면 안 된다 assigned_user_id=%q", extUID)
	}
	if extName != "최영성(앤로보틱스)" {
		t.Fatalf("external_assignee=%q", extName)
	}

	found := false
	for _, it := range ListUnmatchedAssignees(db) {
		if it.Name == "최영성(앤로보틱스)" && it.Table == "as_receipts" {
			found = true
		}
		if it.Name == "최혜영" {
			t.Fatalf("직원 이름이 미매칭에 있으면 안 된다: %+v", it)
		}
	}
	if !found {
		t.Fatalf("외부인 미매칭 보고 없음: %+v", ListUnmatchedAssignees(db))
	}
}

func TestAssigneeUserID_DualWriteAndReadByID(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "a50w.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	u := &model.User{Username: "yang", PasswordHash: "x", FullName: "양기헌", Role: model.RoleTech, IsActive: true}
	if err := NewUserRepo(db).Create(u); err != nil {
		t.Fatal(err)
	}
	wb := NewWBRepo(db)
	task := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "이중저장", Status: model.WBTaskWaiting, Assignee: "양기헌"}
	if err := wb.CreateTask(task); err != nil {
		t.Fatal(err)
	}
	got, err := wb.GetTask(task.TaskID)
	if err != nil || got == nil {
		t.Fatalf("GetTask: %v %v", got, err)
	}
	if got.Assignee != "양기헌" || got.AssigneeUserID != u.UserID {
		t.Fatalf("쓰기 이름=%q id=%q want 양기헌/%s", got.Assignee, got.AssigneeUserID, u.UserID)
	}

	if err := wb.SetTaskAssignee(task.TaskID, "양기헌"); err != nil {
		t.Fatal(err)
	}
	var memUID, memName string
	if err := db.QueryRow(`SELECT COALESCE(user_id,''), COALESCE(assignee,'') FROM work_task_members WHERE task_id=? AND member_role='owner'`,
		task.TaskID).Scan(&memUID, &memName); err != nil {
		t.Fatal(err)
	}
	if memUID != u.UserID || memName != "양기헌" {
		t.Fatalf("멤버 dual-write name=%q id=%q", memName, memUID)
	}

	sql, args := assigneeMatchSQL(AssigneeKindTask, "t", u.UserID)
	if sql == "" || len(args) == 0 {
		t.Fatal("ID 읽기 SQL 이 비었다")
	}
	if !strings.Contains(sql, "assignee_user_id") || !strings.Contains(sql, "m.user_id") {
		t.Fatalf("읽기가 ID 열이 아니다: %s", sql)
	}
}

func TestRebuildWorkTaskMembers_AbortsOnCountMismatch(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "a50r.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	u := &model.User{Username: "choi", PasswordHash: "x", FullName: "최혜영", Role: model.RoleTech, IsActive: true}
	if err := NewUserRepo(db).Create(u); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks (task_id, work_type, title, status, assignee)
		VALUES ('WT-OK','admin','맞음','waiting','최혜영'),
		       ('WT-BAD','admin','외부','waiting','최영성(앤로보틱스)')`); err != nil {
		t.Fatal(err)
	}
	err = rebuildWorkTaskMembersByUserID(db)
	if err == nil {
		t.Fatal("매칭 실패가 있으면 이관이 멈추어야 한다")
	}
	if !strings.Contains(err.Error(), "이관 중단") {
		t.Fatalf("중단 문구 없음: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('work_task_members') WHERE name='assignee'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("중단 후 이름 열이 남아 있어야 한다 n=%d err=%v", n, err)
	}
}

func TestRebuildWorkTaskMembers_SucceedsWhenAllMatch(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "a50ok.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	u := &model.User{Username: "choi", PasswordHash: "x", FullName: "최혜영", Role: model.RoleTech, IsActive: true}
	if err := NewUserRepo(db).Create(u); err != nil {
		t.Fatal(err)
	}
	dropWorkTaskMemberTriggers(db)
	if _, err := db.Exec(`DELETE FROM work_task_members`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM work_tasks`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks (task_id, work_type, title, status, assignee)
		VALUES ('WT-1','admin','하나','waiting','최혜영')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_task_members (task_id, assignee, user_id, member_role, sort_order)
		VALUES ('WT-1','최혜영',?,'owner',0)`, u.UserID); err != nil {
		t.Fatal(err)
	}
	if err := rebuildWorkTaskMembersByUserID(db); err != nil {
		t.Fatal(err)
	}
	var after int
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_task_members`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != 1 {
		t.Fatalf("이관 후 %d행", after)
	}
	var nAssignee int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('work_task_members') WHERE name='assignee'`).Scan(&nAssignee); err != nil {
		t.Fatal(err)
	}
	if nAssignee != 0 {
		t.Fatal("성공 이관 후 assignee 열이 없어야 한다")
	}
}

func TestBindStaff_DoesNotInsertExternalIntoUsers(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "a50b.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	name, uid, ext := bindStaffExternal(db, "최영성(앤로보틱스)", "")
	if uid != "" || ext != "최영성(앤로보틱스)" || name != "최영성(앤로보틱스)" {
		t.Fatalf("name=%q uid=%q ext=%q", name, uid, ext)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE full_name LIKE '%최영성%'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("외부인을 users 에 넣으면 안 된다 n=%d err=%v", n, err)
	}
}

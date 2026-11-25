package handler

import (
	"fmt"
	"path/filepath"
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-222: 조직 칸반에서 AS 재방문·확인·지원 카드가 접수 ID 조회에 걸려 사라지지 않는다.
func TestKanban95_222_RevisitSurvivesOrgFilter(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "k222.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ex := func(q string, a ...interface{}) {
		if _, err := db.Exec(q, a...); err != nil {
			t.Fatal(q, err)
		}
	}
	users := repository.NewUserRepo(db)
	var uid = map[string]string{}
	for i, n := range []string{"박창훈", "박성준", "관리자O1"} {
		u := &model.User{Username: fmt.Sprintf("u222%d", i), PasswordHash: HashPassword("password10"), FullName: n, Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
		g, _ := users.GetByUsername(u.Username)
		uid[n] = g.UserID
	}
	ex(`INSERT INTO customers(customer_id,org_name,official_name,is_active,org_id) VALUES ('C222','시험기관','시험기관',1,?)`, model.OrgIDLibrary)
	ex(`INSERT INTO work_groups(group_id,org_id,name,status,created_by) VALUES ('WG-0001',?,'시험그룹','active',?)`, model.OrgIDLibrary, uid["관리자O1"])
	for _, n := range []string{"박창훈", "박성준"} {
		ex(`INSERT INTO work_group_members(group_id,user_id,role,joined_on,added_by) VALUES ('WG-0001',?,?, '2026-11-01',?)`, uid[n], map[bool]string{true: "leader", false: "member"}[n == "박창훈"], uid["관리자O1"])
	}
	ex(`INSERT INTO as_receipts(as_id,as_number,receipt_datetime,customer_id,symptom,urgency,status,visit_scheduled_date,schedule_confirmed,assigned_to,assigned_user_id,work_group_id,org_id,created_at,updated_at)
		VALUES ('AS-222','R2610-113','2026-10-20 10:00:00','C222','증상','중','partial_complete','2026-12-01',1,'박창훈',?,'WG-0001',?,'2026-10-20 10:00:00','2026-10-20 10:00:00')`, uid["박창훈"], model.OrgIDLibrary)
	ex(`INSERT INTO as_work_items(work_id,work_number,as_id,work_kind,scheduled_date,schedule_confirmed,assigned_to,assigned_user_id,status)
		VALUES ('W-222','R2610-113-W01','AS-222','revisit','2026-12-01',1,'박성준',?,'open')`, uid["박성준"])
	ex(`INSERT INTO as_work_item_assignees(work_id,user_id,assignee_name,role,work_group_id) VALUES ('W-222',?,'박성준','owner','WG-0001')`, uid["박성준"])
	ex(`INSERT INTO as_receipt_assignees(as_id,user_id,assignee_name,role,work_group_id) VALUES ('AS-222',?,'박창훈','owner','WG-0001')`, uid["박창훈"])

	h := New(db)
	date := "2026-12-01"
	for _, status := range []string{"partial_complete", "in_progress"} {
		ex(`UPDATE as_receipts SET status=? WHERE as_id='AS-222'`, status)
		for _, c := range []struct {
			filter string
			want   int
		}{{"", 1}, {"박성준", 1}, {"박창훈", 0}} {
			items, err := h.Workboard.collectRegisterKanbanItems(date, c.filter, model.OrgIDLibrary)
			if err != nil {
				t.Fatal(err)
			}
			_, n := h.Workboard.mapRegisterKanban(items, date, "2026-11-25", c.filter)
			want := c.want
			if status == "in_progress" && c.filter != "박성준" {
				want++ // 접수 카드(박창훈)도 함께 나온다
			}
			if status == "in_progress" && c.filter == "박창훈" {
				want = 1
			}
			if n != want {
				t.Errorf("status=%s filter=%q: 칸반 %d장, want %d", status, c.filter, n, want)
			}
		}
	}
}

// 95-222 ②③: 업무 멤버로만 참여한 사람의 칸반에도 업무가 나오고, 그룹 업무 카드에는 그룹명이 붙는다.
func TestKanban95_222_MemberTaskAndGroupBadge(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "k222m.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ex := func(q string, a ...interface{}) {
		if _, err := db.Exec(q, a...); err != nil {
			t.Fatal(q, err)
		}
	}
	users := repository.NewUserRepo(db)
	uid := map[string]string{}
	for i, n := range []string{"박창훈", "박성준", "관리자O1"} {
		u := &model.User{Username: fmt.Sprintf("m222%d", i), PasswordHash: HashPassword("password10"), FullName: n, Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
		g, _ := users.GetByUsername(u.Username)
		uid[n] = g.UserID
	}
	ex(`INSERT INTO work_groups(group_id,org_id,name,status,created_by) VALUES ('WG-0001',?,'시험그룹','active',?)`, model.OrgIDLibrary, uid["관리자O1"])
	ex(`INSERT INTO work_tasks (task_id, work_type, title, work_date, due_date, status, assignee, assignee_user_id, org_id, work_group_id)
		VALUES ('WT-222','admin','그룹 행정','2026-12-01','2026-12-01','planned','박창훈',?,?,'WG-0001')`, uid["박창훈"], model.OrgIDLibrary)
	ex(`INSERT INTO work_task_members (task_id, assignee, user_id, member_role, sort_order) VALUES ('WT-222','박성준',?,'support',1)`, uid["박성준"])

	h := New(db)
	date := "2026-12-01"
	for _, f := range []string{"", "박창훈", "박성준"} {
		items, err := h.Workboard.collectRegisterKanbanItems(date, f, model.OrgIDLibrary)
		if err != nil {
			t.Fatal(err)
		}
		cols, n := h.Workboard.mapRegisterKanban(items, date, "2026-11-25", f)
		if n != 1 {
			t.Errorf("filter=%q: 칸반 %d장, want 1", f, n)
			continue
		}
		for _, c := range cols {
			for _, k := range c.Items {
				if k.WorkGroupName != "시험그룹" {
					t.Errorf("filter=%q: 그룹명 %q, want 시험그룹", f, k.WorkGroupName)
				}
			}
		}
	}
}

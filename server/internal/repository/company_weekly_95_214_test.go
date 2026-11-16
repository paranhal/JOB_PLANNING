package repository

import (
	"strings"
	"testing"
)

// 95-214: 차주 이후 날짜의 일반 업무 계획도 (날짜)와 함께 계획에 넣는다.
// 정기점검 계획과 정해진 날 반복되는 업무(recurrence)는 차주 한 주 밖이면 넣지 않는다.
func TestCompanyWeekly95_214_PlanBeyondNextWeek(t *testing.T) {
	s := seed213(t)
	ex := func(q string, a ...interface{}) {
		if _, err := s.db.Exec(q, a...); err != nil {
			t.Fatal(err)
		}
	}
	// 마감일만 있는 업무(차주 이후), 끝난 업무, 취소 업무
	ex(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,due_date) VALUES
		('f1','admin','(f1)차주이후 마감','open','WP-A','O03','2026-12-05'),
		('f2','admin','(f2)끝난 먼 업무','complete','WP-A','O03','2026-12-06'),
		('f3','admin','(f3)취소 먼 업무','cancelled','WP-A','O03','2026-12-07')`)
	// 반복 업무: 상위(parent) 1건 + 실행(occurrence) 차주 1건 + 차주 이후 1건
	ex(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,recurrence_role) VALUES ('rp','admin','(rp)매주 반복','open','WP-A','O03','parent')`)
	ex(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,parent_task_id,recurrence_role,work_date) VALUES
		('r1','admin','(r1)반복 차주','open','WP-A','O03','rp','occurrence','2026-11-18'),
		('r2','admin','(r2)반복 차차주','open','WP-A','O03','rp','occurrence','2026-11-25')`)
	ex(`INSERT INTO work_actions(action_id,task_id,title,status,scheduled_date) VALUES ('wr','r2','반복 조치','todo','2026-11-26')`)
	plan := texts213(row213(t, s).AutoPlan)

	if !strings.Contains(plan, "(f1)차주이후 마감(12/5)") {
		t.Errorf("차주 이후 마감 업무가 날짜와 함께 계획에 있어야 함:\n%s", plan)
	}
	if strings.Contains(plan, "(f2)") || strings.Contains(plan, "(f3)") {
		t.Errorf("완료·취소 업무는 계획에 없다:\n%s", plan)
	}
	if !strings.Contains(plan, "(rp)매주 반복(11/18)") {
		t.Errorf("차주 안의 반복 업무는 종전대로 있다:\n%s", plan)
	}
	if strings.Contains(plan, "(r2)") || strings.Contains(plan, "반복 조치") {
		t.Errorf("차주 이후의 반복 업무는 계획에 넣지 않는다:\n%s", plan)
	}
	if strings.Count(plan, "(f1)차주이후 마감") != 1 {
		t.Errorf("한 업무는 한 번만:\n%s", plan)
	}
}

package repository

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func Test95_58_Migration094Columns(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "094.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, col := range []string{"action_start_at", "action_duration_min", "action_remark", "recurrence_possible", "customer_confirmed_at"} {
		if !tableHasColumn(db, "work_tasks", col) {
			t.Fatalf("컬럼 없음 %s", col)
		}
	}
	if _, err := db.Exec(`INSERT INTO work_tasks (task_id, work_type, title, status, assignee, duration_min, org_id)
		VALUES ('wt094','project_work','기존','waiting','관리자',30,'O01')`); err != nil {
		t.Fatal(err)
	}
	var start, remark, conf string
	var dur, rec int
	if err := db.QueryRow(`SELECT action_start_at, action_duration_min, action_remark, recurrence_possible, customer_confirmed_at
		FROM work_tasks WHERE task_id='wt094'`).Scan(&start, &dur, &remark, &rec, &conf); err != nil {
		t.Fatal(err)
	}
	if start != "" || dur != 0 || remark != "" || rec != 0 || conf != "" {
		t.Fatalf("기존 행 기본값 start=%q dur=%d remark=%q rec=%d conf=%q", start, dur, remark, rec, conf)
	}
	up, err := os.ReadFile(filepath.Join("..", "..", "migrations", "094_work_task_action_v58.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToUpper(string(up)), "INSERT INTO") && !strings.Contains(string(up), "시드 INSERT 없음") {
		t.Fatal("094 up 에 시드 INSERT")
	}
	down, err := os.ReadFile(filepath.Join("..", "..", "migrations", "094_work_task_action_v58.down.sql"))
	if err != nil || !strings.Contains(string(down), "DROP COLUMN") {
		t.Fatalf("down 없음 err=%v", err)
	}
	r := NewWBRepo(db)
	tk := &model.WorkTask{TaskID: "wt094", OrgID: "O01", Title: "기존", ActionStartAt: "2026-10-23T04:50", ActionDurationMin: 30}
	if err := r.LoadTaskActionFields(tk); err != nil {
		t.Fatal(err)
	}
}

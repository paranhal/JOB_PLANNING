package repository

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func Test95_78_Migration095NoBackfill(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "095.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO work_tasks (task_id, work_type, title, status, assignee, duration_min, org_id, start_time, end_time, work_date)
		VALUES ('wt095','admin','기존','waiting','관리자',30,'O01','09:00','18:00','2026-11-10')`); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_task_segments`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("운영 데이터 보정됨 n=%d", n)
	}
	up, err := os.ReadFile(filepath.Join("..", "..", "migrations", "095_work_task_segments.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if workTaskSegmentSQLHasSeedInsert(string(up)) {
		t.Fatal("095 up 에 INSERT")
	}
	down, err := os.ReadFile(filepath.Join("..", "..", "migrations", "095_work_task_segments.down.sql"))
	if err != nil || !strings.Contains(string(down), "DROP TABLE") {
		t.Fatalf("down 없음 err=%v", err)
	}
	r := NewWBRepo(db)
	t0, err := r.GetTask("wt095")
	if err != nil || t0 == nil {
		t.Fatal(err)
	}
	segs, err := r.ListPlanSegmentsOrVirtual(*t0)
	if err != nil || len(segs) != 1 || !segs[0].Virtual {
		t.Fatalf("가상 구간 %+v err=%v", segs, err)
	}
}

func Test95_78_PersonOccupancyCaseD(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "095d.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r := NewWBRepo(db)
	if err := r.CreateTask(&model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "OO", Status: model.WBTaskWaiting,
		Assignee: "김현우", WorkDate: "2026-11-10", DueDate: "2026-11-10",
		StartTime: "09:00", EndTime: "18:00", DurationMin: 540, OrgID: model.OrgIDLibrary,
	}); err != nil {
		t.Fatal(err)
	}
	hit, err := r.AssigneeTimeOverlaps("2026-11-10", "이다른", "10:00", "14:00", "")
	if err != nil {
		t.Fatal(err)
	}
	if hit {
		t.Fatal("작업자가 다른데 겹침")
	}
	hit, err = r.AssigneeTimeOverlaps("2026-11-10", "김현우", "10:00", "14:00", "")
	if err != nil || !hit {
		t.Fatalf("같은 작업자 겹침 실패 hit=%v err=%v", hit, err)
	}
}

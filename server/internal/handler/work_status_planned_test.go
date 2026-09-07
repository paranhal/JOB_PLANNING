package handler

import (
	"net/http"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func TestWorkStatusPlannedRedirectsToAction(t *testing.T) {
	e, db := newMntSyncServer(t, "ws_planned.db")

	seedVisit(t, db, "mvs_p1", "2026-08-14", "최혜영", "KLAS", "나성동도서관")
	if _, err := db.Exec(`UPDATE maintenance_visits SET completed=1, completed_date='2026-08-14' WHERE visit_id='mvs_p1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks
		(task_id, work_type, title, due_date, work_date, duration_min, status, priority, assignee, complete_date)
		VALUES ('WT-P4','admin','미완료 행정','2026-08-14','2026-08-14',30,'waiting','normal','최혜영',NULL)`); err != nil {
		t.Fatal(err)
	}

	rec := doGet(t, e, "/work-status?view=day&date=2026-08-14&kind=planned")
	if rec.Code != http.StatusFound {
		t.Fatalf("status=%d want 302", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "/work-status") || !strings.Contains(loc, "view=day") || !strings.Contains(loc, "date=2026-08-14") {
		t.Fatalf("redirect loc=%s", loc)
	}
	if strings.Contains(loc, "kind=planned") {
		t.Fatalf("planned 가 남아 있음: %s", loc)
	}

	done := doGet(t, e, "/work-status?view=day&date=2026-08-14&kind=action").Body.String()
	if !strings.Contains(done, "나성동도서관") {
		t.Fatal("조치 탭에 완료 점검이 없다")
	}
	if strings.Contains(done, "미완료 행정") || strings.Contains(done, "예정업무") {
		t.Fatal("예정 건이 조치에 남았다")
	}
}

func TestPlannedCardLabels(t *testing.T) {
	mnt := plannedCardFromWork(model.WorkTask{
		TaskID: "plan:mvs_1", WorkType: model.WBWorkMaintenance, SourceType: model.WBSourceMaintenance,
		SourceID: "mvs_1", Title: "[점검]나성동도서관_2026-08-14 · KLAS", Tags: "2026-08-14 · KLAS",
		Status: model.WBTaskWaiting, WorkDate: "2026-08-14",
		BoardHref: "/maintenance/visits/mvs_1/action",
	})
	if mnt.StatusLabel == "" || mnt.SourceNumber != "2026-08-14 · KLAS" {
		t.Fatalf("%+v", mnt)
	}
	if mnt.DetailHref() != "/maintenance/visits/mvs_1/action" {
		t.Fatalf("링크=%s", mnt.DetailHref())
	}

	as := plannedCardFromWork(model.WorkTask{
		TaskID: "WT-1", WorkType: model.WBWorkAS, SourceType: model.WBSourceAS,
		SourceID: "as1", Title: "[AS]도서관", Status: "in_progress", WorkDate: "2026-08-14",
	})
	if as.StatusLabel == "" || as.StatusLabel == "예정" {
		t.Fatalf("AS 상태 라벨=%q", as.StatusLabel)
	}
}

func TestWorkStatusFilterQueryOmitsPlanned(t *testing.T) {
	if strings.Contains(workStatusFilterQuery("", "", wsKindAction, ""), "kind=planned") {
		t.Fatal("조치 기본에 planned 가 들어가면 안 됨")
	}
}

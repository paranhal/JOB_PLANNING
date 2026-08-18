package handler

import (
	"net/http"
	"strings"
	"testing"

	"customer-support/internal/model"
)

// 업무처리현황「예정업무」탭 — 완료 전 업무와 아직 시간표에 없는 예정 건까지 모아 본다.
func TestWorkStatusPlannedTab(t *testing.T) {
	e, db := newMntSyncServer(t, "ws_planned.db")

	// 시간표에 올라간 미완료 점검
	seedVisit(t, db, "mvs_p1", "2026-08-14", "최혜영", "KLAS", "나성동도서관")
	seedVisitTask(t, db, "WT-P1", "mvs_p1", "2026-08-14", "09:00", "09:30", "최혜영",
		"[점검]나성동도서관_2026-08-14 · KLAS")

	// 아직 일일업무로 만들지 않은 점검(팔레트 대기)
	seedVisit(t, db, "mvs_p2", "2026-08-14", "양기헌", "KLAS", "보람동도서관")

	// 완료된 점검 — 예정업무에는 빠져야 한다
	seedVisit(t, db, "mvs_p3", "2026-08-14", "최혜영", "KLAS", "완료도서관")
	if _, err := db.Exec(`UPDATE maintenance_visits SET completed=1, completed_date='2026-08-14' WHERE visit_id='mvs_p3'`); err != nil {
		t.Fatal(err)
	}
	seedVisitTask(t, db, "WT-P3", "mvs_p3", "2026-08-14", "13:00", "13:30", "최혜영",
		"[점검]완료도서관_2026-08-14 · KLAS")

	// 미완료 행정업무
	if _, err := db.Exec(`INSERT INTO work_tasks
		(task_id, work_type, title, due_date, work_date, start_time, end_time, duration_min, status, priority, assignee)
		VALUES ('WT-P4','admin','미완료 행정','2026-08-14','2026-08-14','15:00','15:30',30,'waiting','normal','최혜영')`); err != nil {
		t.Fatal(err)
	}
	// 완료 행정업무
	if _, err := db.Exec(`INSERT INTO work_tasks
		(task_id, work_type, title, due_date, work_date, start_time, end_time, duration_min, status, priority, assignee)
		VALUES ('WT-P5','admin','행정 마감건','2026-08-14','2026-08-14','16:00','16:30',30,'complete','normal','최혜영')`); err != nil {
		t.Fatal(err)
	}

	rec := doGet(t, e, "/work-status?view=day&date=2026-08-14&kind=planned")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"나성동도서관", "보람동도서관", "미완료 행정", "예정업무"} {
		if !strings.Contains(body, want) {
			t.Fatalf("예정업무에 %q 없음", want)
		}
	}
	for _, deny := range []string{"완료도서관", "행정 마감건"} {
		if strings.Contains(body, deny) {
			t.Fatalf("완료 건 %q 가 예정업무에 포함됨", deny)
		}
	}

	// 조치 탭은 반대로 완료 건만
	done := doGet(t, e, "/work-status?view=day&date=2026-08-14&kind=action").Body.String()
	if !strings.Contains(done, "완료도서관") || strings.Contains(done, "미완료 행정") {
		t.Fatal("조치 탭 집계가 어긋남")
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

func TestWorkStatusFilterQueryPlanned(t *testing.T) {
	if !strings.Contains(workStatusFilterQuery("", "", wsKindPlanned), "kind=planned") {
		t.Fatal("예정업무 탭 전환 시 kind 유지 실패")
	}
}

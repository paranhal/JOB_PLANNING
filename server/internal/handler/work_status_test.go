package handler

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestEnsureTimelineDisplayTimesFillsMissing(t *testing.T) {
	tasks := []model.WorkTask{
		{TaskID: "a", WorkDate: "2026-08-11", StartTime: "10:00", EndTime: "10:30", DurationMin: 30, Title: "scheduled"},
		{TaskID: "b", DueDate: "2026-08-11", Title: "due-only", DurationMin: 30},
		{TaskID: "c", WorkDate: "2026-08-11", Title: "date-only", DurationMin: 30},
	}
	got := ensureTimelineDisplayTimes(tasks)
	if len(got) != 3 {
		t.Fatalf("len=%d", len(got))
	}
	for _, tsk := range got {
		if tsk.WorkDate != "2026-08-11" {
			t.Fatalf("work_date=%q id=%s", tsk.WorkDate, tsk.TaskID)
		}
		if tsk.StartTime == "" || tsk.EndTime == "" {
			t.Fatalf("missing time: %+v", tsk)
		}
	}
	if got[0].StartTime != "10:00" {
		t.Fatalf("kept start=%s", got[0].StartTime)
	}
	if got[1].StartTime == "10:00" || got[2].StartTime == "10:00" {
		t.Fatalf("display slots should avoid occupied 10:00: %s %s", got[1].StartTime, got[2].StartTime)
	}
}

func TestIsCompletedWorkTask(t *testing.T) {
	asOK := map[string]string{"a1": "completed", "a2": "partial_complete", "a3": "in_progress"}
	mnt := map[string]bool{"v1": true, "v2": false}

	if !isCompletedWorkTask(model.WorkTask{SourceType: model.WBSourceAS, SourceID: "a1"}, asOK, mnt) {
		t.Fatal("AS completed")
	}
	if !isCompletedWorkTask(model.WorkTask{SourceType: model.WBSourceAS, SourceID: "a2"}, asOK, mnt) {
		t.Fatal("AS partial")
	}
	if isCompletedWorkTask(model.WorkTask{SourceType: model.WBSourceAS, SourceID: "a3"}, asOK, mnt) {
		t.Fatal("AS open should exclude")
	}
	if !isCompletedWorkTask(model.WorkTask{SourceType: model.WBSourceMaintenance, SourceID: "v1"}, asOK, mnt) {
		t.Fatal("mnt done")
	}
	if isCompletedWorkTask(model.WorkTask{SourceType: model.WBSourceMaintenance, SourceID: "v2"}, asOK, mnt) {
		t.Fatal("mnt open")
	}
	if !isCompletedWorkTask(model.WorkTask{Status: model.WBTaskComplete}, asOK, mnt) {
		t.Fatal("admin complete")
	}
	if isCompletedWorkTask(model.WorkTask{Status: model.WBTaskWaiting}, asOK, mnt) {
		t.Fatal("admin waiting")
	}
}

func TestWorkStatusHTTP_TimelineSmoke(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "ws.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))

	day := time.Date(2026, 8, 11, 0, 0, 0, 0, time.Local).Format("2006-01-02")
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c1','현황도서관','현황도서관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date,
		schedule_confirmed, status, assigned_to, complete_datetime, symptom
	) VALUES ('as1','R2608-W01','c1',?,?,1,'completed','양기헌',?,'완료증상')`,
		day+" 09:00:00", day, day+" 11:00:00"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks (
		task_id, work_type, title, work_date, start_time, end_time, duration_min,
		status, assignee, source_type, source_id
	) VALUES ('WT-ws1','as','[AS]현황도서관',?,'10:00','10:30',30,'complete','양기헌','as','as1')`, day); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks (
		task_id, work_type, title, work_date, start_time, end_time, duration_min,
		status, assignee
	) VALUES ('WT-ws2','admin','미완료 행정',?,'11:00','11:30',30,'waiting','양기헌')`, day); err != nil {
		t.Fatal(err)
	}
	// 완료됐지만 시각 미기재 → 좌·우 모두 보여야 함
	if _, err := db.Exec(`INSERT INTO work_tasks (
		task_id, work_type, title, work_date, duration_min, status, assignee, complete_date
	) VALUES ('WT-ws3','admin','시각없는완료',?,30,'complete','양기헌',?)`, day, day); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/work-status", h.WorkStatus.Calendar)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/work-status?view=day&date="+day, nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		snippet := rec.Body.String()
		if len(snippet) > 500 {
			snippet = snippet[:500]
		}
		t.Fatalf("status=%d body=%s", rec.Code, snippet)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"일일 업무처리현황", "완료", "[AS]현황도서관", "10:00", "팀전체", "사업 전체", "시각없는완료",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q", want)
		}
	}
	if strings.Contains(body, "미완료 행정") {
		t.Fatal("미완료 업무가 실적에 포함되면 안 됨")
	}
	// 우측 팔레트·좌측 시간표에 동일 제목이 각각 존재해야 함(최소 2회)
	if strings.Count(body, "시각없는완료") < 2 {
		t.Fatal("좌·우에 동일 완료 건이 모두 보여야 함")
	}
	if strings.Contains(body, "일일 업무 수정") || strings.Contains(body, "title=\"조치 화면\"") {
		t.Fatal("조치/수정 버튼이 보이면 안 됨")
	}
	if !strings.Contains(body, "kind=receipt") || !strings.Contains(body, ">접수<") {
		t.Fatal("접수/조치 전환 버튼 없음")
	}
}

func TestWorkStatusFilterQueryIncludesKind(t *testing.T) {
	q := workStatusFilterQuery("양기헌", "WP1", wsKindReceipt)
	if !strings.Contains(q, "kind=receipt") || !strings.Contains(q, "assignee=") {
		t.Fatalf("q=%s", q)
	}
	if strings.Contains(workStatusFilterQuery("", "", wsKindAction), "kind=") {
		t.Fatal("action(default) should omit kind")
	}
}

func TestReceiptCardLabel(t *testing.T) {
	card := receiptCardFromWork(model.WorkTask{
		TaskID: "rcpt:a1", WorkType: model.WBWorkAS, SourceType: model.WBSourceAS,
		SourceID: "a1", Title: "[AS]도서관", Tags: "R2608-001", Status: "received",
		WorkDate: "2026-08-11", StartTime: "10:00", EndTime: "10:30",
		BoardHref: "/as/a1",
	})
	if card.StatusLabel != "접수" || card.SourceNumber != "R2608-001" {
		t.Fatalf("%+v", card)
	}
}

func TestWorkStatusPlannedURLPreservesFilters(t *testing.T) {
	got := registerURLWithProject("week", "2026-08-11", "양기헌", "WPSEED01")
	if !strings.Contains(got, "/workboard/register?") {
		t.Fatalf("base: %s", got)
	}
	if !strings.Contains(got, "view=week") || !strings.Contains(got, "date=2026-08-11") {
		t.Fatalf("view/date: %s", got)
	}
	if !strings.Contains(got, "assignee=") || !strings.Contains(got, "project=WPSEED01") {
		t.Fatalf("filters: %s", got)
	}
}

func TestStatusCardShowsLabelNotActions(t *testing.T) {
	card := statusCardFromWork(model.WorkTask{
		TaskID: "WT1", WorkType: model.WBWorkAS, SourceType: model.WBSourceAS,
		SourceID: "as1", Title: "[AS]테스트", Status: "partial_complete",
		WorkDate: "2026-08-11", StartTime: "09:00", EndTime: "09:30",
	})
	if card.StatusLabel != "부분완료" {
		t.Fatalf("label=%q", card.StatusLabel)
	}
	if card.DetailHref() == "" {
		t.Fatal("detail href empty")
	}
}

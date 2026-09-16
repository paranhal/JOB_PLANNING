package handler

import (
	"database/sql"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestASVisitDateCreatesDailyTaskAndRegisterPalette(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "as_daily.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))

	_, err = db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('C1','유구도서관','유구도서관',1)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, symptom, status, assigned_to, visit_scheduled_date, schedule_confirmed)
		VALUES ('R2608-036','R2608-036','C1','2026-08-10 10:00:00','장서점검 자료요청','in_progress','최혜영','',0)`)
	if err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.POST("/as/:id/visit-date", h.AS.UpdateVisitDate)
	g.GET("/workboard/register", h.Workboard.Register)

	rec := doForm(t, e, "/as/R2608-036/visit-date", url.Values{
		"visit_scheduled_date": {"2026-08-14"},
		"schedule_confirmed":   {"1"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("visit-date status=%d", rec.Code)
	}

	wb := repository.NewWBRepo(db)
	task, err := wb.GetTaskBySource(model.WBSourceAS, "R2608-036")
	if err != nil || task == nil {
		t.Fatalf("일일업무 미생성: %+v err=%v", task, err)
	}
	if task.DueDate != "2026-08-14" || task.WorkDate != "2026-08-14" {
		t.Fatalf("예정일 미반영: due=%s work=%s", task.DueDate, task.WorkDate)
	}
	if strings.TrimSpace(task.StartTime) != "" {
		t.Fatalf("미배치여야 함 start=%s", task.StartTime)
	}
	if !strings.Contains(task.Title, "R2608-036") {
		t.Fatalf("업무명: %s", task.Title)
	}

	// 예정일 날짜의 등록 화면 → 자동 배치되어 일정표에 나타남
	page := doGet(t, e, "/workboard/register?date=2026-08-14&view=day")
	if page.Code != http.StatusOK {
		t.Fatalf("register status=%d", page.Code)
	}
	body := page.Body.String()
	if !strings.Contains(body, "R2608-036") {
		t.Fatal("일일 업무 등록에 R2608-036 없음")
	}
	placed, err := wb.GetTaskBySource(model.WBSourceAS, "R2608-036")
	if err != nil || placed == nil || strings.TrimSpace(placed.StartTime) == "" {
		t.Fatalf("자동 배치 실패: %+v err=%v", placed, err)
	}
}

func newASDailyHTTP(t *testing.T) (*echo.Echo, *Handler, *sql.DB, *repository.WBRepo) {
	t.Helper()
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "as_daily_http.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('C1','유구도서관','유구도서관',1)`); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.POST("/as", h.AS.Create)
	g.GET("/as/:id", h.AS.Show)
	g.GET("/as", h.AS.List)
	g.POST("/as/:id/edit", h.AS.UpdateReceipt)
	g.POST("/as/:id/visit-date", h.AS.UpdateVisitDate)
	g.GET("/workboard/register", h.Workboard.Register)
	g.GET("/plan/unplanned", h.Work.UnplannedList)
	return e, h, db, repository.NewWBRepo(db)
}

func TestASAssignedWithoutDateAppearsUndatedOnRegister(t *testing.T) {
	e, _, db, wb := newASDailyHTTP(t)
	rec := doForm(t, e, "/as", url.Values{
		"customer_id":      {"C1"},
		"symptom":          {"장서점검 자료요청"},
		"received_by":      {"테스터"},
		"assigned_to":      {"최혜영"},
		"receipt_datetime": {"2026-08-10T10:00"},
	})
	if rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/as/") {
		t.Fatalf("접수 loc=%s status=%d", rec.Header().Get("Location"), rec.Code)
	}
	asID := strings.TrimPrefix(rec.Header().Get("Location"), "/as/")
	task, err := wb.GetTaskBySource(model.WBSourceAS, asID)
	if err != nil || task == nil {
		t.Fatalf("예정일 없이 일일업무가 없다: %+v err=%v", task, err)
	}
	if strings.TrimSpace(task.WorkDate) != "" || strings.TrimSpace(task.DueDate) != "" {
		t.Fatalf("날짜는 비워야 한다 due=%s work=%s", task.DueDate, task.WorkDate)
	}
	if task.Assignee != "최혜영" {
		t.Fatalf("담당자=%s", task.Assignee)
	}

	page := doGet(t, e, "/workboard/register?view=day")
	if page.Code != http.StatusOK {
		t.Fatalf("register status=%d", page.Code)
	}
	body := page.Body.String()
	if !strings.Contains(body, "날짜 미정") {
		t.Fatal("시간표 위 날짜 미정 줄이 없다")
	}
	if !strings.Contains(body, asID) && !strings.Contains(body, "유구도서관") {
		t.Fatal("날짜 미정 줄에 접수가 없다")
	}

	var asCount int
	_ = db.QueryRow(`SELECT COUNT(*) FROM work_tasks WHERE source_type='as'`).Scan(&asCount)
	show := doGet(t, e, "/as/"+asID)
	if show.Code != http.StatusOK {
		t.Fatalf("show status=%d", show.Code)
	}
	list := doGet(t, e, "/as")
	if list.Code != http.StatusOK {
		t.Fatalf("list status=%d", list.Code)
	}
	reg := doGet(t, e, "/workboard/register?view=day")
	if reg.Code != http.StatusOK {
		t.Fatalf("register GET status=%d", reg.Code)
	}
	var asCount2 int
	_ = db.QueryRow(`SELECT COUNT(*) FROM work_tasks WHERE source_type='as'`).Scan(&asCount2)
	if asCount2 != asCount {
		t.Fatalf("조회 화면이 AS 일일업무를 바꿨다 %d → %d", asCount, asCount2)
	}
}

func TestASUnassignedStaysInUnplannedNotDaily(t *testing.T) {
	e, _, _, wb := newASDailyHTTP(t)
	rec := doForm(t, e, "/as", url.Values{
		"customer_id":      {"C1"},
		"symptom":          {"미배정 증상"},
		"received_by":      {"테스터"},
		"receipt_datetime": {"2026-08-10T10:00"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create status=%d", rec.Code)
	}
	asID := strings.TrimPrefix(rec.Header().Get("Location"), "/as/")
	task, err := wb.GetTaskBySource(model.WBSourceAS, asID)
	if err != nil || task != nil {
		t.Fatalf("미배정은 일일업무에 있으면 안 된다: %+v", task)
	}
	page := doGet(t, e, "/plan/unplanned")
	if page.Code != http.StatusOK {
		t.Fatalf("unplanned status=%d", page.Code)
	}
	if !strings.Contains(page.Body.String(), asID) && !strings.Contains(page.Body.String(), "미배정 증상") {
		t.Fatal("미계획 업무함에 미배정 접수가 없다")
	}
	reg := doGet(t, e, "/workboard/register?view=day")
	if strings.Contains(reg.Body.String(), "미배정 증상") {
		t.Fatal("미배정 건이 일일 업무 화면에 보인다")
	}
}

func TestASAssigneeChangeFollowsUntilManual(t *testing.T) {
	e, h, _, wb := newASDailyHTTP(t)
	rec := doForm(t, e, "/as", url.Values{
		"customer_id":      {"C1"},
		"symptom":          {"담당자 변경"},
		"received_by":      {"테스터"},
		"assigned_to":      {"양기헌"},
		"receipt_datetime": {"2026-08-10T10:00"},
	})
	asID := strings.TrimPrefix(rec.Header().Get("Location"), "/as/")
	as, err := h.AS.repo.GetByID(asID)
	if err != nil || as == nil {
		t.Fatal(err)
	}
	as.AssignedTo = "태자운"
	if err := h.AS.repo.UpdateReceipt(as); err != nil {
		t.Fatal(err)
	}
	h.AS.syncASPlannedDailyTask(asID)
	task, _ := wb.GetTaskBySource(model.WBSourceAS, asID)
	if task == nil || task.Assignee != "태자운" {
		t.Fatalf("AS 담당자 변경이 일일업무에 안 따라감: %+v", task)
	}
	if err := wb.SetTaskAssignee(task.TaskID, "최혜영"); err != nil {
		t.Fatal(err)
	}
	as.AssignedTo = "양기헌"
	if err := h.AS.repo.UpdateReceipt(as); err != nil {
		t.Fatal(err)
	}
	h.AS.syncASPlannedDailyTask(asID)
	task, _ = wb.GetTaskBySource(model.WBSourceAS, asID)
	if task == nil || task.Assignee != "최혜영" {
		t.Fatalf("일일 업무에서 손으로 바꾼 담당자가 덮였다: %+v", task)
	}
}


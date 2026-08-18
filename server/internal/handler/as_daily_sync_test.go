package handler

import (
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

	// 예정일 날짜의 등록 화면 → 자동 배치되어 시간표에 나타남
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

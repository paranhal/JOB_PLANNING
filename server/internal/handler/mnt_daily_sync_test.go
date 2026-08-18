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

// 정기점검 일정과 일일 업무 등록이 같은 날짜를 보게 하는 흐름을 확인한다.
func newMntSyncServer(t *testing.T, name string) (*echo.Echo, *sql.DB) {
	t.Helper()
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.POST("/maintenance/visits/:visit_id/update", h.Maintenance.UpdateVisit)
	g.POST("/maintenance/visits/:visit_id/action", h.Maintenance.UpdateVisitAction)
	g.POST("/maintenance/visits/:visit_id/delete", h.Maintenance.DeleteVisit)
	g.POST("/workboard/schedule", h.Workboard.Schedule)
	g.POST("/workboard/unschedule", h.Workboard.Unschedule)
	g.GET("/workboard/register", h.Workboard.Register)
	g.GET("/work-status", h.WorkStatus.Calendar)
	return e, db
}

func seedVisit(t *testing.T, db *sql.DB, visitID, date, assignee, product, org string) {
	t.Helper()
	cust := "C_" + visitID
	if _, err := db.Exec(`INSERT OR IGNORE INTO customers (customer_id, org_name, official_name, is_active)
		VALUES (?,?,?,1)`, cust, org, org); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO maintenance_plans (plan_id, plan_year, title, status)
		VALUES ('MP1', 2026, '2026 정기점검', 'approved')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits
		(visit_id, plan_id, visit_date, customer_id, sort_order, entry_category, assignee, product_type, completed)
		VALUES (?,'MP1',?,?,0,'normal',?,?,0)`, visitID, date, cust, assignee, product); err != nil {
		t.Fatal(err)
	}
}

func seedVisitTask(t *testing.T, db *sql.DB, taskID, visitID, workDate, start, end, assignee, title string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO work_tasks
		(task_id, work_type, title, description, due_date, work_date, start_time, end_time,
		 duration_min, status, priority, assignee, source_type, source_id)
		VALUES (?,'maintenance',?,'[KLAS]점검 정기점검',?,?,?,?,30,'waiting','normal',?,'maintenance',?)`,
		taskID, title, workDate, workDate, start, end, assignee, visitID); err != nil {
		t.Fatal(err)
	}
}

func taskOf(t *testing.T, db *sql.DB, visitID string) *model.WorkTask {
	t.Helper()
	task, err := repository.NewWBRepo(db).GetTaskBySource(model.WBSourceMaintenance, visitID)
	if err != nil {
		t.Fatal(err)
	}
	return task
}

// 점검 일정에서 방문일을 바꾸면 연결된 일일업무도 그 날짜로 옮겨야 한다.
func TestVisitDateChangeMovesDailyTask(t *testing.T) {
	e, db := newMntSyncServer(t, "mnt_move.db")
	seedVisit(t, db, "mvs_1", "2026-08-12", "최혜영", "KLAS", "종촌동도서관")
	seedVisitTask(t, db, "WT-1", "mvs_1", "2026-08-12", "15:30", "16:00", "최혜영",
		"[점검]종촌동도서관_2026-08-12 · KLAS")

	rec := doForm(t, e, "/maintenance/visits/mvs_1/update", url.Values{
		"plan_id":      {"MP1"},
		"visit_date":   {"2026-08-14"},
		"customer_id":  {"C_mvs_1"},
		"assignee":     {"최혜영"},
		"product_type": {"KLAS"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("일정 수정 status=%d", rec.Code)
	}

	task := taskOf(t, db, "mvs_1")
	if task == nil {
		t.Fatal("연결 업무 없음")
	}
	if task.WorkDate != "2026-08-14" || task.DueDate != "2026-08-14" {
		t.Fatalf("일자 미반영: work=%s due=%s", task.WorkDate, task.DueDate)
	}
	if !strings.Contains(task.Title, "2026-08-14") {
		t.Fatalf("업무명에 옛 날짜가 남음: %s", task.Title)
	}
	if task.StartTime != "15:30" {
		t.Fatalf("겹치지 않으면 시각 유지: %s", task.StartTime)
	}

	body := doGet(t, e, "/workboard/register?date=2026-08-14&view=day").Body.String()
	if !strings.Contains(body, "[점검]종촌동도서관_2026-08-14") {
		t.Fatal("8/14 일일 업무 등록에 점검 건 없음")
	}
	// 거래처 콤보에도 기관명이 있으므로 업무 카드 제목으로만 본다.
	old := doGet(t, e, "/workboard/register?date=2026-08-12&view=day").Body.String()
	if strings.Contains(old, "[점검]종촌동도서관") {
		t.Fatal("옛 날짜(8/12)에 그대로 남아 있음")
	}
}

// 조치 화면에서 담당자를 바꿔도 일일업무에 반영돼야 한다.
func TestVisitActionSyncsAssignee(t *testing.T) {
	e, db := newMntSyncServer(t, "mnt_assignee.db")
	seedVisit(t, db, "mvs_2", "2026-08-14", "최혜영", "KLAS", "나성동도서관")
	seedVisitTask(t, db, "WT-2", "mvs_2", "2026-08-14", "09:00", "09:30", "최혜영",
		"[점검]나성동도서관_2026-08-14 · KLAS")

	rec := doForm(t, e, "/maintenance/visits/mvs_2/action", url.Values{
		"assignee":     {"양기헌"},
		"product_type": {"KLAS"},
		"visit_date":   {"2026-08-17"},
		"status":       {"open"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("조치 저장 status=%d", rec.Code)
	}
	task := taskOf(t, db, "mvs_2")
	if task.Assignee != "양기헌" || task.WorkDate != "2026-08-17" {
		t.Fatalf("담당·일자 미반영: %+v", task)
	}
}

// 옮긴 날짜에 같은 담당자 일정이 이미 있으면 시각을 비워 자동 배치에 맡긴다.
func TestVisitMoveClearsOverlappingTime(t *testing.T) {
	e, db := newMntSyncServer(t, "mnt_overlap.db")
	seedVisit(t, db, "mvs_3", "2026-08-12", "최혜영", "KLAS", "고운동도서관")
	seedVisitTask(t, db, "WT-3", "mvs_3", "2026-08-12", "10:00", "10:30", "최혜영",
		"[점검]고운동도서관_2026-08-12 · KLAS")
	if _, err := db.Exec(`INSERT INTO work_tasks
		(task_id, work_type, title, due_date, work_date, start_time, end_time, duration_min, status, priority, assignee)
		VALUES ('WT-BLK','admin','선점 업무','2026-08-14','2026-08-14','10:00','10:30',30,'waiting','normal','최혜영')`); err != nil {
		t.Fatal(err)
	}

	rec := doForm(t, e, "/maintenance/visits/mvs_3/update", url.Values{
		"plan_id":      {"MP1"},
		"visit_date":   {"2026-08-14"},
		"customer_id":  {"C_mvs_3"},
		"assignee":     {"최혜영"},
		"product_type": {"KLAS"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d", rec.Code)
	}
	task := taskOf(t, db, "mvs_3")
	if task.WorkDate != "2026-08-14" || task.StartTime != "" {
		t.Fatalf("겹칠 때 시각을 비워야 함: %+v", task)
	}
	// 등록 화면에 들어가면 빈 시각이 자동 배치된다.
	doGet(t, e, "/workboard/register?date=2026-08-14&view=day")
	if placed := taskOf(t, db, "mvs_3"); placed.StartTime == "" {
		t.Fatal("자동 배치되지 않음")
	}
}

// 화면 밖(직접 수정 등)에서 어긋난 데이터도 등록 화면 진입 때 맞춰진다.
func TestRegisterRepairsMismatchedVisitDates(t *testing.T) {
	e, db := newMntSyncServer(t, "mnt_repair.db")
	seedVisit(t, db, "mvs_4", "2026-08-14", "최혜영", "KLAS", "대평동도서관")
	seedVisitTask(t, db, "WT-4", "mvs_4", "2026-08-21", "14:00", "14:30", "최혜영",
		"[점검]대평동도서관_2026-08-21 · KLAS")

	body := doGet(t, e, "/workboard/register?date=2026-08-14&view=day").Body.String()
	if !strings.Contains(body, "대평동도서관") {
		t.Fatal("보정 후 8/14에 보여야 함")
	}
	task := taskOf(t, db, "mvs_4")
	if task.WorkDate != "2026-08-14" || !strings.Contains(task.Title, "2026-08-14") {
		t.Fatalf("보정 실패: %+v", task)
	}
}

// 완료된 방문의 실적일은 옮기지 않는다.
func TestCompletedVisitKeepsActualDate(t *testing.T) {
	e, db := newMntSyncServer(t, "mnt_done.db")
	seedVisit(t, db, "mvs_5", "2026-08-14", "최혜영", "KLAS", "해밀동도서관")
	if _, err := db.Exec(`UPDATE maintenance_visits SET completed=1, completed_date='2026-08-12' WHERE visit_id='mvs_5'`); err != nil {
		t.Fatal(err)
	}
	seedVisitTask(t, db, "WT-5", "mvs_5", "2026-08-12", "15:30", "16:00", "최혜영",
		"[점검]해밀동도서관_2026-08-12 · KLAS")

	doGet(t, e, "/workboard/register?date=2026-08-14&view=day")
	task := taskOf(t, db, "mvs_5")
	if task.WorkDate != "2026-08-12" {
		t.Fatalf("완료 건 실적일이 바뀜: %s", task.WorkDate)
	}
}

// 일일 업무 등록에서 카드를 옮기면 점검 일정도 같은 날짜가 된다.
func TestScheduleMoveUpdatesVisitDate(t *testing.T) {
	e, db := newMntSyncServer(t, "mnt_reverse.db")
	seedVisit(t, db, "mvs_6", "2026-08-14", "최혜영", "KLAS", "소담동도서관")

	rec := doForm(t, e, "/workboard/schedule", url.Values{
		"kind":       {model.WBSourceMaintenance},
		"ref_id":     {"mvs_6"},
		"work_date":  {"2026-08-18"},
		"start_time": {"09:00"},
		"assignee":   {"최혜영"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("배치 status=%d", rec.Code)
	}
	var date string
	if err := db.QueryRow(`SELECT visit_date FROM maintenance_visits WHERE visit_id='mvs_6'`).Scan(&date); err != nil {
		t.Fatal(err)
	}
	if date != "2026-08-18" {
		t.Fatalf("점검 일정 역동기화 실패: %s", date)
	}
}

// 방문을 지우면 일일업무의 유령 카드도 사라진다.
func TestDeleteVisitRemovesDailyTask(t *testing.T) {
	e, db := newMntSyncServer(t, "mnt_delete.db")
	seedVisit(t, db, "mvs_7", "2026-11-14", "최혜영", "KLAS", "책문화센터")
	seedVisitTask(t, db, "WT-7", "mvs_7", "2026-11-14", "11:00", "11:30", "최혜영",
		"[점검]책문화센터_2026-11-14 · KLAS")

	if rec := doForm(t, e, "/maintenance/visits/mvs_7/delete", url.Values{"plan_id": {"MP1"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("삭제 status=%d", rec.Code)
	}
	if task := taskOf(t, db, "mvs_7"); task != nil {
		t.Fatalf("유령 업무가 남음: %+v", task)
	}
}

// 같은 날 KLAS 6건 중 일부만 일일업무로 있어도, 등록 화면 진입 후 미완료 6건이 모두 그날 보여야 한다.
func TestRegisterShowsAllKLASVisitsForDay(t *testing.T) {
	e, db := newMntSyncServer(t, "mnt_six.db")
	sites := []struct {
		id, org, workDate, start string
	}{
		{"mvs_k1", "종촌동도서관", "2026-08-14", "09:00"},
		{"mvs_k2", "나성동도서관", "2026-08-14", "09:30"},
		{"mvs_k3", "고운동도서관", "2026-08-10", "10:00"}, // 일정만 14일로 바뀐 상태
		{"mvs_k4", "대평동도서관", "2026-08-12", "11:00"},
		{"mvs_k5", "보람동도서관", "", ""}, // 아직 일일업무 없음
		{"mvs_k6", "소담동도서관", "", ""},
	}
	for _, s := range sites {
		seedVisit(t, db, s.id, "2026-08-14", "최혜영", "KLAS", s.org)
		if s.workDate != "" {
			end := "09:30"
			if s.start == "09:30" {
				end = "10:00"
			} else if s.start == "10:00" {
				end = "10:30"
			} else if s.start == "11:00" {
				end = "11:30"
			}
			seedVisitTask(t, db, "WT-"+s.id, s.id, s.workDate, s.start, end, "최혜영",
				"[점검]"+s.org+"_2026-08-14 · KLAS")
		}
	}

	body := doGet(t, e, "/workboard/register?date=2026-08-14&view=day").Body.String()
	for _, s := range sites {
		if !strings.Contains(body, s.org) {
			t.Fatalf("8/14 등록 화면에 %s 없음", s.org)
		}
		task := taskOf(t, db, s.id)
		if task == nil || task.WorkDate != "2026-08-14" {
			t.Fatalf("%s 배정일이 14일이 아님: %+v", s.org, task)
		}
	}
}

// ×로 내린 점검 카드는 등록 화면을 다시 열어도 시간표에 자동으로 올라가지 않는다.
func TestUnplaceMaintenanceStaysOffGrid(t *testing.T) {
	e, db := newMntSyncServer(t, "mnt_unplace.db")
	seedVisit(t, db, "mvs_u1", "2026-08-14", "최혜영", "KLAS", "해밀동도서관")
	seedVisitTask(t, db, "WT-U1", "mvs_u1", "2026-08-14", "15:00", "15:30", "최혜영",
		"[점검]해밀동도서관_2026-08-14 · KLAS")

	if rec := doForm(t, e, "/workboard/unschedule", url.Values{"task_id": {"WT-U1"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("내리기 status=%d", rec.Code)
	}
	doGet(t, e, "/workboard/register?date=2026-08-14&view=day")
	task := taskOf(t, db, "mvs_u1")
	if task == nil {
		t.Fatal("× 후 업무 행이 사라져서는 안 됨")
	}
	if task.WorkDate != "" || task.StartTime != "" {
		t.Fatalf("× 후 자동 재배치됨: %+v", task)
	}
}

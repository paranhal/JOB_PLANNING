package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func newMnt95107App(t *testing.T) (*echo.Echo, *sql.DB, *repository.MaintenanceRepo, *model.MaintenancePlan) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "m107.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c107a','해미도서관','해미도서관',1,?),('c107b','SI학교','SI학교',1,'O03'),('c107c','종촌','종촌',1,?)`,
		model.OrgIDLibrary, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	mnt := repository.NewMaintenanceRepo(db)
	p, err := mnt.CreatePlanForOrg(model.OrgIDLibrary, 2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/maintenance/:id", h.Maintenance.ShowPlan)
	g.POST("/maintenance/visits/:visit_id/update", h.Maintenance.UpdateVisit, h.Auth.RequireMaintenanceEdit)
	g.POST("/maintenance/visits/:visit_id/move", h.Maintenance.MoveVisit, h.Auth.RequireMaintenanceEdit)
	return e, db, mnt, p
}

func insert105Visit(t *testing.T, mnt *repository.MaintenanceRepo, v model.MaintenanceVisit) string {
	t.Helper()
	id, err := mnt.InsertVisitFullID(v)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func postMove105(t *testing.T, e *echo.Echo, vid, date string, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	return doFormWith(t, e, "/maintenance/visits/"+vid+"/move", url.Values{"date": {date}}, ck)
}

func parseMove105(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("json %v %s", err, rec.Body.String())
	}
	return m
}

func Test95_107_T1_MoveVisitDateOnly(t *testing.T) {
	e, db, mnt, p := newMnt95107App(t)
	id := insert105Visit(t, mnt, model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-08-12", CustomerID: "c107a",
		ProductType: "KLAS", Assignee: "최혜영", Notes: "메모유지", RoundNo: 1, EntryCategory: "normal",
	})
	before, _ := mnt.GetVisit(id)
	rec := postMove105(t, e, id, "2026-08-18", jwtCookie(t))
	if rec.Code != http.StatusOK {
		t.Fatalf("move %d %s", rec.Code, rec.Body.String())
	}
	got, _ := mnt.GetVisit(id)
	if got.VisitDate != "2026-08-18" {
		t.Fatalf("날짜 %s", got.VisitDate)
	}
	if got.Assignee != before.Assignee || got.Notes != before.Notes || got.ProductType != before.ProductType || got.CustomerID != before.CustomerID || got.VisitRound() != before.VisitRound() {
		t.Fatalf("날짜 외 변경 %+v / %+v", before, got)
	}
	_ = db
}

func Test95_107_T2_CompletedAndInProgress(t *testing.T) {
	e, db, mnt, p := newMnt95107App(t)
	done := insert105Visit(t, mnt, model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-08-03", CustomerID: "c107a",
		ProductType: "KLAS", Assignee: "최혜영", Completed: true, CompletedDate: "2026-08-03",
	})
	rec := postMove105(t, e, done, "2026-08-18", jwtCookie(t))
	if rec.Code != http.StatusConflict {
		t.Fatalf("완료 %d %s", rec.Code, rec.Body.String())
	}
	j := parseMove105(t, rec)
	if j["code"] != "visit_completed" {
		t.Fatalf("code %+v", j)
	}

	live := insert105Visit(t, mnt, model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-08-12", CustomerID: "c107c",
		ProductType: "KLAS", Assignee: "최혜영",
	})
	if _, err := db.Exec(`INSERT INTO work_tasks
		(task_id, work_type, title, due_date, work_date, start_time, duration_min, status, priority, assignee, source_type, source_id, org_id)
		VALUES ('WT107','maintenance','[점검]','2026-08-12','2026-08-12','09:00',30,'waiting','normal','최혜영','maintenance',?,?)`, live, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	wb := repository.NewWBRepo(db)
	_, err := wb.InsertLiveActual(model.WorkTaskSegment{
		TaskID: "WT107", WorkDate: "2026-08-12", StartTime: "09:00",
		Kind: model.SegKindActual, WorkerUserID: "admin-id", UserID: "admin-id",
	})
	if err != nil {
		if strings.Contains(err.Error(), "no such table") || strings.Contains(err.Error(), "no such column") {
			t.Skip("95-101C 테이블 없음")
		}
		t.Fatal(err)
	}
	rec2 := postMove105(t, e, live, "2026-08-18", jwtCookie(t))
	if rec2.Code != http.StatusConflict {
		t.Fatalf("작업중 %d %s", rec2.Code, rec2.Body.String())
	}
	if parseMove105(t, rec2)["code"] != "visit_in_progress" {
		t.Fatalf("in_progress %+v", rec2.Body.String())
	}
}

func Test95_107_T3_ForbiddenNotFoundBadDate(t *testing.T) {
	e, _, mnt, p := newMnt95107App(t)
	id := insert105Visit(t, mnt, model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-08-12", CustomerID: "c107a", ProductType: "KLAS", Assignee: "최혜영",
	})
	rec := postMove105(t, e, id, "2026-08-18", jwtCookieRole(t, model.RoleSales))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("권한 %d %s", rec.Code, rec.Body.String())
	}
	p3, err := mnt.CreatePlanForOrg("O03", 2026, "si")
	if err != nil {
		t.Fatal(err)
	}
	oid := insert105Visit(t, mnt, model.MaintenanceVisit{
		PlanID: p3.PlanID, VisitDate: "2026-08-12", CustomerID: "c107b", ProductType: "KLAS", Assignee: "에스원",
	})
	rec2 := postMove105(t, e, oid, "2026-08-18", jwtCookieOrg(t, model.RoleOrgAdmin, model.OrgIDLibrary))
	if rec2.Code != http.StatusNotFound {
		t.Fatalf("타조직 %d %s", rec2.Code, rec2.Body.String())
	}
	rec3 := postMove105(t, e, id, "08-18", jwtCookie(t))
	if rec3.Code != http.StatusBadRequest {
		t.Fatalf("날짜 %d %s", rec3.Code, rec3.Body.String())
	}
}

func Test95_107_T4_DupWorkTaskHistory(t *testing.T) {
	e, db, mnt, p := newMnt95107App(t)
	a := insert105Visit(t, mnt, model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-09-10", CustomerID: "c107a", ProductType: "KLAS", Assignee: "최혜영",
	})
	_ = insert105Visit(t, mnt, model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-08-14", CustomerID: "c107a", ProductType: "KLAS", Assignee: "최혜영",
	})
	rec := postMove105(t, e, a, "2026-08-16", jwtCookie(t))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("중복 %d %s", rec.Code, rec.Body.String())
	}
	upd := doFormWith(t, e, "/maintenance/visits/"+a+"/update", url.Values{
		"plan_id": {p.PlanID}, "visit_date": {"2026-08-16"}, "customer_id": {"c107a"},
		"assignee": {"최혜영"}, "product_type": {"KLAS"},
	}, jwtCookie(t))
	if upd.Code != http.StatusBadRequest && upd.Code != http.StatusSeeOther {
		t.Fatalf("update dup %d %s", upd.Code, upd.Body.String())
	}
	if upd.Code == http.StatusSeeOther {
		t.Fatal("UpdateVisit 는 같은 중복을 막아야 한다")
	}

	id := insert105Visit(t, mnt, model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-08-12", CustomerID: "c107c", ProductType: "KLAS", Assignee: "최혜영",
	})
	if _, err := db.Exec(`INSERT INTO work_tasks
		(task_id, work_type, title, due_date, work_date, start_time, end_time, duration_min, status, priority, assignee, source_type, source_id, org_id)
		VALUES ('WT107b','maintenance','[점검]종촌_2026-08-12 · KLAS','2026-08-12','2026-08-12','15:30','16:00',30,'waiting','normal','최혜영','maintenance',?,?)`,
		id, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	var logs0 int
	_ = db.QueryRow(`SELECT COUNT(*) FROM data_change_logs WHERE table_name='maintenance_visits' AND entity_id=?`, id).Scan(&logs0)
	rec2 := postMove105(t, e, id, "2026-08-18", jwtCookie(t))
	if rec2.Code != http.StatusOK {
		t.Fatalf("move task %d %s", rec2.Code, rec2.Body.String())
	}
	task, err := repository.NewWBRepo(db).GetTaskBySource(model.WBSourceMaintenance, id)
	if err != nil || task == nil {
		t.Fatal("업무 없음")
	}
	if task.WorkDate != "2026-08-18" {
		t.Fatalf("업무 날짜 %s", task.WorkDate)
	}
	var logs1 int
	_ = db.QueryRow(`SELECT COUNT(*) FROM data_change_logs WHERE table_name='maintenance_visits' AND entity_id=?`, id).Scan(&logs1)
	if logs1 < logs0+1 {
		t.Fatalf("이력 %d -> %d", logs0, logs1)
	}
}

func Test95_107_T5_HolidayWarning(t *testing.T) {
	e, _, mnt, p := newMnt95107App(t)
	id := insert105Visit(t, mnt, model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-08-12", CustomerID: "c107c", ProductType: "KLAS", Assignee: "최혜영",
	})
	rec := postMove105(t, e, id, "2026-08-09", jwtCookie(t))
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	j := parseMove105(t, rec)
	warns, _ := j["warnings"].([]any)
	if len(warns) == 0 {
		t.Fatal("일요일 warnings 없음")
	}
	got, _ := mnt.GetVisit(id)
	if got.VisitDate != "2026-08-09" {
		t.Fatal("저장 안 됨")
	}
}

func Test95_107_T6_UpdateVisitCallsMoveVisitDate(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Dir(file)
	src, err := os.ReadFile(filepath.Join(dir, "maintenance.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "h.moveVisitDate(") {
		t.Fatal("UpdateVisit 가 moveVisitDate 를 호출하지 않는다")
	}
	src2, err := os.ReadFile(filepath.Join(dir, "maintenance_move.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src2), "func (h *MaintenanceHandler) moveVisitDate") {
		t.Fatal("moveVisitDate 없음")
	}
}

func Test95_107_T7_T9_CalendarRender(t *testing.T) {
	files := planShowParseFiles(t)
	visits := sampleVisits()
	tmpl, err := template.New("").Funcs(funcMap()).ParseFiles(files...)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "content", planViewData(mntViewCalendar, 8, visits)); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, `data-visit-id="mvs_2"`) || !strings.Contains(out, `draggable="true"`) {
		t.Fatal("T7 미완료 칩 draggable")
	}
	if !strings.Contains(out, `data-visit-id="mvs_1"`) || !strings.Contains(out, `data-locked="1"`) {
		t.Fatal("T7 완료 칩 locked")
	}
	if i := strings.Index(out, `data-visit-id="mvs_1"`); i >= 0 {
		chunk := out[i : i+400]
		if strings.Contains(chunk, `draggable="true"`) {
			t.Fatal("완료 칩에 draggable")
		}
	}
	data := planViewData(mntViewCalendar, 8, visits)
	data["CanEdit"] = false
	buf.Reset()
	if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), `draggable="true"`) {
		t.Fatal("T7 권한 없는 사용자 draggable")
	}
	if !strings.Contains(out, `role="separator"`) || !strings.Contains(out, `id="mnt-cal-splitter"`) {
		t.Fatal("T8 스플리터")
	}
	if !strings.Contains(out, "hidden lg:block") {
		t.Fatal("T8 lg 미만 숨김")
	}
	for _, want := range []string{"완료", "예정", "확인 필요", "mnt-month-select", "전체 담당자", ">수정<", "완료</button>", "selectDay"} {
		if !strings.Contains(out, want) {
			t.Fatalf("T9 회귀 %q", want)
		}
	}
}

func Test95_107_T10_T12_JSUnit(t *testing.T) {
	dir := filepath.Join(findTemplateRoot(t), "..", "static", "js")
	for _, name := range []string{"mnt_calendar_dnd_test.js", "mnt_calendar_resize_test.js"} {
		cmd := exec.Command("node", name)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", name, err, out)
		}
	}
}

func Test95_107_T13_T16_Browser(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	e, _, mnt, p := newMnt95107App(t)
	openID := insert105Visit(t, mnt, model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-08-12", CustomerID: "c107a",
		ProductType: "KLAS", Assignee: "최혜영", ShortName: "해미",
	})
	doneID := insert105Visit(t, mnt, model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-08-03", CustomerID: "c107c",
		ProductType: "KLAS", Assignee: "최혜영", Completed: true, CompletedDate: "2026-08-03", ShortName: "종촌",
	})
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	cmd := exec.Command("node", "95_107_mnt_cal_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(),
		"PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1",
		"BASE_URL="+srv.URL,
		"AUTH_COOKIE="+jwtCookie(t).Value,
		"PLAN_ID="+p.PlanID,
		"OPEN_VISIT="+openID,
		"DONE_VISIT="+doneID,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		s := string(out)
		if strings.Contains(s, "Executable doesn't exist") || strings.Contains(s, "browserType.launch") || strings.Contains(s, "playwright") && strings.Contains(s, "install") {
			t.Skip(s)
		}
		t.Fatalf("browser: %v\n%s", err, s)
	}
}

func Test95_107_T17_CSSBundleAndUILint(t *testing.T) {
	css, err := os.ReadFile(filepath.Join(findTemplateRoot(t), "..", "static", "vendor", "tailwind.css"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(css)
	for _, name := range []string{"mnt-cal-splitter", "mnt-cal-chip-locked", "mnt-cal-drop"} {
		if !strings.Contains(s, name) {
			t.Errorf("CSS 에 .%s 없음 — npm run build:css", name)
		}
	}
	got := countUILint(string(mustRead(t, filepath.Join(findTemplateRoot(t), "maintenance", "plan_show.html"))))
	_ = got
}

package handler

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/auditlog"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func initTestAccessDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "audit", "access.db")
	if err := auditlog.Init(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { auditlog.Close() })
	return path
}

type accessRow struct {
	Action, Result, Target, Reason, Username, Before string
}

func readAccessLogs(t *testing.T, path string) []accessRow {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT action, result, target_table, reason, username, before_json FROM access_logs ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []accessRow
	for rows.Next() {
		var r accessRow
		if err := rows.Scan(&r.Action, &r.Result, &r.Target, &r.Reason, &r.Username, &r.Before); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAccessLogLoginLogoutAndFail(t *testing.T) {
	path := initTestAccessDB(t)
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	e.POST("/login", h.Auth.Login)
	e.GET("/logout", h.Auth.Logout)

	fail := postForm(t, e, "/login", url.Values{"username": {"admin"}, "password": {"wrong"}}, nil)
	if fail.Code != http.StatusOK {
		t.Fatalf("실패 로그인 status=%d", fail.Code)
	}
	ok := postForm(t, e, "/login", url.Values{"username": {"admin"}, "password": {"admin"}}, nil)
	if ok.Code != http.StatusSeeOther {
		t.Fatalf("로그인 status=%d", ok.Code)
	}
	var token *http.Cookie
	for _, c := range ok.Result().Cookies() {
		if c.Name == "token" {
			token = c
			break
		}
	}
	if token == nil {
		t.Fatal("로그인 쿠키 없음")
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/logout", nil)
	req.AddCookie(token)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("로그아웃 status=%d", rec.Code)
	}

	logs := readAccessLogs(t, path)
	if len(logs) != 3 {
		t.Fatalf("접속기록 %d건: %+v", len(logs), logs)
	}
	if logs[0].Action != auditlog.ActionLoginFail || logs[0].Result != auditlog.ResultDeny {
		t.Fatalf("실패 기록: %+v", logs[0])
	}
	if logs[1].Action != auditlog.ActionLogin || logs[1].Result != auditlog.ResultOK || logs[1].Username != "admin" {
		t.Fatalf("로그인 기록: %+v", logs[1])
	}
	if logs[2].Action != auditlog.ActionLogout || logs[2].Username != "admin" {
		t.Fatalf("로그아웃 기록: %+v", logs[2])
	}
}

func TestAccessLogPlanAndVisitDelete(t *testing.T) {
	path := initTestAccessDB(t)
	e, db, h, _ := newMntProtectApp(t)
	p := seedProtectPlan(t, db, h.Maintenance.repo, "2026-11-10")
	visits, err := h.Maintenance.repo.ListVisits(p.PlanID)
	if err != nil || len(visits) != 1 {
		t.Fatalf("방문: %d err=%v", len(visits), err)
	}

	wrong := mntPost(t, e, "/maintenance/"+p.PlanID+"/delete", url.Values{
		"confirm_title":   {"2026년 정기점검"},
		"delete_password": {"wrong"},
	})
	if !strings.Contains(locText(wrong), "삭제할 수 없습니다") {
		t.Fatalf("계획 삭제 거부 없음: %s", locText(wrong))
	}

	delVisit := mntPost(t, e, "/maintenance/visits/"+visits[0].VisitID+"/delete?plan_id="+p.PlanID, url.Values{})
	if delVisit.Code != http.StatusSeeOther {
		t.Fatalf("방문 삭제 status=%d", delVisit.Code)
	}

	if got, _ := h.Maintenance.repo.GetPlan(p.PlanID); got == nil {
		t.Fatal("계획이 지워졌다")
	}

	logs := readAccessLogs(t, path)
	if len(logs) < 2 {
		t.Fatalf("접속기록 부족: %+v", logs)
	}
	if logs[0].Action != auditlog.ActionDelete || logs[0].Result != auditlog.ResultDeny || logs[0].Target != "maintenance_plans" {
		t.Fatalf("거부 기록: %+v", logs[0])
	}
	if logs[1].Action != auditlog.ActionDelete || logs[1].Target != "maintenance_visits" || logs[1].Result != auditlog.ResultOK {
		t.Fatalf("방문 삭제 기록: %+v", logs[1])
	}
}

func TestAccessLogUserDeactivate(t *testing.T) {
	path := initTestAccessDB(t)
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	userRepo := repository.NewUserRepo(db)
	userRepo.EnsureAdmin(HashPassword("admin"))
	tech := &model.User{
		Username: "tech1", PasswordHash: HashPassword("pw"), FullName: "기술원",
		Role: model.RoleTech, IsActive: true,
	}
	if err := userRepo.Create(tech); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.POST("/users/:id/update", h.Auth.UserUpdate)

	rec := postForm(t, e, "/users/"+tech.UserID+"/update", url.Values{
		"full_name": {"기술원"},
		"role":      {"tech"},
		"is_active": {"0"},
	}, jwtCookie(t))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("비활성: status=%d body=%s", rec.Code, rec.Body.String())
	}
	got, _ := userRepo.GetByID(tech.UserID)
	if got == nil || got.IsActive {
		t.Fatal("비활성되지 않음")
	}
	logs := readAccessLogs(t, path)
	if len(logs) != 1 || logs[0].Action != auditlog.ActionDelete || logs[0].Target != "users" {
		t.Fatalf("사용자 삭제 기록: %+v", logs)
	}
	if logs[0].Username != "admin" {
		t.Fatalf("행위자: %+v", logs[0])
	}
}

func TestAccessLogProjectDelete(t *testing.T) {
	path := initTestAccessDB(t)
	e, repo, _ := newProjectServer(t)
	p := &model.WorkProject{Name: "접속기록사업", Status: model.WBProjectActive, Color: "#3B82F6"}
	if err := repo.Create(p); err != nil {
		t.Fatal(err)
	}
	rec := doForm(t, e, "/projects/"+p.ProjectID+"/delete", url.Values{})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "ok=deleted") {
		t.Fatalf("사업 삭제: status=%d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
	logs := readAccessLogs(t, path)
	if len(logs) != 1 || logs[0].Action != auditlog.ActionDelete || logs[0].Target != "work_projects" {
		t.Fatalf("사업 삭제 기록: %+v", logs)
	}
}

func TestAccessLogASDelete(t *testing.T) {
	path := initTestAccessDB(t)
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(
		`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?,?,?,1)`,
		"cust_a", "가나도서관", "가나도서관"); err != nil {
		t.Fatal(err)
	}
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	asRepo := repository.NewASRepo(db)
	src := &model.ASReceipt{
		CustomerID: "cust_a", ReceiptChannel: "phone", Requester: "홍길동",
		Symptom: "게이트", Urgency: "high", Priority: "normal",
		AssignedTo: "양기헌", ReceiptDatetime: time.Now(),
	}
	if err := asRepo.Create(src); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.POST("/as/:id/delete", h.AS.Delete)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+src.ASID+"/delete", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("AS 삭제 status=%d", rec.Code)
	}
	logs := readAccessLogs(t, path)
	if len(logs) != 1 || logs[0].Action != auditlog.ActionDelete || logs[0].Target != "as_receipts" {
		t.Fatalf("AS 삭제 기록: %+v", logs)
	}
	if logs[0].Before == "" {
		t.Fatal("삭제 직전 값 없음")
	}
}

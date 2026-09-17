package handler

import (
	"bytes"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func jwtCookieRole(t *testing.T, role string) *http.Cookie {
	t.Helper()
	secret := []byte("cs-system-jwt-secret-2026")
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": role + "-id", "username": role, "role": role,
		"name": role, "exp": time.Now().Add(time.Hour).Unix(),
	})
	s, err := token.SignedString(secret)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: "token", Value: s, Path: "/"}
}

func newMntManageApp(t *testing.T) (*echo.Echo, *repository.MaintenanceRepo, *model.MaintenancePlan) {
	t.Helper()
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "manage.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c1','가나도서관','가나도서관',1)`); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewMaintenanceRepo(db)
	if err := repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "c1", ShortName: "가나", Region: "세종", HasKlas: true, InspectionCycle: "monthly",
	}); err != nil {
		t.Fatal(err)
	}
	p, err := repo.CreatePlan(2026, "2026년 정기점검")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertVisitFull(model.MaintenanceVisit{
		PlanID: p.PlanID, VisitDate: "2026-11-10", CustomerID: "c1", ProductType: "KLAS", Assignee: "최혜영",
	}); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	adminOnly := h.Auth.RequireAdminMW
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/maintenance/:id/manage", h.Maintenance.ManagePlan)
	g.POST("/maintenance/:id/copy", h.Maintenance.CopyPlan, adminOnly)
	g.POST("/maintenance/:id/status", h.Maintenance.SetPlanStatus, adminOnly)
	g.POST("/maintenance/:id/bulk-assignee", h.Maintenance.BulkUpdateAssignees, adminOnly)
	g.POST("/maintenance/:id/bulk-delete", h.Maintenance.BulkDeleteVisits, adminOnly)
	g.POST("/maintenance/:id/assign-slots", h.Maintenance.AssignUnassignedSlots, adminOnly)
	g.POST("/maintenance", h.Maintenance.CreatePlan, adminOnly)
	return e, repo, p
}

func manageGet(t *testing.T, e *echo.Echo, path string, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost"+path, nil)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	return rec
}

func managePost(t *testing.T, e *echo.Echo, path string, form url.Values, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost"+path, strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	return rec
}

func TestManagePlanPageAndCopy(t *testing.T) {
	e, repo, p := newMntManageApp(t)
	rec := manageGet(t, e, "/maintenance/"+p.PlanID+"/manage", jwtCookie(t))
	if rec.Code != http.StatusOK {
		t.Fatalf("관리 화면 status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"연도 계획", "계획 복사", "방문 일괄 담당자", "방문 일괄 삭제", "미배정 일괄 배정", "2026"} {
		if !strings.Contains(body, want) {
			t.Fatalf("%q 없음", want)
		}
	}

	tech := manageGet(t, e, "/maintenance/"+p.PlanID+"/manage", jwtCookieRole(t, "tech"))
	if tech.Code != http.StatusOK {
		t.Fatalf("기술담당 조회 status=%d", tech.Code)
	}
	if strings.Contains(tech.Body.String(), "계획 복사") {
		t.Fatal("기술담당에게 복사 폼이 보인다")
	}

	deny := managePost(t, e, "/maintenance/"+p.PlanID+"/copy", url.Values{"plan_year": {"2027"}}, jwtCookieRole(t, "tech"))
	if deny.Code != http.StatusForbidden {
		t.Fatalf("기술담당 복사 거부 status=%d", deny.Code)
	}
	if plans, _ := repo.ListPlans(); len(plans) != 1 {
		t.Fatalf("기술담당이 계획을 만듦: %d", len(plans))
	}

	ok := managePost(t, e, "/maintenance/"+p.PlanID+"/copy", url.Values{"plan_year": {"2027"}}, jwtCookie(t))
	if ok.Code != http.StatusSeeOther || !strings.Contains(ok.Header().Get("Location"), "/manage") {
		t.Fatalf("복사 loc=%s status=%d", ok.Header().Get("Location"), ok.Code)
	}
	plans, _ := repo.ListPlans()
	if len(plans) != 2 {
		t.Fatalf("복사 후 계획 수=%d", len(plans))
	}
}

func TestManageBulkDeleteRequiresConfirm(t *testing.T) {
	e, repo, p := newMntManageApp(t)
	skip := managePost(t, e, "/maintenance/"+p.PlanID+"/bulk-delete", url.Values{"month": {"11"}}, jwtCookie(t))
	if skip.Code != http.StatusSeeOther || !strings.Contains(skip.Header().Get("Location"), "preview=delete") {
		t.Fatalf("미리보기 없이 삭제: loc=%s", skip.Header().Get("Location"))
	}
	left, _ := repo.ListVisits(p.PlanID)
	if len(model.DatedVisits(left)) != 1 {
		t.Fatalf("확인 전에 삭제됨: %+v", left)
	}

	preview := manageGet(t, e, "/maintenance/"+p.PlanID+"/manage?preview=delete&month=11", jwtCookie(t))
	if !strings.Contains(preview.Body.String(), "삭제 가능") {
		t.Fatalf("건수 확인 없음: %s", preview.Body.String())
	}

	done := managePost(t, e, "/maintenance/"+p.PlanID+"/bulk-delete", url.Values{
		"confirm": {"1"}, "month": {"11"},
	}, jwtCookie(t))
	if done.Code != http.StatusSeeOther {
		t.Fatalf("삭제 status=%d", done.Code)
	}
	left, _ = repo.ListVisits(p.PlanID)
	if len(model.DatedVisits(left)) != 0 {
		t.Fatalf("미래 방문이 남음: %+v", left)
	}
}

func TestManageSetStatusAndCreateDuplicateYear(t *testing.T) {
	e, repo, p := newMntManageApp(t)
	rec := managePost(t, e, "/maintenance/"+p.PlanID+"/status", url.Values{"status": {"approved"}}, jwtCookie(t))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("상태 status=%d", rec.Code)
	}
	got, _ := repo.GetPlan(p.PlanID)
	if got.Status != "approved" {
		t.Fatalf("status=%s", got.Status)
	}

	dup := managePost(t, e, "/maintenance", url.Values{
		"plan_year": {"2026"}, "next": {"manage"}, "from_plan_id": {p.PlanID},
	}, jwtCookie(t))
	loc, _ := url.QueryUnescape(dup.Header().Get("Location"))
	if !strings.Contains(loc, "이미") {
		t.Fatalf("중복 연도 안내: %s", loc)
	}
}

func TestManagePlanRender(t *testing.T) {
	root := findTemplateRoot(t)
	files := []string{
		filepath.Join(root, "layout", "base.html"),
		filepath.Join(root, "maintenance", "plan_manage.html"),
	}
	tmpl, err := template.New("").Funcs(funcMap()).ParseFiles(files...)
	if err != nil {
		t.Fatal(err)
	}
	data := map[string]interface{}{
		"Title": "정기점검 관리", "Active": NavMaintenance, "UserRole": "admin",
		"Plan": &model.MaintenancePlan{PlanID: "mpl_1", PlanYear: 2026, Status: "draft", Title: "2026년 정기점검"},
		"Plans": []repository.PlanListItem{
			{MaintenancePlan: model.MaintenancePlan{PlanID: "mpl_1", PlanYear: 2026, Status: "draft", Title: "t"}, VisitCount: 2, DoneCount: 1},
		},
		"IsAdmin": true, "Regions": []string{"세종"}, "Assignees": []model.User{{FullName: "최혜영"}},
		"Unassigned":      []model.UnassignedMonthSlot{{CustomerID: "c1", ShortName: "가나", ProductType: "KLAS", Cycle: "monthly"}},
		"UnassignedMonth": 8, "DupCount": 0, "FlashOK": "", "FlashErr": "",
		"Preview": nil, "PreviewN": 0, "PreviewSkip": 0,
		"FilterMonth": "", "FilterRegion": "", "FilterProduct": "",
		"NewYear": 2027, "CopyYear": 2027,
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "미배정 일괄 배정") {
		t.Fatal("미배정 구역 없음")
	}
}

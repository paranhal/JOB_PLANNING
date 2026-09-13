package handler

import (
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

func jwtCookiePerms(t *testing.T, role string, perms ...string) *http.Cookie {
	t.Helper()
	secret := []byte("cs-system-jwt-secret-2026")
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":     role + "-id",
		"username":    role,
		"role":        role,
		"name":        role,
		"permissions": model.FormatPermissions(perms),
		"verified_at": time.Now().Unix(),
		"exp":         time.Now().Add(time.Hour).Unix(),
	})
	s, err := token.SignedString(secret)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: "token", Value: s, Path: "/"}
}

func newSalesASPermApp(t *testing.T) (*echo.Echo, *repository.UserRepo, *repository.ASRepo, string) {
	t.Helper()
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "sales_as_perm.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	userRepo := repository.NewUserRepo(db)
	userRepo.EnsureAdmin(HashPassword("admin"))
	if _, err := db.Exec(
		`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?,?,?,1)`,
		"cust_a", "가나도서관", "가나도서관"); err != nil {
		t.Fatal(err)
	}
	asRepo := repository.NewASRepo(db)
	src := &model.ASReceipt{
		CustomerID: "cust_a", ReceiptChannel: "phone", Requester: "홍길동",
		Symptom: "게이트 오작동", Urgency: "high", Priority: "normal",
		AssignedTo: "양기헌", ReceiptDatetime: time.Now(),
		Status: "in_progress",
	}
	if err := asRepo.Create(src); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.Use(h.Auth.RequireActiveRole)
	receiveAS := h.Auth.RequireReceiveAS
	processAS := h.Auth.RequireProcessAS
	g.GET("/", h.Dashboard)
	g.GET("/sales", h.Sales.List)
	g.GET("/users", h.Auth.UserList)
	g.POST("/users/:id/update", h.Auth.UserUpdate)
	g.GET("/as", h.AS.List)
	g.GET("/as/new", h.AS.New, receiveAS)
	g.POST("/as", h.AS.Create, receiveAS)
	g.GET("/as/:id", h.AS.Show)
	g.GET("/as/:id/action", h.AS.Action)
	g.POST("/as/:id/update", h.AS.Update, processAS)
	g.POST("/as/:id/process", h.AS.AddProcess, processAS)
	return e, userRepo, asRepo, src.ASID
}

func salesASGet(t *testing.T, e *echo.Echo, path string, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost"+path, nil)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	return rec
}

func salesASPost(t *testing.T, e *echo.Echo, path string, form url.Values, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost"+path, strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	return rec
}

func hasASMenu(body string) bool {
	return strings.Contains(body, "AS 접수·조치")
}

func TestSalesWithoutASPermsUnchanged(t *testing.T) {
	e, _, _, asID := newSalesASPermApp(t)
	ck := jwtCookieRole(t, "sales")

	home := salesASGet(t, e, "/sales", ck)
	if home.Code != http.StatusOK {
		t.Fatalf("영업 홈 status=%d", home.Code)
	}
	if hasASMenu(home.Body.String()) {
		t.Fatal("권한 없는 영업담당에게 AS 메뉴가 보인다")
	}

	if rec := salesASGet(t, e, "/as/new", ck); rec.Code != http.StatusForbidden {
		t.Fatalf("접수 화면: status=%d want 403", rec.Code)
	}
	if rec := salesASPost(t, e, "/as", url.Values{
		"customer_id": {"cust_a"}, "symptom": {"게이트"}, "received_by": {"영업"},
		"receipt_datetime": {time.Now().Format("2006-01-02T15:04")},
	}, ck); rec.Code != http.StatusForbidden {
		t.Fatalf("접수 POST: status=%d want 403", rec.Code)
	}
	if rec := salesASPost(t, e, "/as/"+asID+"/process", url.Values{"work_content": {"조치"}}, ck); rec.Code != http.StatusForbidden {
		t.Fatalf("조치 POST: status=%d want 403", rec.Code)
	}
	if rec := salesASPost(t, e, "/as/"+asID+"/update", url.Values{"status": {"in_progress"}}, ck); rec.Code != http.StatusForbidden {
		t.Fatalf("조치 저장 POST: status=%d want 403", rec.Code)
	}
}

func TestSalesReceiveOnlyCanReceiveButNotProcess(t *testing.T) {
	e, _, _, asID := newSalesASPermApp(t)
	ck := jwtCookiePerms(t, "sales", model.PermAnalysis, model.PermStats, model.PermASReceive)

	home := salesASGet(t, e, "/sales", ck)
	if home.Code != http.StatusOK {
		t.Fatalf("영업 홈 status=%d", home.Code)
	}
	if !hasASMenu(home.Body.String()) {
		t.Fatal("접수 권한을 받은 영업담당에게 AS 메뉴가 없다")
	}
	if strings.Contains(home.Body.String(), "오늘 내 업무") {
		t.Fatal("AS 권한만으로 오늘 내 업무 메뉴가 열리면 안 된다")
	}

	newPage := salesASGet(t, e, "/as/new", ck)
	if newPage.Code != http.StatusOK {
		t.Fatalf("접수 화면: status=%d", newPage.Code)
	}

	created := salesASPost(t, e, "/as", url.Values{
		"customer_id": {"cust_a"}, "symptom": {"영업 접수"}, "received_by": {"영업"},
		"receipt_datetime": {time.Now().Format("2006-01-02T15:04")},
	}, ck)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("접수 POST: status=%d body=%s", created.Code, created.Body.String())
	}

	show := salesASGet(t, e, "/as/"+asID, ck)
	if show.Code != http.StatusOK {
		t.Fatalf("상세: status=%d", show.Code)
	}
	if strings.Contains(show.Body.String(), "/as/"+asID+"/action") {
		t.Fatal("접수만 받은 사람에게 조치 버튼이 있다")
	}

	if rec := salesASPost(t, e, "/as/"+asID+"/process", url.Values{"work_content": {"조치"}}, ck); rec.Code != http.StatusForbidden {
		t.Fatalf("조치 POST: status=%d want 403", rec.Code)
	}
	if rec := salesASPost(t, e, "/as/"+asID+"/update", url.Values{
		"status": {"in_progress"}, "action_taken": {"현장"}, "time_spent": {"30"},
	}, ck); rec.Code != http.StatusForbidden {
		t.Fatalf("조치 저장 POST: status=%d want 403", rec.Code)
	}
}

func TestSalesWithReceiveAndProcessCanUseAS(t *testing.T) {
	e, _, _, asID := newSalesASPermApp(t)
	ck := jwtCookiePerms(t, "sales", model.PermAnalysis, model.PermStats, model.PermASReceive, model.PermASProcess)

	home := salesASGet(t, e, "/sales", ck)
	if home.Code != http.StatusOK {
		t.Fatalf("영업 홈 status=%d", home.Code)
	}
	if !hasASMenu(home.Body.String()) {
		t.Fatal("권한을 받은 영업담당에게 AS 메뉴가 없다")
	}

	if rec := salesASGet(t, e, "/as/new", ck); rec.Code != http.StatusOK {
		t.Fatalf("접수 화면: status=%d", rec.Code)
	}

	show := salesASGet(t, e, "/as/"+asID, ck)
	if show.Code != http.StatusOK {
		t.Fatalf("상세: status=%d", show.Code)
	}
	if !strings.Contains(show.Body.String(), "/as/"+asID+"/action") {
		t.Fatal("조치 권한이 있는데 조치 버튼이 없다")
	}

	proc := salesASPost(t, e, "/as/"+asID+"/process", url.Values{
		"work_content": {"현장 조치"}, "action_short_ok": {"1"},
	}, ck)
	if proc.Code == http.StatusForbidden {
		t.Fatal("조치 POST 가 403 이다")
	}
	upd := salesASPost(t, e, "/as/"+asID+"/update", url.Values{
		"status": {"in_progress"}, "work_place": {"field"}, "process_type": {"visit"},
		"cause_type": {"hw"}, "action_taken": {"현장 점검"}, "time_spent": {"30"},
		"action_short_ok": {"1"},
	}, ck)
	if upd.Code == http.StatusForbidden {
		t.Fatal("조치 저장 POST 가 403 이다")
	}
}

func TestSalesAccountCanGrantReceiveSeparately(t *testing.T) {
	e, userRepo, _, _ := newSalesASPermApp(t)
	u := &model.User{
		Username: "sales_tech", PasswordHash: HashPassword("x"),
		FullName: "기술영업", Role: model.RoleSales,
		Permissions: "analysis,stats", IsActive: true,
	}
	if err := userRepo.Create(u); err != nil {
		t.Fatal(err)
	}

	rec := salesASPost(t, e, "/users/"+u.UserID+"/update", url.Values{
		"full_name": {"기술영업"}, "role": {model.RoleSales}, "is_active": {"1"},
		"perm": {model.PermAnalysis, model.PermStats, model.PermASReceive},
	}, jwtCookie(t))
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusOK {
		t.Fatalf("사용자 저장: status=%d body=%s", rec.Code, rec.Body.String())
	}

	got, err := userRepo.GetByID(u.UserID)
	if err != nil || got == nil {
		t.Fatalf("조회: %v", err)
	}
	if !got.HasPerm(model.PermASReceive) {
		t.Fatalf("as_receive 가 저장되지 않았다: %q", got.Permissions)
	}
	if got.HasPerm(model.PermASProcess) {
		t.Fatalf("as_process 까지 켜졌다: %q", got.Permissions)
	}

	users := salesASGet(t, e, "/users", jwtCookie(t))
	if users.Code != http.StatusOK {
		t.Fatalf("사용자 관리: status=%d", users.Code)
	}
	body := users.Body.String()
	if !strings.Contains(body, `value="as_receive"`) || !strings.Contains(body, `value="as_process"`) {
		t.Fatal("사용자 관리에 AS 접수·조치 체크박스가 없다")
	}
}

package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
	"customer-support/internal/service/approval"
)

func TestLeaveFormSubmitInboxApproveHWPX(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "leave-http.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	head := &model.User{Username: "h1", PasswordHash: HashPassword("pw"), FullName: "팀장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01"}
	staff := &model.User{Username: "s1", PasswordHash: HashPassword("pw"), FullName: "직원", Role: model.RoleTech, IsActive: true, OrgID: "O01"}
	for _, u := range []*model.User{head, staff} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	h := New(db)
	if err := h.Leave.eng.SetOrgHead(approval.Actor{UserID: head.UserID, Role: model.RoleOrgAdmin}, head.UserID, "O01"); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/leaves", h.Leave.List)
	g.GET("/leaves/new", h.Leave.New)
	g.POST("/leaves", h.Leave.Create)
	g.GET("/leaves/:id", h.Leave.Show)
	g.GET("/leaves/:id/hwpx", h.Leave.Download)
	g.GET("/approvals/inbox", h.Leave.Inbox)
	g.GET("/approvals/:id", h.Leave.ApprovalShow)
	g.POST("/approvals/:id/approve", h.Leave.Approve)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/leaves/new", nil)
	req.AddCookie(jwtCookieUserClaims(t, staff))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "근태허가원") {
		t.Fatalf("form %d %s", rec.Code, rec.Body.String())
	}

	form := url.Values{
		"type_key": {"annual"}, "period_mode": {"days"},
		"start_date": {"2026-08-10"}, "end_date": {"2026-08-10"}, "reason": {"개인사정테스트"},
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/leaves", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieUserClaims(t, staff))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create %d %s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/leaves/LR-") {
		t.Fatalf("loc %s", loc)
	}
	leavePath := strings.Split(loc, "?")[0]

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, leavePath, nil)
	req.AddCookie(jwtCookieUserClaims(t, staff))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `data-stamps`) {
		t.Fatalf("show stamps %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "직원") || !strings.Contains(body, "승인 공란") {
		t.Fatalf("결재 중 작성 이름·승인 공란 없음")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/approvals/inbox", nil)
	req.AddCookie(jwtCookieUserClaims(t, head))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "근태허가원") {
		t.Fatalf("inbox %d %s", rec.Code, rec.Body.String())
	}

	var arID string
	_ = db.QueryRow(`SELECT request_id FROM approval_requests WHERE doc_type='leave' ORDER BY created_at DESC LIMIT 1`).Scan(&arID)
	if arID == "" {
		t.Fatal("ar")
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/approvals/"+arID+"/approve", strings.NewReader(""))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieUserClaims(t, head))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("approve %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, leavePath, nil)
	req.AddCookie(jwtCookieUserClaims(t, staff))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "팀장") || strings.Contains(rec.Body.String(), "승인 공란") {
		t.Fatalf("승인 후 이름 %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/approvals/"+arID, nil)
	req.AddCookie(jwtCookieUserClaims(t, head))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "팀장") {
		t.Fatalf("결재 상세 이름 %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, leavePath+"/hwpx", nil)
	req.AddCookie(jwtCookieUserClaims(t, staff))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.Len() < 100 {
		t.Fatalf("hwpx %d %d %s", rec.Code, rec.Body.Len(), rec.Body.String())
	}
	if rec.Body.Bytes()[0] != 'P' {
		t.Fatal("not zip")
	}
}

func TestLeaveReadOnlyBlocked(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "leave-ro.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	ro := &model.User{Username: "ro1", PasswordHash: HashPassword("pw"), FullName: "읽기", Role: model.RoleTech, IsActive: true, IsReadOnly: true, OrgID: "O01"}
	if err := users.Create(ro); err != nil {
		t.Fatal(err)
	}
	h := New(db)
	e := echo.New()
	e.Renderer = NewRenderer()
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.POST("/leaves", h.Leave.Create)
	form := url.Values{"type_key": {"annual"}, "period_mode": {"days"}, "start_date": {"2026-08-10"}, "reason": {"x"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/leaves", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieUserClaims(t, ro))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("ro %d", rec.Code)
	}
}

func TestCLevelLeaveWriteForbiddenViewOK(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "leave-clevel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	ceo := &model.User{
		Username: "ceo-leave", PasswordHash: HashPassword("pw"), FullName: "대표",
		Role: model.RoleCLevel, IsActive: true, IsReadOnly: false, OrgID: "O01", ProfileDone: true,
	}
	if err := users.Create(ceo); err != nil {
		t.Fatal(err)
	}
	h := New(db)
	e := echo.New()
	e.Renderer = NewRenderer()
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/leaves", h.Leave.List)
	g.GET("/leaves/new", h.Leave.New)
	g.POST("/leaves", h.Leave.Create)
	g.POST("/leaves/:id/withdraw", h.Leave.Withdraw)

	ck := jwtCookieUserClaims(t, ceo)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/leaves", nil)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/leaves/new", nil)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("new %d", rec.Code)
	}

	form := url.Values{"type_key": {"annual"}, "period_mode": {"days"}, "start_date": {"2026-08-10"}, "reason": {"x"}}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/leaves", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("create %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/leaves/LR-none/withdraw", strings.NewReader(""))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("withdraw %d", rec.Code)
	}
}

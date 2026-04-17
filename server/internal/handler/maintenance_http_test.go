package handler

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"

	"customer-support/internal/repository"
)

func jwtCookie(t *testing.T) *http.Cookie {
	t.Helper()
	secret := []byte("cs-system-jwt-secret-2026")
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":  "admin-id", "username": "admin", "role": "admin",
		"name": "관리자", "exp": time.Now().Add(time.Hour).Unix(),
	})
	s, err := token.SignedString(secret)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: "token", Value: s, Path: "/"}
}

func TestMaintenanceHTTP_ListRequiresAuth(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "http.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	userRepo := repository.NewUserRepo(db)
	userRepo.EnsureAdmin(HashPassword("admin"))

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)

	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/maintenance", h.Maintenance.ListPlans)

	// 비인증 → 리다이렉트
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/maintenance", nil)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "/login") {
		t.Fatalf("unauth: status=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}

	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "http://localhost/maintenance", nil)
	req2.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("auth: status=%d body=%s", rec2.Code, rec2.Body.String()[:min(200, rec2.Body.Len())])
	}
	if !strings.Contains(rec2.Body.String(), "정기점검") {
		t.Fatal("expected maintenance page body")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestMaintenanceHTTP_DeleteVisit_visitIDRoute(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "del.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?,?,?,1)`,
		"cx", "X", "X")
	if err != nil {
		t.Fatal(err)
	}
	userRepo := repository.NewUserRepo(db)
	userRepo.EnsureAdmin(HashPassword("admin"))

	repo := repository.NewMaintenanceRepo(db)
	p, err := repo.CreatePlan(2025, "del")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertVisit(p.PlanID, "2025-06-10", "cx", 0, 0, "normal", ""); err != nil {
		t.Fatal(err)
	}
	visits, err := repo.ListVisits(p.PlanID)
	if err != nil || len(visits) != 1 {
		t.Fatalf("setup visits: %v", err)
	}
	vid := visits[0].VisitID

	e := echo.New()
	h := New(db)
	e.POST("/maintenance/visits/:visit_id/delete", h.Maintenance.DeleteVisit, h.Auth.AuthMiddleware)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/maintenance/visits/"+vid+"/delete?plan_id="+p.PlanID, nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("delete: status=%d", rec.Code)
	}

	visits, _ = repo.ListVisits(p.PlanID)
	if len(visits) != 0 {
		t.Fatalf("visit should be deleted, len=%d", len(visits))
	}
}

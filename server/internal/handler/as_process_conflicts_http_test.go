package handler

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/repository"
)

func TestProcessConflictsExcelAndAdminTab(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c-xlsx','엑셀도서관','엑셀도서관',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, status, symptom, data_origin, action_taken
	) VALUES ('as-xlsx','R2609-XL','c-xlsx','2026-09-01','completed','증상','app','접수본문XYZ')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_processes (process_id, as_id, process_datetime, work_content)
		VALUES ('p-xlsx','as-xlsx','2026-09-02','이력본문ABC')`); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/admin/data", h.Backup.Page)
	g.GET("/admin/data/process-conflicts.xlsx", h.Backup.ProcessConflictsExcel)

	page := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/admin/data", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(page, req)
	if page.Code != http.StatusOK {
		t.Fatalf("화면: status=%d", page.Code)
	}
	if !strings.Contains(page.Body.String(), "조치 단일화") {
		t.Fatal("조치 단일화 탭이 없다")
	}

	tab := httptest.NewRecorder()
	reqT := httptest.NewRequest(http.MethodGet, "http://localhost/admin/data?tab=processes", nil)
	reqT.AddCookie(jwtCookie(t))
	e.ServeHTTP(tab, reqT)
	body := tab.Body.String()
	if tab.Code != http.StatusOK {
		t.Fatalf("탭: status=%d", tab.Code)
	}
	if !strings.Contains(body, "R2609-XL") || !strings.Contains(body, "접수본문XYZ") || !strings.Contains(body, "이력본문ABC") {
		t.Fatalf("어긋난 13건 목록이 없다: %s", body)
	}
	if !strings.Contains(body, "/admin/data/process-conflicts.xlsx") {
		t.Fatal("xlsx 받기 링크가 없다")
	}

	xl := httptest.NewRecorder()
	reqX := httptest.NewRequest(http.MethodGet, "http://localhost/admin/data/process-conflicts.xlsx", nil)
	reqX.AddCookie(jwtCookie(t))
	e.ServeHTTP(xl, reqX)
	if xl.Code != http.StatusOK {
		t.Fatalf("xlsx: status=%d body=%s", xl.Code, xl.Body.String())
	}
	ct := xl.Header().Get(echo.HeaderContentType)
	if !strings.Contains(ct, "spreadsheetml") {
		t.Fatalf("content-type=%q", ct)
	}
	raw := xl.Body.Bytes()
	if len(raw) < 4 || string(raw[:2]) != "PK" {
		t.Fatalf("xlsx 매직 아님 len=%d", len(raw))
	}
}

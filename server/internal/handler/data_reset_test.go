package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func newDataResetApp(t *testing.T) (*echo.Echo, *Handler, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "app.db")
	db, err := repository.InitDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	settings := repository.NewSettingsRepo(db)
	if err := settings.Set(repository.SettingVisionAdminPassword, HashPassword("vision-lock")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id, is_test)
		VALUES ('C-live','실기관','실기관',1,?,0)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id, is_test)
		VALUES ('C-test','테스트기관','테스트기관',1,?,1)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	h.DataReset.cfg.DataDir = dir
	h.DataReset.cfg.DB = db
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/admin/data/reset", h.DataReset.Page, h.Auth.RequireDataResetMW)
	g.POST("/admin/data/reset", h.DataReset.Execute, h.Auth.RequireDataResetMW)
	return e, h, dbPath
}

func postReset(t *testing.T, e *echo.Echo, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/data/reset", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	return rec
}

func liveTestCounts(t *testing.T, h *Handler) (live, test int) {
	t.Helper()
	if err := h.DataReset.cfg.DB.QueryRow(`SELECT COUNT(*) FROM customers WHERE customer_id='C-live'`).Scan(&live); err != nil {
		t.Fatal(err)
	}
	if err := h.DataReset.cfg.DB.QueryRow(`SELECT COUNT(*) FROM customers WHERE customer_id='C-test'`).Scan(&test); err != nil {
		t.Fatal(err)
	}
	return
}

func TestDataResetRequiresPassword(t *testing.T) {
	e, h, _ := newDataResetApp(t)
	form := url.Values{
		"scope":           {"test"},
		"confirm_name":    {model.OrgNameLibrary},
		"vision_password": {""},
	}
	rec := postReset(t, e, form)
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("비밀번호 없이 실행됨 code=%d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
	live, testN := liveTestCounts(t, h)
	if live != 1 || testN != 1 {
		t.Fatalf("삭제되면 안 됨 live=%d test=%d", live, testN)
	}
}

func TestDataResetAbortedWhenSnapshotFails(t *testing.T) {
	e, h, _ := newDataResetApp(t)
	h.DataReset.snapshotFn = func() (string, error) {
		return "", fmt.Errorf("디스크 가득")
	}
	form := url.Values{
		"scope":           {"test"},
		"confirm_name":    {model.OrgNameLibrary},
		"vision_password": {"vision-lock"},
	}
	rec := postReset(t, e, form)
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || !strings.Contains(loc, "err=") {
		t.Fatalf("백업 실패 처리 아님 code=%d loc=%s", rec.Code, loc)
	}
	live, testN := liveTestCounts(t, h)
	if live != 1 || testN != 1 {
		t.Fatalf("백업 실패 후 초기화됨 live=%d test=%d", live, testN)
	}
}

func TestDataResetTestScopeKeepsLiveRows(t *testing.T) {
	e, h, _ := newDataResetApp(t)
	h.DataReset.snapshotFn = func() (string, error) { return "manual-test", nil }
	form := url.Values{
		"scope":           {"test"},
		"confirm_name":    {model.OrgNameLibrary},
		"vision_password": {"vision-lock"},
	}
	rec := postReset(t, e, form)
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("실행 실패 loc=%s", rec.Header().Get("Location"))
	}
	live, testN := liveTestCounts(t, h)
	if live != 1 || testN != 0 {
		t.Fatalf("범위 ① live=%d test=%d", live, testN)
	}
}

func TestDataResetPageDefaultsToTesterScope(t *testing.T) {
	e, _, _ := newDataResetApp(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/data/reset", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `value="test"`) || !strings.Contains(body, "checked") {
		t.Fatalf("기본 범위가 테스터가 아님")
	}
	if !strings.Contains(body, "현재 데이터는 사라집니다") {
		t.Fatalf("복구 경고 문구 없음")
	}
}

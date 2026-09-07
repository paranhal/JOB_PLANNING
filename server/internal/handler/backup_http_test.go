package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/audit"
	"customer-support/internal/backup"
	"customer-support/internal/repository"
)

func TestBackupPageAndSave(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))

	dataDir := filepath.Join(dir, "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	h.Backup.cfg.DataDir = dataDir
	h.Backup.cfg.DB = db

	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/admin/data", h.Backup.Page)
	g.POST("/admin/data/save", h.Backup.Save)
	g.POST("/admin/data/metrics", h.Backup.SaveMetrics)
	g.POST("/admin/backup", h.Backup.Save)

	show := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/admin/data", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(show, req)
	if show.Code != http.StatusOK {
		t.Fatalf("화면: status=%d body=%s", show.Code, show.Body.String())
	}
	body := show.Body.String()
	if !strings.Contains(body, "현재 데이터 저장") {
		t.Error("저장 버튼이 없다")
	}
	if !strings.Contains(body, "변경 이력") {
		t.Error("변경 이력 탭이 없다")
	}
	if !strings.Contains(body, "사업 미배정") {
		t.Error("사업 미배정 탭이 없다")
	}
	if !strings.Contains(body, "정합성 점검") {
		t.Error("정합성 점검 탭이 없다")
	}
	if !strings.Contains(body, "AS 반입") {
		t.Error("AS 반입 탭이 없다")
	}
	if !strings.Contains(body, "지표 기준") {
		t.Error("지표 기준 탭이 없다")
	}

	un := httptest.NewRecorder()
	reqU := httptest.NewRequest(http.MethodGet, "http://localhost/admin/data?tab=unassigned", nil)
	reqU.AddCookie(jwtCookie(t))
	e.ServeHTTP(un, reqU)
	if un.Code != http.StatusOK {
		t.Fatalf("미배정 탭: status=%d body=%s", un.Code, un.Body.String())
	}
	if !strings.Contains(un.Body.String(), "사업 미배정") {
		t.Error("미배정 화면 제목이 없다")
	}

	chk := httptest.NewRecorder()
	reqC := httptest.NewRequest(http.MethodGet, "http://localhost/admin/data?tab=checks", nil)
	reqC.AddCookie(jwtCookie(t))
	e.ServeHTTP(chk, reqC)
	if chk.Code != http.StatusOK {
		t.Fatalf("정합성 탭: status=%d body=%s", chk.Code, chk.Body.String())
	}
	if !strings.Contains(chk.Body.String(), "V-11") || !strings.Contains(chk.Body.String(), "연도가 2000") {
		t.Error("V-11 연도 범위 항목이 없다")
	}
	if !strings.Contains(chk.Body.String(), "V-12") || !strings.Contains(chk.Body.String(), "착수가 완료보다") {
		t.Error("V-12 착수>완료 항목이 없다")
	}

	met := httptest.NewRecorder()
	reqM := httptest.NewRequest(http.MethodGet, "http://localhost/admin/data?tab=metrics", nil)
	reqM.AddCookie(jwtCookie(t))
	e.ServeHTTP(met, reqM)
	if met.Code != http.StatusOK {
		t.Fatalf("지표 탭: status=%d", met.Code)
	}
	mb := met.Body.String()
	if !strings.Contains(mb, "name=\"metrics_base_date\"") || !strings.Contains(mb, "name=\"progress_scope\"") {
		t.Error("지표 설정 폼이 없다")
	}
	if !strings.Contains(mb, "4주 연속 90%") {
		t.Error("실행률 되돌림 조건이 없다")
	}

	form := url.Values{}
	form.Set("metrics_base_date", "2026-08-10")
	form.Set("progress_scope", "as,maintenance")
	recM := httptest.NewRecorder()
	reqSave := httptest.NewRequest(http.MethodPost, "http://localhost/admin/data/metrics", strings.NewReader(form.Encode()))
	reqSave.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqSave.AddCookie(jwtCookie(t))
	e.ServeHTTP(recM, reqSave)
	if recM.Code != http.StatusSeeOther {
		t.Fatalf("지표 저장: status=%d", recM.Code)
	}
	got, _ := repository.NewSettingsRepo(db).Get(repository.SettingMetricsBaseDate)
	if got != "2026-08-10" {
		t.Fatalf("기준일 저장=%q", got)
	}

	rec := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "http://localhost/admin/data/save", nil)
	req2.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req2)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("저장: status=%d body=%s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "/admin/data?ok=") {
		t.Fatalf("리다이렉트: %q", loc)
	}
	items, err := backup.List(dataDir)
	if err != nil || len(items) != 1 || items[0].Kind != backup.KindManual {
		t.Fatalf("저장된 복사본: %+v err=%v", items, err)
	}
	if !strings.HasPrefix(items[0].Name, "수동저장_") {
		t.Fatalf("수동 저장 폴더명: %q", items[0].Name)
	}
	if items[0].KindLabel != "사용자 백업" {
		t.Fatalf("구분 라벨: %q", items[0].KindLabel)
	}
	if audit.BackupRowCount() != 1 {
		t.Fatalf("data_backups 건수=%d want 1", audit.BackupRowCount())
	}
}

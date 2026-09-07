package handler

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/backup"
	"customer-support/internal/repository"
)

func newImportHTTP(t *testing.T) (*echo.Echo, *repository.ASImportRepo) {
	t.Helper()
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?,?,?,1)`,
		"c1", "가나도서관", "가나도서관"); err != nil {
		t.Fatal(err)
	}
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	h.Backup.cfg = backup.Config{DataDir: filepath.Join(dir, "data"), DB: db}
	_ = os.MkdirAll(h.Backup.cfg.DataDir, 0755)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/admin/data", h.Backup.Page)
	g.POST("/admin/data/import", h.Backup.ImportUpload)
	g.POST("/admin/data/import/:id/exclude", h.Backup.ImportExclude)
	g.POST("/admin/data/import/:id/apply", h.Backup.ImportApply)
	g.POST("/admin/data/import/:id/cancel", h.Backup.ImportCancel)
	return e, repository.NewASImportRepo(db)
}

func TestASImportHTTPReportThenApplyThenCancel(t *testing.T) {
	e, repo := newImportHTTP(t)
	csv := "거래처명,접수일자,내용,답변 및 처리\n가나도서관,2024-06-01,팝업 오류입니다,재시작\n없는기관,2024-06-02,접속 안 됨,확인\n"
	rec := postImportCSV(t, e, csv)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("upload status=%d %s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "tab=import") || !strings.Contains(loc, "batch=") {
		t.Fatalf("redirect: %s", loc)
	}
	batchID := importBatchFromLoc(loc)

	page := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost"+loc, nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(page, req)
	if page.Code != http.StatusOK {
		t.Fatalf("report status=%d", page.Code)
	}
	body := page.Body.String()
	if !strings.Contains(body, "검증 리포트") || !strings.Contains(body, "고객 매칭 실패") {
		t.Fatal("검증 리포트에 매칭 실패가 없다")
	}

	applyBad := httptest.NewRecorder()
	reqA := httptest.NewRequest(http.MethodPost, "http://localhost/admin/data/import/"+batchID+"/apply", nil)
	reqA.AddCookie(jwtCookie(t))
	e.ServeHTTP(applyBad, reqA)
	if applyBad.Code != http.StatusSeeOther {
		t.Fatalf("apply status=%d", applyBad.Code)
	}
	if !strings.Contains(applyBad.Header().Get("Location"), "err=") {
		t.Fatal("오류가 남은 채 반영되면 안 된다")
	}

	rows, _ := repo.ListRows(batchID)
	for _, row := range rows {
		if !row.HasBlocker() {
			continue
		}
		form := url.Values{"row_no": {strconv.Itoa(row.RowNo)}, "excluded": {"1"}}
		ex := httptest.NewRecorder()
		reqE := httptest.NewRequest(http.MethodPost, "http://localhost/admin/data/import/"+batchID+"/exclude", strings.NewReader(form.Encode()))
		reqE.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		reqE.AddCookie(jwtCookie(t))
		e.ServeHTTP(ex, reqE)
	}

	applyOK := httptest.NewRecorder()
	reqB := httptest.NewRequest(http.MethodPost, "http://localhost/admin/data/import/"+batchID+"/apply", nil)
	reqB.AddCookie(jwtCookie(t))
	e.ServeHTTP(applyOK, reqB)
	if applyOK.Code != http.StatusSeeOther || strings.Contains(applyOK.Header().Get("Location"), "err=") {
		t.Fatalf("apply loc=%s", applyOK.Header().Get("Location"))
	}

	cancel := httptest.NewRecorder()
	reqC := httptest.NewRequest(http.MethodPost, "http://localhost/admin/data/import/"+batchID+"/cancel", nil)
	reqC.AddCookie(jwtCookie(t))
	e.ServeHTTP(cancel, reqC)
	if cancel.Code != http.StatusSeeOther || strings.Contains(cancel.Header().Get("Location"), "err=") {
		t.Fatalf("cancel loc=%s", cancel.Header().Get("Location"))
	}
}

func postImportCSV(t *testing.T, e *echo.Echo, csv string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", "as.csv")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write([]byte(csv))
	_ = w.Close()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/admin/data/import", &buf)
	req.Header.Set(echo.HeaderContentType, w.FormDataContentType())
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	return rec
}

func importBatchFromLoc(loc string) string {
	u, err := url.Parse(loc)
	if err != nil {
		return ""
	}
	return u.Query().Get("batch")
}

package handler

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/xuri/excelize/v2"

	"customer-support/internal/auditlog"
	"customer-support/internal/repository"
)

func TestCreateCompanyWeeklyPreservesOriginalAndDeletesUpload(t *testing.T) {
	dir := t.TempDir()
	accessPath := initTestAccessDB(t)
	db, err := repository.InitDB(filepath.Join(dir, "cw.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.POST("/stats/company-weekly", h.Stats.CreateCompanyWeekly)
	g.GET("/stats/company-weekly/download", h.Stats.DownloadCompanyWeekly)

	src := mustCompanyWeeklyFixture(t)
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("company_date", "2026-08-17")
	_ = w.WriteField("sheet_name", "26.8월3주(업무)")
	_ = w.WriteField("prev_401", "·전주 손질")
	_ = w.WriteField("plan_401", "·금주 손질")
	_ = w.WriteField("help_401", "지원")
	_ = w.WriteField("decision_401", "결정")
	fw, err := w.CreateFormFile("file", "260814_주간업무보고.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(src); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/stats/company-weekly", &buf)
	req.Header.Set(echo.HeaderContentType, w.FormDataContentType())
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String()[:min(400, rec.Body.Len())])
	}
	body := rec.Body.String()
	if !strings.Contains(body, "원본 시트 7개 유지") {
		t.Fatalf("결과 문구 없음: %s", trimBody(body))
	}
	if !strings.Contains(body, "수식") || !strings.Contains(body, "개 유지") {
		t.Fatal("수식 유지 문구 없음")
	}
	if strings.Contains(body, "한 장짜리") {
		t.Fatal("보존 가능한 픽스처인데 후퇴했다")
	}
	token := downloadTokenFrom(body)
	if token == "" {
		t.Fatal("내려받기 토큰 없음")
	}

	logs := readAccessLogs(t, accessPath)
	found := false
	for _, row := range logs {
		if row.Action == auditlog.ActionDownload && row.Target == "company_weekly_report" && row.Result == auditlog.ResultOK {
			found = true
			if strings.Contains(row.Before, "VLOOKUP") || strings.Contains(row.Before, "계약") {
				t.Fatal("접속기록에 파일 내용이 들어 있다")
			}
		}
	}
	if !found {
		t.Fatalf("접속기록 없음: %+v", logs)
	}

	dl := httptest.NewRecorder()
	dreq := httptest.NewRequest(http.MethodGet, "http://localhost/stats/company-weekly/download?token="+token, nil)
	dreq.AddCookie(jwtCookie(t))
	e.ServeHTTP(dl, dreq)
	if dl.Code != http.StatusOK {
		t.Fatalf("download status=%d", dl.Code)
	}
	data, err := io.ReadAll(dl.Body)
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if len(f.GetSheetList()) != 8 {
		t.Fatalf("시트=%v", f.GetSheetList())
	}
	assertCompanyWeeklyBizPreserved(t, f)
	f20, _ := f.GetCellValue("26.8월3주(업무)", "F20")
	if f20 != "·전주 손질" {
		t.Fatalf("F20=%q", f20)
	}

	again := httptest.NewRecorder()
	areq := httptest.NewRequest(http.MethodGet, "http://localhost/stats/company-weekly/download?token="+token, nil)
	areq.AddCookie(jwtCookie(t))
	e.ServeHTTP(again, areq)
	if again.Code == http.StatusOK && strings.Contains(again.Header().Get("Content-Type"), "spreadsheet") {
		t.Fatal("토큰을 두 번 쓰면 안 된다")
	}
}

func downloadTokenFrom(body string) string {
	const prefix = `/stats/company-weekly/download?token=`
	i := strings.Index(body, prefix)
	if i < 0 {
		return ""
	}
	rest := body[i+len(prefix):]
	end := strings.IndexAny(rest, `"'& <`)
	if end < 0 {
		return rest
	}
	return rest[:end]
}

func trimBody(s string) string {
	if len(s) > 400 {
		return s[:400]
	}
	return s
}

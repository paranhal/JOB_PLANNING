package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestKnowledgeGapHTTPRecordWriteResolve(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "gap.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?,?,?,1)`,
		"c1", "가나도서관", "가나도서관"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO assets (asset_id, customer_id, product_name) VALUES ('A1','c1','무인예약기')`); err != nil {
		t.Fatal(err)
	}
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	asRepo := repository.NewASRepo(db)
	as := &model.ASReceipt{
		CustomerID: "c1", AssetID: "A1", Symptom: "로그인 화면이 안 열림",
		ReceiptDatetime: time.Date(2026, 8, 12, 9, 0, 0, 0, time.Local),
		Urgency:         "normal", Priority: "normal",
	}
	if err := asRepo.Create(as); err != nil {
		t.Fatal(err)
	}
	if _, err := asRepo.RebuildKeywordLinks([]string{as.ASID}); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/as/knowledge", h.AS.Knowledge)
	g.GET("/as/knowledge/gaps", h.AS.KnowledgeGaps)
	g.POST("/as/knowledge", h.AS.CreateKnowledge)

	get := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://localhost"+path, nil)
		req.AddCookie(jwtCookie(t))
		e.ServeHTTP(rec, req)
		return rec
	}
	post := func(form url.Values) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://localhost/as/knowledge", strings.NewReader(form.Encode()))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		req.AddCookie(jwtCookie(t))
		e.ServeHTTP(rec, req)
		return rec
	}

	q := "/as/knowledge?q=" + url.QueryEscape("RFID 태그 인식 불량")
	if rec := get(q); rec.Code != http.StatusOK {
		t.Fatalf("search1 %d", rec.Code)
	}
	if rec := get(q); rec.Code != http.StatusOK {
		t.Fatalf("search2 %d", rec.Code)
	}
	open, err := asRepo.ListOpenKBGaps()
	if err != nil || len(open) != 1 || open[0].SearchCount != 2 {
		t.Fatalf("자동 기록 %+v err=%v", open, err)
	}

	page := get("/as/knowledge/gaps")
	if page.Code != http.StatusOK {
		t.Fatalf("gaps %d %s", page.Code, page.Body.String())
	}
	body := page.Body.String()
	if !strings.Contains(body, "RFID 태그 인식 불량") || !strings.Contains(body, "2회 찾음") {
		t.Fatal("횟수 순 검색어가 없다")
	}
	if !strings.Contains(body, "로그인 관련") && !strings.Contains(body, "로그인") {
		t.Fatal("조치 없는 AS 묶음이 없다")
	}
	if !strings.Contains(body, "지식 쓰기") || !strings.Contains(body, "이번 달에 채운 지식") {
		t.Fatal("진척·쓰기 버튼이 없다")
	}

	if rec := post(url.Values{
		"from":        {"gaps"},
		"gap_id":      {open[0].GapID},
		"symptom_text": {open[0].Query},
		"action_text": {"리더기 재시작 후 태그를 다시 등록합니다"},
	}); rec.Code != http.StatusSeeOther {
		t.Fatalf("write %d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
	got, err := asRepo.GetKBGap(open[0].GapID)
	if err != nil || strings.TrimSpace(got.ResolvedKBID) == "" {
		t.Fatalf("resolved %+v err=%v", got, err)
	}
	after := get("/as/knowledge/gaps")
	if strings.Contains(after.Body.String(), "RFID 태그 인식 불량") {
		t.Fatal("채운 검색어가 목록에 남았다")
	}
}
